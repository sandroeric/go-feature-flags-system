//go:build integration

package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"launchdarkly/internal/domain"
)

func TestRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}

	ctx := context.Background()
	database, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("OpenPostgres() error = %v", err)
	}
	defer database.Close()

	if err := RunMigrations(ctx, database); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	repo := NewRepository(database)
	key := "integration_" + time.Now().UTC().Format("20060102150405")
	flag := integrationFlag(key)

	created, err := repo.CreateFlag(ctx, flag)
	if err != nil {
		t.Fatalf("CreateFlag() error = %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("created version = %d, want 1", created.Version)
	}

	loaded, err := repo.GetFlag(ctx, key)
	if err != nil {
		t.Fatalf("GetFlag() error = %v", err)
	}
	if loaded.Key != key {
		t.Fatalf("loaded key = %q, want %q", loaded.Key, key)
	}

	loaded.Enabled = false
	updated, err := repo.UpdateFlag(ctx, key, loaded)
	if err != nil {
		t.Fatalf("UpdateFlag() error = %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("updated version = %d, want 2", updated.Version)
	}

	flags, err := repo.LoadAllFlags(ctx)
	if err != nil {
		t.Fatalf("LoadAllFlags() error = %v", err)
	}
	if len(flags) == 0 {
		t.Fatal("LoadAllFlags() returned no flags")
	}

	if err := repo.DeleteFlag(ctx, key); err != nil {
		t.Fatalf("DeleteFlag() error = %v", err)
	}
	if _, err := repo.GetFlag(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetFlag() error = %v, want ErrNotFound", err)
	}
}

func integrationFlag(key string) domain.Flag {
	return domain.Flag{
		Key:     key,
		Enabled: true,
		Default: "off",
		Variants: []domain.Variant{
			{Name: "off", Weight: 50},
			{Name: "on", Weight: 50},
		},
		Rules: []domain.Rule{
			{
				Attribute: "country",
				Operator:  domain.OperatorEq,
				Values:    []string{"BR"},
				Variant:   "on",
				Priority:  1,
			},
		},
		Version: 1,
	}
}
