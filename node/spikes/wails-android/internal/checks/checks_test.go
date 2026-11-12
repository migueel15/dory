package checks

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func newEnv(t *testing.T) *Env {
	t.Helper()
	return &Env{DataDir: t.TempDir()}
}

func TestSQLite(t *testing.T) {
	d, err := CheckSQLite(newEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(d)
}

func TestHandlerNoPort(t *testing.T) {
	env := newEnv(t)
	app := NewApp(env)
	h := app.Handler(fstest.MapFS{"index.html": {Data: []byte("hi")}})
	for _, p := range []string{"/", "/index.html", "/api/ping", "/api/checks/handler", "/api/checks/sqlite"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "https://wails.localhost"+p, nil))
		if rec.Code != 200 {
			t.Fatalf("%s -> %d", p, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "https://wails.localhost/api/checks/sqlite", nil))
	var r Result
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil || !r.OK {
		t.Fatalf("sqlite via handler: %v %+v", err, r)
	}
}

func TestMTLS(t *testing.T) {
	env := newEnv(t)
	m := NewMTLS(env, 47891)
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	d, err := m.SelfTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(d)

	// Same thing the desktop script does: client files from disk, CA from disk.
	dir := PKIDir(env)
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "client.pem"), filepath.Join(dir, "client.key"))
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := os.ReadFile(filepath.Join(dir, "ca.pem"))
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	c := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{cert}, ServerName: "dory-spike.local"}}}
	resp, err := c.Get("https://127.0.0.1:47891/ping")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal(resp.Status)
	}
}

func TestHeartbeatDetectsGap(t *testing.T) {
	h := NewHeartbeat(newEnv(t))
	h.Interval = 20 * time.Millisecond
	h.Tolerate = 50 * time.Millisecond
	h.Start()
	defer h.Stop()
	time.Sleep(150 * time.Millisecond)
	if _, err := h.Verdict(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Simulate a freeze by moving "last" into the past.
	h.mu.Lock()
	h.last = h.last.Add(-time.Second)
	h.mu.Unlock()
	time.Sleep(60 * time.Millisecond)
	if _, err := h.Verdict(); err == nil {
		t.Fatal("expected a gap error")
	}
}
