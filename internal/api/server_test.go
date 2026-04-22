package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"launchdarkly/internal/config"
	flagstore "launchdarkly/internal/store"
)

func TestHealth(t *testing.T) {
	server := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %q, want %q", body["status"], "ok")
	}
	if body["service"] != "launchdarkly" {
		t.Fatalf("service body = %q, want %q", body["service"], "launchdarkly")
	}
	if body["store_generation"] != "0" {
		t.Fatalf("store_generation body = %q, want %q", body["store_generation"], "0")
	}
}

func TestHealthRejectsUnsupportedMethod(t *testing.T) {
	server := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != "method_not_allowed" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "method_not_allowed")
	}
}

func TestNotFoundUsesStructuredError(t *testing.T) {
	server := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "not_found")
	}
}

func newTestServer() http.Handler {
	return NewServer(config.Config{}, flagstore.NewHolder(flagstore.Empty())).Routes()
}
