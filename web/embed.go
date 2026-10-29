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
		r.URL.Path = "/"
		files.ServeHTTP(w, r)
	})
}
