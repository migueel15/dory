// Package compress gzips HTTP responses whose content compresses well (JSON,
// HTML, JavaScript, CSS, SVG) when the client accepts it. Event streams and
// already encoded bodies pass through untouched.
package compress

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// minSize leaves tiny bodies alone: gzip framing would outweigh the saving.
const minSize = 1024

var pool = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
	return w
}}

// compressible reports whether a Content-Type is worth gzipping.
func compressible(ct string) bool {
	ct = strings.ToLower(ct)
	if strings.HasPrefix(ct, "text/event-stream") {
		return false
	}
	return strings.HasPrefix(ct, "text/") || strings.Contains(ct, "json") || strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "svg") || strings.Contains(ct, "wasm")
}

// Middleware gzips eligible responses.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || r.Header.Get("Upgrade") != "" || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		gw := &writer{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

// writer decides on the first write whether to compress: it buffers up to
// minSize bytes so small answers go out plain.
type writer struct {
	http.ResponseWriter
	status  int
	decided bool
	gz      *gzip.Writer
	buf     []byte
}

func (w *writer) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	// Bodiless or partial answers, or ones another layer already encoded, go out as they are.
	h := w.Header()
	if status < 200 || status == http.StatusNoContent || status == http.StatusNotModified || status == http.StatusPartialContent ||
		h.Get("Content-Encoding") != "" || !compressible(h.Get("Content-Type")) {
		w.passthrough()
	}
}

func (w *writer) passthrough() {
	if w.decided {
		return
	}
	w.decided = true
	w.flushHeader()
	if len(w.buf) > 0 {
		_, _ = w.ResponseWriter.Write(w.buf)
		w.buf = nil
	}
}

func (w *writer) flushHeader() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.status)
}

func (w *writer) startGzip() {
	w.decided = true
	h := w.Header()
	h.Del("Content-Length")
	h.Set("Content-Encoding", "gzip")
	if etag := h.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
		h.Set("ETag", "W/"+etag) // the encoded body differs byte for byte
	}
	w.flushHeader()
	w.gz = pool.Get().(*gzip.Writer)
	w.gz.Reset(w.ResponseWriter)
	if len(w.buf) > 0 {
		_, _ = w.gz.Write(w.buf)
		w.buf = nil
	}
}

func (w *writer) Write(p []byte) (int, error) {
	if w.status == 0 {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(p))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(p)
	}
	if w.decided {
		return w.ResponseWriter.Write(p)
	}
	w.buf = append(w.buf, p...)
	if len(w.buf) >= minSize {
		w.startGzip()
	}
	return len(p), nil
}

// Flush sends what is buffered; a streaming handler that flushes early is
// served plain so each chunk reaches the client at once.
func (w *writer) Flush() {
	if w.gz != nil {
		_ = w.gz.Flush()
	} else if !w.decided {
		if w.status == 0 {
			w.WriteHeader(http.StatusOK)
		}
		w.passthrough()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *writer) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *writer) close() {
	if w.gz != nil {
		_ = w.gz.Close()
		pool.Put(w.gz)
		w.gz = nil
		return
	}
	if !w.decided && (w.status != 0 || len(w.buf) > 0) {
		w.passthrough()
	}
}
