package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"launchdarkly/internal/domain"
)

type ErrorResponse struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details []APIFieldError `json:"details,omitempty"`
}

type APIFieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, ErrorResponse{
		Error: APIError{
			Code:    code,
			Message: message,
		},
	})
}

func writeValidationError(w http.ResponseWriter, err error) bool {
	var validationErrors domain.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return false
	}

	details := make([]APIFieldError, 0, len(validationErrors))
	for _, validationError := range validationErrors {
		details = append(details, APIFieldError{
			Field:   validationError.Field,
			Code:    validationError.Code,
			Message: validationError.Message,
		})
	}

	writeJSON(w, http.StatusBadRequest, ErrorResponse{
		Error: APIError{
			Code:    "validation_failed",
			Message: "request validation failed",
			Details: details,
		},
	})

	return true
}
