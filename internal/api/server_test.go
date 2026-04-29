package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"launchdarkly/internal/config"
	"launchdarkly/internal/db"
	"launchdarkly/internal/domain"
	"launchdarkly/internal/eval"
	flagstore "launchdarkly/internal/store"
)

// mockRepository is a test implementation of FlagRepository
type mockRepository struct {
	flags map[string]domain.Flag
}

func newMockRepository() *mockRepository {
	return &mockRepository{flags: make(map[string]domain.Flag)}
}

func (m *mockRepository) CreateFlag(ctx context.Context, flag domain.Flag) (domain.Flag, error) {
	if flag.Key == "" {
		return domain.Flag{}, domain.ValidationErrors{{Field: "key", Code: "required", Message: "key is required"}}
	}
	if _, exists := m.flags[flag.Key]; exists {
		return domain.Flag{}, domain.ValidationErrors{{Field: "key", Code: "duplicate", Message: "flag already exists"}}
	}
	flag.Version = 1
	if err := domain.ValidateFlag(flag); err != nil {
		return domain.Flag{}, err
	}
	m.flags[flag.Key] = flag
	return flag, nil
}

func (m *mockRepository) UpdateFlag(ctx context.Context, key string, flag domain.Flag) (domain.Flag, error) {
	existing, exists := m.flags[key]
	if !exists {
		return domain.Flag{}, db.ErrNotFound
	}
	flag.Key = key
	flag.Version = existing.Version + 1
	if err := domain.ValidateFlag(flag); err != nil {
		return domain.Flag{}, err
	}
	m.flags[key] = flag
	return flag, nil
}

func (m *mockRepository) DeleteFlag(ctx context.Context, key string) error {
	if _, exists := m.flags[key]; !exists {
		return db.ErrNotFound
	}
	delete(m.flags, key)
	return nil
}

func (m *mockRepository) GetFlag(ctx context.Context, key string) (domain.Flag, error) {
	flag, exists := m.flags[key]
	if !exists {
		return domain.Flag{}, db.ErrNotFound
	}
	return flag, nil
}

func (m *mockRepository) ListFlags(ctx context.Context) ([]domain.Flag, error) {
	flags := make([]domain.Flag, 0, len(m.flags))
	for _, flag := range m.flags {
		flags = append(flags, flag)
	}
	return flags, nil
}

func (m *mockRepository) LoadAllFlags(ctx context.Context) ([]domain.Flag, error) {
	return m.ListFlags(ctx)
}

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

// ===== Admin Control Plane API Tests =====

func TestCreateFlagSuccess(t *testing.T) {
	server := newTestServer()

	flag := domain.Flag{
		Key:     "test_flag",
		Enabled: true,
		Default: "control",
		Variants: []domain.Variant{
			{Name: "control", Weight: 50},
			{Name: "variant", Weight: 50},
		},
	}

	body := encodeJSON(flag)
	req := httptest.NewRequest(http.MethodPost, "/flags", body)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var created domain.Flag
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if created.Key != "test_flag" {
		t.Fatalf("key = %q, want %q", created.Key, "test_flag")
	}
	if created.Version != 1 {
		t.Fatalf("version = %d, want %d", created.Version, 1)
	}
}

func TestCreateFlagValidationError(t *testing.T) {
	server := newTestServer()

	// Flag with invalid weights (don't sum to 100)
	flag := domain.Flag{
		Key:     "bad_flag",
		Enabled: true,
		Default: "control",
		Variants: []domain.Variant{
			{Name: "control", Weight: 30},
			{Name: "variant", Weight: 30},
		},
	}

	body := encodeJSON(flag)
	req := httptest.NewRequest(http.MethodPost, "/flags", body)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if errResp.Error.Code != "validation_failed" {
		t.Fatalf("error code = %q, want %q", errResp.Error.Code, "validation_failed")
	}
	if len(errResp.Error.Details) == 0 {
		t.Fatal("expected validation details")
	}
}

func TestGetFlagsEmpty(t *testing.T) {
	server := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/flags", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var flags []domain.Flag
	if err := json.NewDecoder(rec.Body).Decode(&flags); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(flags) != 0 {
		t.Fatalf("len(flags) = %d, want 0", len(flags))
	}
}

