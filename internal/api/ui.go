package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed adminui/*
var adminUIFiles embed.FS

func adminUIHandler() http.Handler {
	sub, err := fs.Sub(adminUIFiles, "adminui")
	if err != nil {
		panic(err)
	}

	return http.FileServer(http.FS(sub))
}

func (s *Server) handleAdminIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	http.ServeFileFS(w, r, adminUIFiles, "adminui/index.html")
}
