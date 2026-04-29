package metrics

import (
	"testing"
	"time"
)

func TestCollectorSnapshot(t *testing.T) {
	collector := NewCollector()

	collector.ObserveEvaluation(80*time.Microsecond, true, 3, 9)
	collector.ObserveEvaluation(2*time.Millisecond, false, 3, 9)
	collector.ObserveSync(true, 12*time.Millisecond, 4, 10)
	collector.ObserveSync(false, 0, 4, 10)

	snapshot := collector.Snapshot()
	if snapshot.EvaluationCount != 2 {
		t.Fatalf("evaluation count = %d, want 2", snapshot.EvaluationCount)
	}
	if snapshot.UnknownFlagCount != 1 {
		t.Fatalf("unknown flag count = %d, want 1", snapshot.UnknownFlagCount)
	}
	if snapshot.SyncSuccessCount != 1 {
		t.Fatalf("sync success count = %d, want 1", snapshot.SyncSuccessCount)
	}
	if snapshot.SyncFailureCount != 1 {
		t.Fatalf("sync failure count = %d, want 1", snapshot.SyncFailureCount)
	}
	if snapshot.StoreGeneration != 4 {
		t.Fatalf("store generation = %d, want 4", snapshot.StoreGeneration)
	}
	if snapshot.StoreVersion != 10 {
		t.Fatalf("store version = %d, want 10", snapshot.StoreVersion)
	}
	if snapshot.RefreshDuration != "12ms" {
		t.Fatalf("refresh duration = %q, want %q", snapshot.RefreshDuration, "12ms")
	}
	if snapshot.EvaluateLatency.Count != 2 {
		t.Fatalf("latency count = %d, want 2", snapshot.EvaluateLatency.Count)
	}
	if snapshot.EvaluateLatency.P50 == "0s" {
		t.Fatal("expected non-zero p50 latency")
	}
	if snapshot.EvaluateLatency.P95 == "0s" {
		t.Fatal("expected non-zero p95 latency")
	}
	if snapshot.EvaluateLatency.P99 == "0s" {
		t.Fatal("expected non-zero p99 latency")
	}
}
