package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Migrator struct {
	db *sql.DB
}

type migration struct {
	version string
	sql     string
}

func NewMigrator(database *sql.DB) *Migrator {
	return &Migrator{db: database}
}

func RunMigrations(ctx context.Context, database *sql.DB) error {
	return NewMigrator(database).Migrate(ctx)
}

func (m *Migrator) Migrate(ctx context.Context) error {
	if m == nil || m.db == nil {
		return fmt.Errorf("migrate: database is nil")
	}

	if _, err := m.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		applied, err := m.isApplied(ctx, migration.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		if err := m.apply(ctx, migration); err != nil {
			return err
		}
	}

	return nil
}

func (m *Migrator) isApplied(ctx context.Context, version string) (bool, error) {
	var applied bool
	if err := m.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM schema_migrations WHERE version = $1
		)
	`, version).Scan(&applied); err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}

	return applied, nil
}

func (m *Migrator) apply(ctx context.Context, migration migration) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", migration.version, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.version, err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (version) VALUES ($1)
	`, migration.version); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", migration.version, err)
	}

	return nil
}

func loadMigrations() ([]migration, error) {
	paths, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("load migrations: %w", err)
	}
	sort.Strings(paths)

	migrations := make([]migration, 0, len(paths))
	for _, path := range paths {
		contents, err := migrationFiles.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", path, err)
		}

		version := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		migrations = append(migrations, migration{
			version: version,
			sql:     string(contents),
		})
	}

	return migrations, nil
}
