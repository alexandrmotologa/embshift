package server

import (
	"io/fs"
	"net/http"
	"strings"
)

// ServeStaticSPA configures an HTTP handler that serves embedded static assets with SPA index.html fallback.
func ServeStaticSPA(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		// Check if exact file exists
		if path != "" {
			if f, err := staticFS.Open(path); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// Fallback to index.html for SPA client-side routing
		indexBytes, err := fs.ReadFile(staticFS, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexBytes)
	})
}
