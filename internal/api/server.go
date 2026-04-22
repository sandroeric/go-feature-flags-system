package api

import (
	"net/http"
	"strconv"
	"time"

	"launchdarkly/internal/config"
	flagstore "launchdarkly/internal/store"
)

type Server struct {
	cfg       config.Config
	flags     *flagstore.Holder
	startedAt time.Time
}

func NewServer(cfg config.Config, flags *flagstore.Holder) *Server {
	if flags == nil {
		flags = flagstore.NewHolder(flagstore.Empty())
	}

	return &Server{
		cfg:       cfg,
		flags:     flags,
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
