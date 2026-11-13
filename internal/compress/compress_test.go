package compress

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(h http.HandlerFunc, enc string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if enc != "" {
		r.Header.Set("Accept-Encoding", enc)
	}
	w := httptest.NewRecorder()
	Middleware(h).ServeHTTP(w, r)
	return w
}

func TestGzipsLargeJSON(t *testing.T) {
	body := strings.Repeat(`{"a":"b"},`, 500)
	w := serve(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"x"`)
		_, _ = io.WriteString(w, body)
	}, "gzip, br")
	if w.Header().Get("Content-Encoding") != "gzip" || w.Header().Get("ETag") != `W/"x"` {
		t.Fatalf("headers = %v", w.Header())
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != body {
		t.Fatalf("body mismatch")
	}
}

func TestLeavesSmallStreamsAndUnaccepted(t *testing.T) {
	small := serve(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}, "gzip")
	if small.Header().Get("Content-Encoding") != "" || small.Body.String() != `{}` {
		t.Fatalf("small = %v %q", small.Header(), small.Body.String())
	}
	stream := serve(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, strings.Repeat("data: x\n\n", 300))
		w.(http.Flusher).Flush()
	}, "gzip")
	if stream.Header().Get("Content-Encoding") != "" || !stream.Flushed {
		t.Fatalf("stream = %v", stream.Header())
	}
	plain := serve(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, strings.Repeat("x", 5000))
	}, "")
	if plain.Header().Get("Content-Encoding") != "" || plain.Body.Len() != 5000 {
		t.Fatalf("plain = %v", plain.Header())
	}
	notModified := serve(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotModified)
	}, "gzip")
	if notModified.Code != http.StatusNotModified || notModified.Header().Get("Content-Encoding") != "" {
		t.Fatalf("304 = %d %v", notModified.Code, notModified.Header())
	}
}
