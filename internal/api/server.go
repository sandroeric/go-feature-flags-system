package api

import (
	"net/http"
	"time"

	"launchdarkly/internal/config"
)

type Server struct {
	cfg       config.Config
	startedAt time.Time
}

func NewServer(cfg config.Config) *Server {
	return &Server{
		cfg:       cfg,
		startedAt: time.Now().UTC(),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleNotFound)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":     "ok",
		"service":    "launchdarkly",
		"started_at": s.startedAt.Format(time.RFC3339),
	})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "route not found")
}
