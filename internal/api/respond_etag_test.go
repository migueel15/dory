package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSONETag(t *testing.T) {
	v := map[string]string{"diff": "a"}
	w := httptest.NewRecorder()
	writeJSONETag(w, httptest.NewRequest("GET", "/", nil), v)
	etag := w.Header().Get("ETag")
	if w.Code != http.StatusOK || etag == "" || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("first reply: %d etag=%q", w.Code, etag)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	writeJSONETag(w, r, v)
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Fatalf("same body: want 304, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	writeJSONETag(w, r, map[string]string{"diff": "b"})
	if w.Code != http.StatusOK {
		t.Fatalf("changed body: want 200, got %d", w.Code)
	}
}
