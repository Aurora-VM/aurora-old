package http

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// RegisterSPARoutes serves the compiled React frontend application for non-API routes.
func RegisterSPARoutes(r chi.Router, distDirs ...string) {
	if len(distDirs) == 0 {
		distDirs = []string{"web/dist", "./dist", "../web/dist"}
	}

	var validDistDir string
	for _, d := range distDirs {
		if stat, err := os.Stat(d); err == nil && stat.IsDir() {
			validDistDir, _ = filepath.Abs(d)
			break
		}
	}

	if validDistDir == "" {
		return
	}

	fs := http.FileServer(http.Dir(validDistDir))

	// Serve exact assets from dist if they exist
	r.Get("/assets/*", func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	})

	// Handle all non-API GET requests by serving SPA index.html
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/healthz") || strings.HasPrefix(path, "/readyz") {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "endpoint not found")
			return
		}

		filePath := filepath.Join(validDistDir, filepath.Clean(path))
		if stat, err := os.Stat(filePath); err == nil && !stat.IsDir() {
			http.ServeFile(w, r, filePath)
			return
		}

		// Fallback to index.html for SPA client-side routing
		indexPath := filepath.Join(validDistDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}

		http.NotFound(w, r)
	})
}
