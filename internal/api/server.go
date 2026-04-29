package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"launchdarkly/internal/config"
	"launchdarkly/internal/db"
	"launchdarkly/internal/domain"
	"launchdarkly/internal/metrics"
	flagstore "launchdarkly/internal/store"
)

// FlagRepository defines the interface for flag persistence operations.
type FlagRepository interface {
	CreateFlag(context.Context, domain.Flag) (domain.Flag, error)
	UpdateFlag(context.Context, string, domain.Flag) (domain.Flag, error)
	DeleteFlag(context.Context, string) error
	GetFlag(context.Context, string) (domain.Flag, error)
	ListFlags(context.Context) ([]domain.Flag, error)
	LoadAllFlags(context.Context) ([]domain.Flag, error)
}

// EvaluateRequest is the request body for POST /evaluate
type EvaluateRequest struct {
	FlagKey string         `json:"flag_key"`
	Context domain.Context `json:"context"`
}

// EvaluateResponse is the response body for POST /evaluate
type EvaluateResponse struct {
	Variant string `json:"variant"`
}

type Server struct {
	cfg       config.Config
	flags     *flagstore.Holder
	repo      FlagRepository
	startedAt time.Time
	refreshFn func(context.Context) error // Called after writes to refresh data plane
	metrics   *metrics.Collector
}

func NewServer(cfg config.Config, flags *flagstore.Holder, repo FlagRepository) *Server {
	if flags == nil {
		flags = flagstore.NewHolder(flagstore.Empty())
	}

	s := &Server{
		cfg:       cfg,
		flags:     flags,
		repo:      repo,
		startedAt: time.Now().UTC(),
		metrics:   metrics.NewCollector(),
	}

	// Default refresh function (no-op for now, Phase 8 will implement)
	s.refreshFn = func(ctx context.Context) error {
		return nil
	}

	return s
}

// SetRefreshFunc allows Phase 8 (Sync) to inject the actual refresh implementation.
func (s *Server) SetRefreshFunc(fn func(context.Context) error) {
	if fn != nil {
		s.refreshFn = fn
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("GET /app", s.handleAdminIndex)
	mux.Handle("GET /app/", http.StripPrefix("/app/", adminUIHandler()))
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.HandleFunc("GET /flags", s.handleGetFlags)
	mux.HandleFunc("POST /flags", s.handleCreateFlag)
	mux.HandleFunc("GET /flags/{key}", s.handleGetFlag)
	mux.HandleFunc("PUT /flags/{key}", s.handleUpdateFlag)
	mux.HandleFunc("DELETE /flags/{key}", s.handleDeleteFlag)
	mux.HandleFunc("POST /evaluate", s.handleEvaluate)
	mux.HandleFunc("/", s.handleNotFound)
	return mux
}

func (s *Server) Metrics() *metrics.Collector {
	return s.metrics
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	current := s.flags.Current()
	writeJSON(w, http.StatusOK, map[string]string{
		"status":           "ok",
		"service":          "launchdarkly",
		"started_at":       s.startedAt.Format(time.RFC3339),
		"store_generation": strconv.FormatUint(current.Generation(), 10),
		"store_version":    strconv.Itoa(current.Version()),
		"flag_count":       strconv.Itoa(current.Len()),
	})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "route not found")
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, s.metrics.Snapshot())
}

// handleCreateFlag handles POST /flags
func (s *Server) handleCreateFlag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	var flag domain.Flag
	if err := parseJSON(r, &flag); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	flag.Version = 0 // Will be set to 1 by repository
	created, err := s.repo.CreateFlag(r.Context(), flag)
	if writeValidationError(w, err) {
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create_failed", "failed to create flag")
		return
	}

	// Trigger data-plane refresh
	if err := s.refreshFn(r.Context()); err != nil {
		// Log but don't fail the request - refresh will be retried by polling
		writeError(w, http.StatusInternalServerError, "refresh_failed", "flag created but refresh failed")
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// handleUpdateFlag handles PUT /flags/:key
func (s *Server) handleUpdateFlag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	key := strings.TrimSpace(r.PathValue("key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, "invalid_key", "flag key is required")
		return
	}

	var flag domain.Flag
	if err := parseJSON(r, &flag); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	updated, err := s.repo.UpdateFlag(r.Context(), key, flag)
	if err == db.ErrNotFound {
		writeError(w, http.StatusNotFound, "not_found", "flag not found")
		return
	}
	if writeValidationError(w, err) {
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update_failed", "failed to update flag")
		return
	}

	// Trigger data-plane refresh
	if err := s.refreshFn(r.Context()); err != nil {
		// Log but don't fail the request
		writeError(w, http.StatusInternalServerError, "refresh_failed", "flag updated but refresh failed")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteFlag handles DELETE /flags/:key
func (s *Server) handleDeleteFlag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	key := strings.TrimSpace(r.PathValue("key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, "invalid_key", "flag key is required")
		return
	}

	err := s.repo.DeleteFlag(r.Context(), key)
	if err == db.ErrNotFound {
		writeError(w, http.StatusNotFound, "not_found", "flag not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", "failed to delete flag")
		return
	}

	// Trigger data-plane refresh
	if err := s.refreshFn(r.Context()); err != nil {
		// Log but don't fail the request
		writeError(w, http.StatusInternalServerError, "refresh_failed", "flag deleted but refresh failed")
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

// handleGetFlag handles GET /flags/:key
func (s *Server) handleGetFlag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	key := strings.TrimSpace(r.PathValue("key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, "invalid_key", "flag key is required")
		return
	}

	flag, err := s.repo.GetFlag(r.Context(), key)
	if err == db.ErrNotFound {
		writeError(w, http.StatusNotFound, "not_found", "flag not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get_failed", "failed to get flag")
		return
	}

	writeJSON(w, http.StatusOK, flag)
}

// handleGetFlags handles GET /flags
func (s *Server) handleGetFlags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	flags, err := s.repo.ListFlags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", "failed to list flags")
		return
	}

	if flags == nil {
		flags = []domain.Flag{}
	}

	writeJSON(w, http.StatusOK, flags)
}

// handleEvaluate handles POST /evaluate - fast in-memory evaluation using data plane
func (s *Server) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	start := time.Now()

	var req EvaluateRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if strings.TrimSpace(req.FlagKey) == "" {
		writeError(w, http.StatusBadRequest, "missing_flag_key", "flag_key is required")
		return
	}

	// Evaluate using only the in-memory store (data plane)
	current := s.flags.Current()
	variant, found := current.Evaluate(req.FlagKey, &req.Context)
	s.metrics.ObserveEvaluation(time.Since(start), found, current.Generation(), current.Version())
	if !found {
		writeError(w, http.StatusNotFound, "flag_not_found", "flag not found")
		return
	}

	writeJSON(w, http.StatusOK, EvaluateResponse{Variant: variant})
}
