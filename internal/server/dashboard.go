package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// The admin dashboard is a Vite/React build copied into dashboard/dist and
// embedded in the binary, so `go build` ships one artifact (ADR-0018, ADR-0019).
// Build it with: cd web && npm run build
//
//go:embed all:dashboard/dist
var dashboardFS embed.FS

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(dashboardFS, "dashboard/dist")
	if err != nil {
		http.Error(w, "dashboard assets missing", http.StatusInternalServerError)
		return
	}
	// Vite fingerprints everything under /assets/, so those are safe to cache
	// forever. index.html must revalidate so a redeploy is picked up.
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		w.Header().Set("cache-control", "public, max-age=31536000, immutable")
		http.FileServer(http.FS(sub)).ServeHTTP(w, r)
		return
	}
	w.Header().Set("cache-control", "no-cache")
	http.ServeFileFS(w, r, sub, "index.html")
}
