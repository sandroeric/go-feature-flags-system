package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"launchdarkly/internal/domain"
	"launchdarkly/internal/eval"
	"launchdarkly/internal/store"
)

// mockRepository is a test implementation of Repository
type mockRepository struct {
	flags []domain.Flag
	err   error
}

func (m *mockRepository) LoadAllFlags(ctx context.Context) ([]domain.Flag, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.flags, nil
}

// mockHolder is a test implementation of Holder
type mockHolder struct {
	current *store.Store
}

func (m *mockHolder) Current() *store.Store {
	return m.current
}

func (m *mockHolder) Swap(s *store.Store) {
	m.current = s
}

func TestSyncSuccess(t *testing.T) {
	flag := domain.Flag{
		Key:     "test",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}

	repo := &mockRepository{flags: []domain.Flag{flag}}
	holder := &mockHolder{current: store.Empty()}
	syncer := NewSyncer(repo, holder)

	err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify the store was updated
	current := holder.Current()
	if current.Len() != 1 {
		t.Fatalf("store length = %d, want 1", current.Len())
	}

	retrieved, ok := current.GetFlag("test")
	if !ok {
		t.Fatal("flag not found in store after sync")
	}
	if retrieved.Key != "test" {
		t.Fatalf("flag key = %q, want %q", retrieved.Key, "test")
	}
}

func TestSyncFailure(t *testing.T) {
	expectedErr := errors.New("database error")
	repo := &mockRepository{err: expectedErr}
	holder := &mockHolder{current: store.Empty()}
	syncer := NewSyncer(repo, holder)

	err := syncer.Sync(context.Background())
	if err == nil {
		t.Fatal("sync should have failed")
	}

	// Verify the store was NOT updated
	if holder.Current().Len() != 0 {
		t.Fatalf("store should be empty after failed sync, got %d flags", holder.Current().Len())
	}

	// Verify last error is recorded
	if syncer.LastError() != expectedErr {
		t.Fatalf("last error = %v, want %v", syncer.LastError(), expectedErr)
	}
}

func TestSyncPreservesOldStoreOnFailure(t *testing.T) {
	// Create an initial flag and store it
	initialFlag := domain.Flag{
		Key:     "initial",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}

	compiled, _ := eval.CompileFlag(initialFlag)
	initialStore := store.New(compiled)
	holder := &mockHolder{current: initialStore}

	// Create a syncer that will fail
	expectedErr := errors.New("database error")
	repo := &mockRepository{err: expectedErr}
	syncer := NewSyncer(repo, holder)

	err := syncer.Sync(context.Background())
	if err == nil {
		t.Fatal("sync should have failed")
	}

	// Verify the old store is still there
	if holder.Current().Len() != 1 {
		t.Fatalf("store length = %d, want 1", holder.Current().Len())
	}

	retrieved, ok := holder.Current().GetFlag("initial")
	if !ok {
		t.Fatal("initial flag was removed after failed sync")
	}
	if retrieved.Key != "initial" {
		t.Fatalf("flag key = %q, want %q", retrieved.Key, "initial")
	}
}

func TestSyncDeletedFlags(t *testing.T) {
	// Create initial store with a flag
	initialFlag := domain.Flag{
		Key:     "old_flag",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}

	compiled, _ := eval.CompileFlag(initialFlag)
	initialStore := store.New(compiled)
	holder := &mockHolder{current: initialStore}

	// Create a syncer with no flags (simulating deletion)
	repo := &mockRepository{flags: []domain.Flag{}}
	syncer := NewSyncer(repo, holder)

	err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify the old flag is gone
	if holder.Current().Len() != 0 {
		t.Fatalf("store should be empty after sync with no flags, got %d", holder.Current().Len())
	}

	_, ok := holder.Current().GetFlag("old_flag")
	if ok {
		t.Fatal("deleted flag should not exist in store")
	}
}

func TestSyncUpdatesVersion(t *testing.T) {
	// Create initial store with version 1
	initialFlag := domain.Flag{
		Key:     "test",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
		Version: 1,
	}

	compiled, _ := eval.CompileFlag(initialFlag)
	initialStore := store.New(compiled)
	holder := &mockHolder{current: initialStore}

	// Sync with updated flag (version 2)
	updatedFlag := domain.Flag{
		Key:     "test",
		Enabled: false,
		Default: "off",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
		Version: 2,
	}

	repo := &mockRepository{flags: []domain.Flag{updatedFlag}}
	syncer := NewSyncer(repo, holder)

	err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify the flag was updated
	retrieved, ok := holder.Current().GetFlag("test")
	if !ok {
		t.Fatal("flag not found after sync")
	}
	if retrieved.Version != 2 {
		t.Fatalf("flag version = %d, want 2", retrieved.Version)
	}
	if retrieved.Enabled != false {
		t.Fatalf("flag enabled = %v, want false", retrieved.Enabled)
	}
}

func TestSyncIncrementsGeneration(t *testing.T) {
	flag := domain.Flag{
		Key:     "test",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}

	repo := &mockRepository{flags: []domain.Flag{flag}}
	initialStore := store.Empty()
	holder := &mockHolder{current: initialStore}
	syncer := NewSyncer(repo, holder)

	initialGen := holder.Current().Generation()
	syncer.Sync(context.Background())
	afterGen := holder.Current().Generation()

	if afterGen != initialGen+1 {
		t.Fatalf("generation = %d, want %d", afterGen, initialGen+1)
	}
}

func TestLastSyncTracking(t *testing.T) {
	flag := domain.Flag{
		Key:     "test",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}

	repo := &mockRepository{flags: []domain.Flag{flag}}
	holder := &mockHolder{current: store.Empty()}
	syncer := NewSyncer(repo, holder)

	before := time.Now()
	syncer.Sync(context.Background())
	after := time.Now()

	lastSync := syncer.LastSync()
	if lastSync.Before(before) || lastSync.After(after) {
		t.Fatalf("last sync time = %v, should be between %v and %v", lastSync, before, after)
	}

	if syncer.LastError() != nil {
		t.Fatalf("last error should be nil after successful sync, got %v", syncer.LastError())
	}
}

func TestSyncSkipsInvalidFlags(t *testing.T) {
	// Create a valid flag and an invalid one
	validFlag := domain.Flag{
		Key:     "valid",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}

	// Invalid flag (weights don't sum to 100)
	invalidFlag := domain.Flag{
		Key:     "invalid",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 30},
			{Name: "off", Weight: 30},
		},
	}

	repo := &mockRepository{flags: []domain.Flag{validFlag, invalidFlag}}
	holder := &mockHolder{current: store.Empty()}
	syncer := NewSyncer(repo, holder)

	// Should succeed but only compile the valid flag
	err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync should succeed despite invalid flag: %v", err)
	}

	// Only the valid flag should be in the store
	if holder.Current().Len() != 1 {
		t.Fatalf("store length = %d, want 1 (only valid flag)", holder.Current().Len())
	}

	_, ok := holder.Current().GetFlag("valid")
	if !ok {
		t.Fatal("valid flag not found in store")
	}

	_, ok = holder.Current().GetFlag("invalid")
	if ok {
		t.Fatal("invalid flag should not be in store")
	}
}
