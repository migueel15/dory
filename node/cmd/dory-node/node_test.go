package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/service"
)

func startTest(t *testing.T, headless bool) (*Node, string) {
	t.Helper()
	dir := t.TempDir()
	n, err := Start(context.Background(), Options{DataDir: dir, T3URL: "http://127.0.0.1:1", Listen: "127.0.0.1:0", PeerPort: 0, Headless: headless, NoMDNS: true, DeviceName: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Stop)
	return n, dir
}

func do(t *testing.T, c *http.Client, method, url, token string, body any) (int, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func TestNodeSmoke(t *testing.T) {
	n, dir := startTest(t, false)
	base := "http://" + n.HTTPAddr

	// Identity file is private.
	if st, err := os.Stat(filepath.Join(dir, "identity.key")); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("identity.key: %v %v", st, err)
	}

	// The SPA is served (or, with no built assets, the embedded handler's notice)
	// and hands out the local session.
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	code, body := do(t, browser, "GET", base+"/", "", nil)
	if code == 200 && !strings.Contains(strings.ToLower(string(body)), "<html") {
		t.Fatalf("index: %d %s", code, body)
	}
	if code != 200 && !strings.Contains(string(body), "web assets not built") {
		t.Fatalf("index: %d %s", code, body)
	}
	if code, _ = do(t, &http.Client{}, "GET", base+"/api/auth/me", "", nil); code != 401 {
		t.Fatalf("me without session: %d", code)
	}
	if code, body = do(t, browser, "GET", base+"/api/auth/me", "", nil); code != 200 || !strings.Contains(string(body), "local") {
		t.Fatalf("me with auto session: %d %s", code, body)
	}

	// REST: create and list items with the session.
	_, body = do(t, browser, "POST", base+"/api/projects", "", map[string]any{"slug": "p", "name": "P", "status": "active", "stack": []string{}})
	var proj service.Project
	json.Unmarshal(body, &proj)
	if proj.ID == "" {
		t.Fatalf("create project: %s", body)
	}
	code, body = do(t, browser, "POST", base+"/api/items", "", map[string]any{"project_id": proj.ID, "title": "hello", "status": "in_progress", "refs": []string{}, "tags": []string{}})
	if code/100 != 2 {
		t.Fatalf("create item: %d %s", code, body)
	}
	code, body = do(t, browser, "GET", base+"/api/items", "", nil)
	if code != 200 || !strings.Contains(string(body), "hello") {
		t.Fatalf("list items: %d %s", code, body)
	}

	// MCP with the bearer token.
	tok, _ := os.ReadFile(n.TokenFile)
	token := strings.TrimSpace(string(tok))
	if code, _ = do(t, &http.Client{}, "POST", base+"/mcp", "", nil); code != 401 {
		t.Fatalf("mcp without token: %d", code)
	}
	mcpCall(t, base, token)

	// Own address record is published and the peer port is bound.
	a, ok, err := n.Store.AddressOf(context.Background(), n.Identity.DeviceID())
	if err != nil || !ok || a.Name != "test" {
		t.Fatalf("address: %v %v %v", a, ok, err)
	}
}

func mcpCall(t *testing.T, base, token string) {
	t.Helper()
	post := func(session string, payload string) (*http.Response, string) {
		req, _ := http.NewRequest("POST", base+"/mcp", strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	resp, out := post("", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
	if resp.StatusCode != 200 {
		t.Fatalf("initialize: %d %s", resp.StatusCode, out)
	}
	sid := resp.Header.Get("Mcp-Session-Id")
	post(sid, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	for _, tool := range []string{"session_start", "list_items"} {
		_, out = post(sid, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"`+tool+`","arguments":{}}}`)
		if !strings.Contains(out, `"result"`) || strings.Contains(out, `"isError":true`) {
			t.Fatalf("%s: %s", tool, out)
		}
	}
}

func TestHeadlessAndLoopbackOnly(t *testing.T) {
	n, _ := startTest(t, true)
	// Headless still has the API but no auto session on the index.
	c := &http.Client{}
	if code, _ := do(t, c, "GET", "http://"+n.HTTPAddr+"/", "", nil); code != 404 {
		t.Fatalf("headless index: %d", code)
	}
	_, err := Start(context.Background(), Options{DataDir: t.TempDir(), Listen: "0.0.0.0:0", NoMDNS: true})
	if err == nil {
		t.Fatal("non-loopback listen accepted")
	}
}

func TestT3InstanceOwnedByDevice(t *testing.T) {
	n, _ := startTest(t, true)
	ins, err := n.Store.T3Instances().List(context.Background(), service.Filter{Limit: 10})
	if err != nil || len(ins) != 1 || ins[0].BaseUrl != "http://127.0.0.1:1" || ins[0].ID != t3InstanceID(n.Identity.DeviceID()) {
		t.Fatalf("instances: %v %v", ins, err)
	}
}