func TestGetFlagsWithData(t *testing.T) {
	repo := newMockRepository()
	flag := domain.Flag{
		Key:     "test",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}
	repo.CreateFlag(context.Background(), flag)

	server := newTestServerWithRepo(repo)

	req := httptest.NewRequest(http.MethodGet, "/flags", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var flags []domain.Flag
	if err := json.NewDecoder(rec.Body).Decode(&flags); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(flags) != 1 {
		t.Fatalf("len(flags) = %d, want 1", len(flags))
	}
}

func TestGetFlagSuccess(t *testing.T) {
	repo := newMockRepository()
	flag := domain.Flag{
		Key:     "test",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}
	repo.CreateFlag(context.Background(), flag)

	server := newTestServerWithRepo(repo)

	req := httptest.NewRequest(http.MethodGet, "/flags/test", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var retrieved domain.Flag
	if err := json.NewDecoder(rec.Body).Decode(&retrieved); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if retrieved.Key != "test" {
		t.Fatalf("key = %q, want %q", retrieved.Key, "test")
	}
}

func TestGetFlagNotFound(t *testing.T) {
	server := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/flags/missing", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if errResp.Error.Code != "not_found" {
		t.Fatalf("error code = %q, want %q", errResp.Error.Code, "not_found")
	}
}

func TestUpdateFlagSuccess(t *testing.T) {
	repo := newMockRepository()
	flag := domain.Flag{
		Key:     "test",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}
	repo.CreateFlag(context.Background(), flag)

	server := newTestServerWithRepo(repo)

	// Update the flag
	updatedFlag := domain.Flag{
		Enabled: false,
		Default: "off",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}

	body := encodeJSON(updatedFlag)
	req := httptest.NewRequest(http.MethodPut, "/flags/test", body)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result domain.Flag
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if result.Version != 2 {
		t.Fatalf("version = %d, want %d", result.Version, 2)
	}
	if result.Enabled != false {
		t.Fatalf("enabled = %v, want false", result.Enabled)
	}
}

func TestUpdateFlagNotFound(t *testing.T) {
	server := newTestServer()

	updatedFlag := domain.Flag{
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}

	body := encodeJSON(updatedFlag)
	req := httptest.NewRequest(http.MethodPut, "/flags/missing", body)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if errResp.Error.Code != "not_found" {
		t.Fatalf("error code = %q, want %q", errResp.Error.Code, "not_found")
	}
}

func TestDeleteFlagSuccess(t *testing.T) {
	repo := newMockRepository()
	flag := domain.Flag{
		Key:     "test",
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}
	repo.CreateFlag(context.Background(), flag)

	server := newTestServerWithRepo(repo)

	req := httptest.NewRequest(http.MethodDelete, "/flags/test", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	// Verify it's deleted
	_, err := repo.GetFlag(context.Background(), "test")
	if err != db.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteFlagNotFound(t *testing.T) {
	server := newTestServer()

	req := httptest.NewRequest(http.MethodDelete, "/flags/missing", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if errResp.Error.Code != "not_found" {
		t.Fatalf("error code = %q, want %q", errResp.Error.Code, "not_found")
	}
}

// ===== Evaluation API Tests =====

func TestEvaluateSuccess(t *testing.T) {
	// Create a flag and compile it into the store
	flag := domain.Flag{
		Key:     "test_flag",
		Enabled: true,
		Default: "control",
		Variants: []domain.Variant{
			{Name: "control", Weight: 50},
			{Name: "variant", Weight: 50},
		},
	}

	compiled, err := eval.CompileFlag(flag)
	if err != nil {
		t.Fatalf("compile flag: %v", err)
	}

	store := flagstore.New(compiled)
	holder := flagstore.NewHolder(store)
	server := NewServer(config.Config{}, holder, newMockRepository()).Routes()

	req := EvaluateRequest{
		FlagKey: "test_flag",
		Context: domain.Context{UserID: "user123"},
	}

	body := encodeJSON(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/evaluate", body)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp EvaluateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Variant == "" {
		t.Fatal("variant should not be empty")
	}
}

func TestEvaluateFlagNotFound(t *testing.T) {
	holder := flagstore.NewHolder(flagstore.Empty())
	server := NewServer(config.Config{}, holder, newMockRepository()).Routes()

	req := EvaluateRequest{
		FlagKey: "missing_flag",
		Context: domain.Context{UserID: "user123"},
	}

	body := encodeJSON(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/evaluate", body)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if errResp.Error.Code != "flag_not_found" {
		t.Fatalf("error code = %q, want %q", errResp.Error.Code, "flag_not_found")
	}
}

func TestEvaluateMissingFlagKey(t *testing.T) {
	holder := flagstore.NewHolder(flagstore.Empty())
	server := NewServer(config.Config{}, holder, newMockRepository()).Routes()

	req := EvaluateRequest{
		FlagKey: "",
		Context: domain.Context{UserID: "user123"},
	}

	body := encodeJSON(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/evaluate", body)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if errResp.Error.Code != "missing_flag_key" {
		t.Fatalf("error code = %q, want %q", errResp.Error.Code, "missing_flag_key")
	}
}

func TestEvaluateWithRules(t *testing.T) {
	// Create a flag with rules that match the context
	flag := domain.Flag{
		Key:     "test_flag",
		Enabled: true,
		Default: "control",
		Variants: []domain.Variant{
			{Name: "control", Weight: 50},
			{Name: "variant", Weight: 50},
		},
		Rules: []domain.Rule{
			{
				Attribute: "country",
				Operator:  domain.OperatorEq,
				Values:    []string{"BR"},
				Variant:   "variant",
				Priority:  1,
			},
		},
	}

	compiled, err := eval.CompileFlag(flag)
	if err != nil {
		t.Fatalf("compile flag: %v", err)
	}

	store := flagstore.New(compiled)
	holder := flagstore.NewHolder(store)
	server := NewServer(config.Config{}, holder, newMockRepository()).Routes()

	// Request with matching context
	req := EvaluateRequest{
		FlagKey: "test_flag",
		Context: domain.Context{UserID: "user123", Country: "BR"},
	}

	body := encodeJSON(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/evaluate", body)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp EvaluateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Variant != "variant" {
		t.Fatalf("variant = %q, want %q", resp.Variant, "variant")
	}
}

func TestEvaluateDeterministic(t *testing.T) {
	// Test that evaluation is deterministic - same user/flag should always return same variant
	flag := domain.Flag{
		Key:     "test_flag",
		Enabled: true,
		Default: "control",
		Variants: []domain.Variant{
			{Name: "control", Weight: 50},
			{Name: "variant", Weight: 50},
		},
	}

	compiled, err := eval.CompileFlag(flag)
	if err != nil {
		t.Fatalf("compile flag: %v", err)
	}

	store := flagstore.New(compiled)
	holder := flagstore.NewHolder(store)
	server := NewServer(config.Config{}, holder, newMockRepository()).Routes()

	// Evaluate the same user multiple times
	variants := make([]string, 5)
	for i := 0; i < 5; i++ {
		req := EvaluateRequest{
			FlagKey: "test_flag",
			Context: domain.Context{UserID: "user123"},
		}

		body := encodeJSON(req)
		httpReq := httptest.NewRequest(http.MethodPost, "/evaluate", body)
		rec := httptest.NewRecorder()

		server.ServeHTTP(rec, httpReq)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var resp EvaluateResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		variants[i] = resp.Variant
	}

	// All evaluations should return the same variant
	for i := 1; i < len(variants); i++ {
		if variants[i] != variants[0] {
			t.Fatalf("variant %d = %q, want %q (not deterministic)", i, variants[i], variants[0])
		}
	}
}

func BenchmarkEvaluate(b *testing.B) {
	flag := domain.Flag{
		Key:     "test_flag",
		Enabled: true,
		Default: "control",
		Variants: []domain.Variant{
			{Name: "control", Weight: 50},
			{Name: "variant", Weight: 50},
		},
	}

	compiled, err := eval.CompileFlag(flag)
	if err != nil {
		b.Fatalf("compile flag: %v", err)
	}

	store := flagstore.New(compiled)
	holder := flagstore.NewHolder(store)
	server := NewServer(config.Config{}, holder, newMockRepository()).Routes()

	req := EvaluateRequest{
		FlagKey: "test_flag",
		Context: domain.Context{UserID: "user123"},
	}

	body := encodeJSON(req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body.Reset()
		json.NewEncoder(body).Encode(req)

		httpReq := httptest.NewRequest(http.MethodPost, "/evaluate", body)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, httpReq)
	}
}

// ===== Helper functions =====

func newTestServer() http.Handler {
	return NewServer(config.Config{}, flagstore.NewHolder(flagstore.Empty()), newMockRepository()).Routes()
}

func newTestServerWithRepo(repo FlagRepository) http.Handler {
	return NewServer(config.Config{}, flagstore.NewHolder(flagstore.Empty()), repo).Routes()
}

func encodeJSON(v any) *bytes.Buffer {
	buf := new(bytes.Buffer)
	json.NewEncoder(buf).Encode(v)
	return buf
}
