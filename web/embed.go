// Package web embeds the compiled SPA and serves it with index.html fallback.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the embedded SPA; unknown paths fall back to index.html.
func Handler() http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if st, err := fs.Stat(root, name); err == nil && !st.IsDir() {
			// Vite names built assets by content hash, so they never change;
			// the rest (index.html, sw.js, icons) is revalidated each time.
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			files.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(root, "index.html"); err != nil {
			http.Error(w, "web assets not built (run npm run build in web/)", http.StatusNotFound)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		r.URL.Path = "/"
		files.ServeHTTP(w, r)
	})
}
