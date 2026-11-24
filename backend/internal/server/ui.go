package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// uiHandler serves the embedded single-page app with history fallback.
func (a *App) uiHandler() http.Handler {
	if a.ui == nil {
		return http.NotFoundHandler()
	}
	files := http.FileServerFS(a.ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeOAuthError(w, http.StatusNotFound, "not_found", "No route for "+r.Method+" "+r.URL.Path)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		for _, api := range []string{"public/", "portal-api/", "admin-api/", "oauth/"} {
			if strings.HasPrefix(p+"/", api) {
				writeOAuthError(w, http.StatusNotFound, "not_found", "No route for "+r.Method+" "+r.URL.Path)
				return
			}
		}
		if p != "" {
			if st, err := fs.Stat(a.ui, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback
		b, err := fs.ReadFile(a.ui, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(b)
	})
}
