package main

import (
	"log"
	"net/http"
)

// trace logs every request the window makes (DORY_DESKTOP_TRACE=1): method,
// URL with query, body length and response status. It exists to verify what
// the Wails AssetServer forwards to the handler.
func trace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		log.Printf("req %s %s len=%d ct=%q host=%s origin=%q -> %d", r.Method, r.URL.RequestURI(), r.ContentLength, r.Header.Get("Content-Type"), r.Host, r.Header.Get("Origin"), sw.status)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(c int) { s.status = c; s.ResponseWriter.WriteHeader(c) }
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
