package store

import (
	"sync"
	"sync/atomic"
	"testing"

	"launchdarkly/internal/domain"
	"launchdarkly/internal/eval"
)

func TestStoreGetFlag(t *testing.T) {
	source := flagWithKey("checkout")
	source.Version = 7
	compiled := mustCompile(t, source)
	store := New(compiled)

	got, ok := store.GetFlag("checkout")
	if !ok {
		t.Fatal("GetFlag() ok = false, want true")
	}
	if got.Key != "checkout" {
		t.Fatalf("GetFlag() key = %q, want %q", got.Key, "checkout")
	}
	if store.Version() != 7 {
		t.Fatalf("Version() = %d, want 7", store.Version())
	}
}

func TestStoreEvaluate(t *testing.T) {
	store := New(mustCompile(t, flagWithKey("checkout")))

	got, ok := store.Evaluate("checkout", &domain.Context{
		UserID:  "123",
		Country: "BR",
	})

	if !ok {
		t.Fatal("Evaluate() ok = false, want true")
	}
	if got != "on" {
		t.Fatalf("Evaluate() = %q, want %q", got, "on")
	}
}

func TestStoreEvaluateUnknownFlag(t *testing.T) {
	store := New(mustCompile(t, flagWithKey("checkout")))

	got, ok := store.Evaluate("missing", &domain.Context{UserID: "123"})

	if ok {
		t.Fatal("Evaluate() ok = true, want false")
	}
	if got != "" {
		t.Fatalf("Evaluate() = %q, want empty string", got)
	}
}

func TestStoreCopiesInputFlags(t *testing.T) {
	compiled := mustCompile(t, flagWithKey("checkout"))
	store := New(compiled)

	compiled.Enabled = false
	compiled.Default = "changed"

	got, ok := store.Evaluate("checkout", &domain.Context{
		UserID:  "123",
		Country: "BR",
	})

	if !ok {
		t.Fatal("Evaluate() ok = false, want true")
	}
	if got != "on" {
		t.Fatalf("Evaluate() = %q, want %q", got, "on")
	}
}

func TestHolderCurrentAndSwap(t *testing.T) {
	holder := NewHolder(New(mustCompile(t, flagWithKey("checkout"))))

	if holder.Current().Len() != 1 {
		t.Fatalf("Current().Len() = %d, want 1", holder.Current().Len())
	}
	if holder.Current().Generation() != 0 {
		t.Fatalf("Current().Generation() = %d, want 0", holder.Current().Generation())
	}

	holder.Swap(New(mustCompile(t, flagWithKey("search"))))

	if _, ok := holder.GetFlag("checkout"); ok {
		t.Fatal("GetFlag(checkout) ok = true, want false")
	}
	if _, ok := holder.GetFlag("search"); !ok {
		t.Fatal("GetFlag(search) ok = false, want true")
	}
	if holder.Current().Generation() != 1 {
		t.Fatalf("Current().Generation() = %d, want 1", holder.Current().Generation())
	}
}

func TestHolderReadersContinueDuringSwaps(t *testing.T) {
	holder := NewHolder(New(mustCompile(t, flagWithKey("checkout"))))
	ctx := &domain.Context{
		UserID:  "123",
		Country: "BR",
	}

	var failures atomic.Int64
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					variant, ok := holder.Evaluate("checkout", ctx)
					if !ok || variant == "" {
						failures.Add(1)
					}
				}
			}
		}()
	}

	for i := 0; i < 1000; i++ {
		flag := flagWithKey("checkout")
		flag.Version = i
		holder.Swap(New(mustCompile(t, flag)))
	}

	close(stop)
	wg.Wait()

	if failures.Load() != 0 {
		t.Fatalf("reader failures = %d, want 0", failures.Load())
	}
}

func BenchmarkHolderCurrent(b *testing.B) {
	holder := NewHolder(New(mustCompile(b, flagWithKey("checkout"))))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = holder.Current()
	}
}

func BenchmarkStoreLookup(b *testing.B) {
	store := New(mustCompile(b, flagWithKey("checkout")))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.GetFlag("checkout")
	}
}

func BenchmarkHolderEvaluate(b *testing.B) {
	holder := NewHolder(New(mustCompile(b, flagWithKey("checkout"))))
	ctx := &domain.Context{
		UserID:  "123",
		Country: "BR",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = holder.Evaluate("checkout", ctx)
	}
}

func mustCompile(tb testing.TB, flag domain.Flag) *eval.CompiledFlag {
	tb.Helper()

	compiled, err := eval.CompileFlag(flag)
	if err != nil {
		tb.Fatalf("CompileFlag() error = %v", err)
	}

	return compiled
}

func flagWithKey(key string) domain.Flag {
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
