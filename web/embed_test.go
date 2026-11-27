package web

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedHandlerETagAndGzip(t *testing.T) {
	largeJS := strings.Repeat("console.log('hello dory');\n", 100)
	fsys := fstest.MapFS{
		"index.html":          {Data: []byte("<!doctype html><html><body>dory</body></html>")},
		"assets/app-12345.js": {Data: []byte(largeJS)},
	}
	h := handlerForFS(fsys)

	// 1. Gzip + ETag + immutable Cache-Control on large JS asset.
	req := httptest.NewRequest(http.MethodGet, "/assets/app-12345.js", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag header")
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("Cache-Control = %q, want immutable", got)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	plain, err := io.ReadAll(zr)
	zr.Close()
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	if string(plain) != largeJS {
		t.Fatalf("decompressed body mismatch")
	}

	// 2. If-None-Match returns 304 Not Modified with empty body.
	req304 := httptest.NewRequest(http.MethodGet, "/assets/app-12345.js", nil)
	req304.Header.Set("If-None-Match", etag)
	rec304 := httptest.NewRecorder()
	h.ServeHTTP(rec304, req304)

	if rec304.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", rec304.Code)
	}
	if rec304.Body.Len() != 0 {
		t.Fatalf("304 body should be empty, got %d bytes", rec304.Body.Len())
	}

	// 3. SPA fallback serves index.html with ETag and no-cache.
	reqSPA := httptest.NewRequest(http.MethodGet, "/threads/123", nil)
	recSPA := httptest.NewRecorder()
	h.ServeHTTP(recSPA, reqSPA)
	if recSPA.Code != http.StatusOK {
		t.Fatalf("SPA fallback status = %d, want 200", recSPA.Code)
	}
	if recSPA.Header().Get("ETag") == "" {
		t.Fatal("SPA fallback missing ETag")
	}
	if got := recSPA.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("SPA fallback Cache-Control = %q, want no-cache", got)
	}
}

func TestEmbeddedHandlerMissingAssetIsNotFound(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><html><body>dory</body></html>")},
	}
	h := handlerForFS(fsys)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/old-build.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d, want 404", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "dory") {
		t.Fatalf("SPA route status = %d, want index.html", rec.Code)
	}
}
