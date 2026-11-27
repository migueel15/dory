// Package web embeds the compiled SPA and serves it with index.html fallback.
package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

//go:embed all:dist
var dist embed.FS

const minGzipBytes = 1024

type embeddedFile struct {
	data        []byte
	etag        string
	contentType string
	compressible bool

	gzOnce sync.Once
	gzData []byte
}

func (f *embeddedFile) gzipped() []byte {
	f.gzOnce.Do(func() {
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return
		}
		if _, err := zw.Write(f.data); err != nil {
			return
		}
		if err := zw.Close(); err != nil {
			return
		}
		if buf.Len() < len(f.data) {
			f.gzData = buf.Bytes()
		}
	})
	return f.gzData
}

func isCompressibleExt(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".js", ".mjs", ".css", ".html", ".svg", ".json":
		return true
	default:
		return false
	}
}

func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(part)
		if tag == "*" || tag == etag || strings.TrimPrefix(tag, "W/") == etag {
			return true
		}
	}
	return false
}

func acceptsGzip(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Accept-Encoding")), "gzip")
}

// Handler serves the embedded SPA; unknown paths fall back to index.html.
func Handler() http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return handlerForFS(root)
}

func handlerForFS(root fs.FS) http.Handler {
	cache := make(map[string]*embeddedFile)
	_ = fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		ct := mime.TypeByExtension(path.Ext(p))
		if ct == "" {
			ct = http.DetectContentType(data)
		}
		cache[p] = &embeddedFile{
			data:         data,
			etag:         `"` + hex.EncodeToString(sum[:8]) + `"`,
			contentType:  ct,
			compressible: len(data) >= minGzipBytes && isCompressibleExt(p),
		}
		return nil
	})

	serveFile := func(w http.ResponseWriter, r *http.Request, name string, f *embeddedFile) {
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Set("ETag", f.etag)
		if f.compressible {
			w.Header().Add("Vary", "Accept-Encoding")
		}
		if etagMatches(r.Header.Get("If-None-Match"), f.etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", f.contentType)
		body := f.data
		if f.compressible && acceptsGzip(r) && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			if gz := f.gzipped(); len(gz) > 0 {
				w.Header().Set("Content-Encoding", "gzip")
				body = gz
			}
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if f, ok := cache[name]; ok {
			serveFile(w, r, name, f)
			return
		}
		// A missing built file must not answer with the page: a service worker
		// would keep that HTML as the asset.
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
		idx, ok := cache["index.html"]
		if !ok {
			http.Error(w, "web assets not built (run npm run build in web/)", http.StatusNotFound)
			return
		}
		serveFile(w, r, "index.html", idx)
	})
}
