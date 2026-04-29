package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"launchdarkly/internal/domain"
)

func TestWriteValidationErrorMapsDomainValidationErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	err := domain.ValidationErrors{
		{Field: "key", Code: "required", Message: "flag key is required"},
	}

	if ok := writeValidationError(rec, err); !ok {
		t.Fatal("writeValidationError() = false, want true")
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Error.Code != "validation_failed" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "validation_failed")
	}
	if len(body.Error.Details) != 1 {
		t.Fatalf("details length = %d, want 1", len(body.Error.Details))
	}
	if body.Error.Details[0].Field != "key" {
		t.Fatalf("details[0].field = %q, want %q", body.Error.Details[0].Field, "key")
	}
}

func TestWriteValidationErrorIgnoresOtherErrors(t *testing.T) {
	rec := httptest.NewRecorder()

	if ok := writeValidationError(rec, errors.New("boom")); ok {
		t.Fatal("writeValidationError() = true, want false")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want default recorder status %d", rec.Code, http.StatusOK)
	}
}
