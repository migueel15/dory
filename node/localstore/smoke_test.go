package localstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/config"
	"github.com/migueel15/dory/internal/mcp"
	"github.com/migueel15/dory/node/localstore"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// capture returns what fn prints to stdout.
func capture(t *testing.T, fn func()) string {
	t.Helper()
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = old
	w.Close()
	out, _ := io.ReadAll(r)
	return strings.TrimSpace(string(out))
}

func TestSmokeAPIAndMCP(t *testing.T) {
	ctx := context.Background()
	store, err := localstore.Open(t.TempDir()+"/dory.db", localstore.Options{Device: "dev-a"})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	t.Setenv("DORY_PASSWORD", "una-clave-larga-123")
	capture(t, func() {
		if _, err = auth.Command(ctx, []string{"user", "create", "--username", "ana"}, store); err != nil {
			t.Fatal(err)
		}
	})
	token := capture(t, func() {
		if _, err = auth.Command(ctx, []string{"token", "create", "--username", "ana", "--name", "mcp", "--scopes", "read,write"}, store); err != nil {
			t.Fatal(err)
		}
	})

	cfg := config.Config{}
	mux := http.NewServeMux()
	api.Register(mux, store, nil, nil, nil)
	auth.Register(mux, store, cfg)
	mux.Handle("/mcp", mcp.Handler(store, nil))
	srv := httptest.NewServer(auth.Middleware(store, cfg)(mux))
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	call := func(method, path string, body any) (int, []byte) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			rd = bytes.NewReader(raw)
		}
		req, _ := http.NewRequest(method, srv.URL+path, rd)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, raw
	}

	if code, _ := call("GET", "/api/items", nil); code != 401 {
		t.Fatalf("anonymous list = %d", code)
	}
	if code, _ := call("POST", "/api/auth/login", map[string]string{"username": "ana", "password": "mala-clave-123"}); code != 401 {
		t.Fatalf("bad login = %d", code)
	}
	if code, body := call("POST", "/api/auth/login", map[string]string{"username": "ana", "password": "una-clave-larga-123"}); code != 200 {
		t.Fatalf("login = %d %s", code, body)
	}
	code, body := call("POST", "/api/items", map[string]any{"raw_text": "idea", "title": "Sincronización offline", "body_md": "primero local"})
	if code != 201 && code != 200 {
		t.Fatalf("create item = %d %s", code, body)
	}
	var item struct{ ID string }
	json.Unmarshal(body, &item)
	code, body = call("GET", "/api/items", nil)
	if code != 200 || !strings.Contains(string(body), item.ID) {
		t.Fatalf("list = %d %s", code, body)
	}
	code, body = call("GET", "/api/search?q=sincronizacion", nil)
	if code != 200 || !strings.Contains(string(body), item.ID) {
		t.Fatalf("search = %d %s", code, body)
	}

	// MCP with the bearer token.
	transport := &sdk.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearer{token}}}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "smoke", Version: "0"}, nil).Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "session_start", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("session_start = %+v, %v", res, err)
	}
	raw, _ := json.Marshal(res)
	if !strings.Contains(string(raw), "projects") {
		t.Fatalf("session_start result = %s", raw)
	}
	events, _ := store.ListEvents(ctx, 0, "item", 0)
	if len(events) != 1 {
		t.Fatalf("item events = %d", len(events))
	}
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}
