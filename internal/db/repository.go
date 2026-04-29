package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"launchdarkly/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	db *sql.DB
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func NewRepository(database *sql.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) CreateFlag(ctx context.Context, flag domain.Flag) (domain.Flag, error) {
	if flag.Version == 0 {
		flag.Version = 1
	}
	if err := domain.ValidateFlag(flag); err != nil {
		return domain.Flag{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Flag{}, fmt.Errorf("begin create flag: %w", err)
	}
	defer tx.Rollback()

	if err := insertFlag(ctx, tx, flag); err != nil {
		return domain.Flag{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Flag{}, fmt.Errorf("commit create flag: %w", err)
	}

	return flag, nil
}

func (r *Repository) UpdateFlag(ctx context.Context, key string, flag domain.Flag) (domain.Flag, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return domain.Flag{}, domain.ValidationErrors{{
			Field:   "key",
			Code:    "required",
			Message: "flag key is required",
		}}
	}
	if flag.Key != "" && flag.Key != key {
		return domain.Flag{}, domain.ValidationErrors{{
			Field:   "key",
			Code:    "mismatch",
			Message: "flag key must match the path key",
		}}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Flag{}, fmt.Errorf("begin update flag: %w", err)
	}
	defer tx.Rollback()

	var currentVersion int
	if err := tx.QueryRowContext(ctx, `
		SELECT version FROM flags WHERE key = $1 FOR UPDATE
	`, key).Scan(&currentVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Flag{}, ErrNotFound
		}
		return domain.Flag{}, fmt.Errorf("load flag version: %w", err)
	}

	flag.Key = key
	flag.Version = currentVersion + 1
	if err := domain.ValidateFlag(flag); err != nil {
		return domain.Flag{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM rules WHERE flag_key = $1`, key); err != nil {
		return domain.Flag{}, fmt.Errorf("delete rules: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM variants WHERE flag_key = $1`, key); err != nil {
		return domain.Flag{}, fmt.Errorf("delete variants: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE flags
		SET enabled = $2, default_variant = $3, version = $4
		WHERE key = $1
	`, flag.Key, flag.Enabled, flag.Default, flag.Version); err != nil {
		return domain.Flag{}, fmt.Errorf("update flag: %w", err)
	}
	if err := insertVariants(ctx, tx, flag); err != nil {
		return domain.Flag{}, err
	}
	if err := insertRules(ctx, tx, flag); err != nil {
		return domain.Flag{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Flag{}, fmt.Errorf("commit update flag: %w", err)
	}

	return flag, nil
}

func (r *Repository) DeleteFlag(ctx context.Context, key string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM flags WHERE key = $1`, key)
	if err != nil {
		return fmt.Errorf("delete flag: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete flag rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *Repository) GetFlag(ctx context.Context, key string) (domain.Flag, error) {
	return loadFlag(ctx, r.db, key)
}

func (r *Repository) ListFlags(ctx context.Context) ([]domain.Flag, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT key FROM flags ORDER BY key
	`)
	if err != nil {
		return nil, fmt.Errorf("list flag keys: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan flag key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate flag keys: %w", err)
	}

	flags := make([]domain.Flag, 0, len(keys))
	for _, key := range keys {
		flag, err := loadFlag(ctx, r.db, key)
		if err != nil {
			return nil, err
		}
		flags = append(flags, flag)
	}

	return flags, nil
}

func (r *Repository) LoadAllFlags(ctx context.Context) ([]domain.Flag, error) {
	return r.ListFlags(ctx)
}

func insertFlag(ctx context.Context, tx *sql.Tx, flag domain.Flag) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO flags (key, enabled, default_variant, version)
		VALUES ($1, $2, $3, $4)
	`, flag.Key, flag.Enabled, flag.Default, flag.Version); err != nil {
		return fmt.Errorf("insert flag: %w", err)
	}
	if err := insertVariants(ctx, tx, flag); err != nil {
		return err
	}
	if err := insertRules(ctx, tx, flag); err != nil {
		return err
	}

	return nil
}

func insertVariants(ctx context.Context, tx *sql.Tx, flag domain.Flag) error {
	for _, variant := range flag.Variants {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO variants (flag_key, name, weight)
			VALUES ($1, $2, $3)
		`, flag.Key, variant.Name, variant.Weight); err != nil {
			return fmt.Errorf("insert variant %s: %w", variant.Name, err)
		}
	}

	return nil
}

func insertRules(ctx context.Context, tx *sql.Tx, flag domain.Flag) error {
	for _, rule := range flag.Rules {
		values, err := json.Marshal(rule.Values)
		if err != nil {
			return fmt.Errorf("marshal rule values: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO rules (flag_key, attribute, operator, values_json, variant, priority)
			VALUES ($1, $2, $3, $4::jsonb, $5, $6)
		`, flag.Key, rule.Attribute, rule.Operator, string(values), rule.Variant, rule.Priority); err != nil {
			return fmt.Errorf("insert rule priority %d: %w", rule.Priority, err)
		}
	}

	return nil
}

func loadFlag(ctx context.Context, q queryer, key string) (domain.Flag, error) {
	var flag domain.Flag
	if err := q.QueryRowContext(ctx, `
		SELECT key, enabled, default_variant, version
		FROM flags
		WHERE key = $1
	`, key).Scan(&flag.Key, &flag.Enabled, &flag.Default, &flag.Version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Flag{}, ErrNotFound
		}
		return domain.Flag{}, fmt.Errorf("load flag: %w", err)
	}

	variants, err := loadVariants(ctx, q, key)
	if err != nil {
		return domain.Flag{}, err
	}
	flag.Variants = variants

	rules, err := loadRules(ctx, q, key)
	if err != nil {
		return domain.Flag{}, err
	}
	flag.Rules = rules

	return flag, nil
}

func loadVariants(ctx context.Context, q queryer, key string) ([]domain.Variant, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT name, weight
		FROM variants
		WHERE flag_key = $1
		ORDER BY name
	`, key)
	if err != nil {
		return nil, fmt.Errorf("load variants: %w", err)
	}
	defer rows.Close()

	var variants []domain.Variant
	for rows.Next() {
		var variant domain.Variant
		if err := rows.Scan(&variant.Name, &variant.Weight); err != nil {
			return nil, fmt.Errorf("scan variant: %w", err)
		}
		variants = append(variants, variant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate variants: %w", err)
	}

	return variants, nil
}

func loadRules(ctx context.Context, q queryer, key string) ([]domain.Rule, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT attribute, operator, values_json, variant, priority
		FROM rules
		WHERE flag_key = $1
		ORDER BY priority
	`, key)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}
	defer rows.Close()

	var rules []domain.Rule
	for rows.Next() {
		var rule domain.Rule
		var values []byte
		if err := rows.Scan(&rule.Attribute, &rule.Operator, &values, &rule.Variant, &rule.Priority); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		if err := json.Unmarshal(values, &rule.Values); err != nil {
			return nil, fmt.Errorf("unmarshal rule values: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}

	return rules, nil
}
