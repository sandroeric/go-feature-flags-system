package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SYNC_INTERVAL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8080")
	}
	if cfg.SyncInterval != 5*time.Second {
		t.Fatalf("SyncInterval = %s, want %s", cfg.SyncInterval, 5*time.Second)
	}
}

func TestLoadFromEnvironment(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://localhost/flags")
	t.Setenv("SYNC_INTERVAL", "2s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":9090")
	}
	if cfg.DatabaseURL != "postgres://localhost/flags" {
		t.Fatalf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://localhost/flags")
	}
	if cfg.SyncInterval != 2*time.Second {
		t.Fatalf("SyncInterval = %s, want %s", cfg.SyncInterval, 2*time.Second)
	}
}

func TestLoadRejectsInvalidSyncInterval(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SYNC_INTERVAL", "nope")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}
