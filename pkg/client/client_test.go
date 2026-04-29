package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"launchdarkly/internal/domain"
)

func TestRemoteEval(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/evaluate" {
			t.Fatalf("path = %s, want /evaluate", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}

		var req evaluateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.FlagKey != "checkout" {
			t.Fatalf("flag key = %q, want checkout", req.FlagKey)
		}
		if req.Context.UserID != "123" {
			t.Fatalf("user ID = %q, want 123", req.Context.UserID)
		}

		_ = json.NewEncoder(w).Encode(evaluateResponse{Variant: "treatment"})
	}))
	defer server.Close()

	client, err := New(Config{
		Mode:    ModeRemote,
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	variant, err := client.Eval("checkout", domain.Context{UserID: "123"})
	if err != nil {
		t.Fatalf("Eval() error = %v", err)
	}
	if variant != "treatment" {
		t.Fatalf("variant = %q, want treatment", variant)
	}
}

func TestRemoteEvalFlagNotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"code":    "flag_not_found",
				"message": "flag not found",
			},
		})
	}))
	defer server.Close()

	client, err := New(Config{
		Mode:    ModeRemote,
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Eval("missing", domain.Context{UserID: "123"})
	if !errors.Is(err, ErrFlagNotFound) {
		t.Fatalf("Eval() error = %v, want ErrFlagNotFound", err)
	}
}

func TestLocalEvalUsesFlagsEndpointOnly(t *testing.T) {
	var evaluateCalls atomic.Int32
	var flagsCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/flags":
			flagsCalls.Add(1)
			_ = json.NewEncoder(w).Encode([]domain.Flag{
				testFlag("checkout", "control", "treatment"),
			})
		case "/evaluate":
			evaluateCalls.Add(1)
			http.Error(w, "should not be called in local mode", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(Config{
		Mode:         ModeLocal,
		BaseURL:      server.URL,
		SyncInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()

	variant, err := client.Eval("checkout", domain.Context{UserID: "123", Country: "BR"})
	if err != nil {
		t.Fatalf("Eval() error = %v", err)
	}
	if variant != "treatment" {
		t.Fatalf("variant = %q, want treatment", variant)
	}
	if evaluateCalls.Load() != 0 {
		t.Fatalf("/evaluate calls = %d, want 0", evaluateCalls.Load())
	}
	if flagsCalls.Load() == 0 {
		t.Fatal("/flags should be called during local sync")
	}
}

func TestLocalSyncFailureKeepsPreviousStore(t *testing.T) {
	var failFlags atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/flags" {
			http.NotFound(w, r)
			return
		}
		if failFlags.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode([]domain.Flag{
			testFlag("checkout", "control", "treatment"),
		})
	}))
	defer server.Close()

	client, err := New(Config{
		Mode:         ModeLocal,
		BaseURL:      server.URL,
		SyncInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()

	before, err := client.Eval("checkout", domain.Context{UserID: "123", Country: "BR"})
	if err != nil {
		t.Fatalf("Eval() before failure error = %v", err)
	}

	failFlags.Store(true)
	if err := client.Sync(context.Background()); err == nil {
		t.Fatal("Sync() error = nil, want error")
	}

	after, err := client.Eval("checkout", domain.Context{UserID: "123", Country: "BR"})
	if err != nil {
		t.Fatalf("Eval() after failure error = %v", err)
	}
	if after != before {
		t.Fatalf("variant after failed sync = %q, want %q", after, before)
	}
}

func TestLocalBackgroundSyncRefreshesStore(t *testing.T) {
	var response atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/flags" {
			http.NotFound(w, r)
			return
		}
		if response.Load() == 0 {
			_ = json.NewEncoder(w).Encode([]domain.Flag{
				testFlag("checkout", "control", "treatment"),
			})
			return
		}

		_ = json.NewEncoder(w).Encode([]domain.Flag{
			{
				Key:     "checkout",
				Enabled: true,
				Default: "control",
				Variants: []domain.Variant{
					{Name: "control", Weight: 100},
					{Name: "treatment", Weight: 0},
				},
				Version: 2,
			},
		})
	}))
	defer server.Close()

	client, err := New(Config{
		Mode:         ModeLocal,
		BaseURL:      server.URL,
		SyncInterval: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()

	response.Store(1)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		variant, err := client.Eval("checkout", domain.Context{UserID: "123", Country: "BR"})
		if err == nil && variant == "control" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("local background sync did not refresh the store")
}

func testFlag(key, defaultVariant, targetedVariant string) domain.Flag {
	return domain.Flag{
		Key:     key,
		Enabled: true,
		Default: defaultVariant,
		Variants: []domain.Variant{
			{Name: defaultVariant, Weight: 50},
			{Name: targetedVariant, Weight: 50},
		},
		Rules: []domain.Rule{
			{
				Attribute: "country",
				Operator:  domain.OperatorEq,
				Values:    []string{"BR"},
				Variant:   targetedVariant,
				Priority:  1,
			},
		},
		Version: 1,
	}
}
