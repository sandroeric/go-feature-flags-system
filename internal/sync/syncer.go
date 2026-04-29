package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"launchdarkly/internal/domain"
	"launchdarkly/internal/eval"
	"launchdarkly/internal/store"
)

// Repository defines the interface for loading flags from persistence.
type Repository interface {
	LoadAllFlags(context.Context) ([]domain.Flag, error)
}

// Holder defines the interface for updating the in-memory store.
type Holder interface {
	Current() *store.Store
	Swap(*store.Store)
}

// Syncer loads flags from the database, compiles them, and updates the in-memory store.
type Syncer struct {
	repo      Repository
	holder    Holder
	lastSync  time.Time
	lastError error
}

// NewSyncer creates a new syncer.
func NewSyncer(repo Repository, holder Holder) *Syncer {
	return &Syncer{
		repo:   repo,
		holder: holder,
	}
}

// Sync performs a full reload from the database.
// It compiles all flags into a fresh immutable Store and atomically swaps it.
// If the refresh fails, the old store is kept active and the error is logged.
func (s *Syncer) Sync(ctx context.Context) error {
	// Load all flags from database
	flags, err := s.repo.LoadAllFlags(ctx)
	if err != nil {
		s.lastError = err
		slog.Error("failed to load flags from database", "error", err)
		return fmt.Errorf("load flags: %w", err)
	}

	// Compile all flags
	compiled := make([]*eval.CompiledFlag, 0, len(flags))
	for _, flag := range flags {
		// Skip invalid flags but log them
		cf, err := eval.CompileFlag(flag)
		if err != nil {
			slog.Warn("failed to compile flag, skipping", "key", flag.Key, "error", err)
			continue
		}
		compiled = append(compiled, cf)
	}

	// Create new store with next generation
	current := s.holder.Current()
	nextGeneration := current.Generation() + 1
	newStore := store.NewWithGeneration(nextGeneration, compiled...)

	// Atomically swap
	s.holder.Swap(newStore)
	s.lastSync = time.Now().UTC()
	s.lastError = nil

	slog.Info("synced flags from database", "count", len(compiled), "generation", nextGeneration)
	return nil
}

// LastSync returns the time of the last successful sync.
func (s *Syncer) LastSync() time.Time {
	return s.lastSync
}

// LastError returns the error from the last sync attempt.
func (s *Syncer) LastError() error {
	return s.lastError
}

// StartPolling starts a background goroutine that periodically syncs the store.
// It returns a channel that can be used to stop the polling.
func StartPolling(ctx context.Context, syncer *Syncer, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Sync immediately on startup
		if err := syncer.Sync(ctx); err != nil {
			slog.Error("initial sync failed", "error", err)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := syncer.Sync(ctx); err != nil {
					slog.Error("sync failed", "error", err)
				}
			}
		}
	}()

	return done
}
