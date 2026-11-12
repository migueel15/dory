package nodeapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/migueel15/dory/node/peer"
)

func nodeCall(t *testing.T, c *http.Client, method, url string, csrf bool, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, url, strings.NewReader(string(b)))
	if csrf {
		req.Header.Set(csrfHeader, "1")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestNodeAPI(t *testing.T) {
	n, dir := startTest(t, true)
	base := "http://" + n.HTTPAddr
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}

	if code, _ := nodeCall(t, &http.Client{}, "GET", base+"/node/api/state", false, nil); code != 401 {
		t.Fatalf("state without session: %d", code)
	}
	// The page hands out the session; the API then answers.
	if resp, err := c.Get(base + "/node/"); err != nil || resp.StatusCode != 200 {
		t.Fatalf("page: %v %v", resp, err)
	}
	code, st := nodeCall(t, c, "GET", base+"/node/api/state", false, nil)
	if code != 200 || len(st["members"].([]any)) != 1 || st["members"].([]any)[0].(map[string]any)["self"] != true {
		t.Fatalf("state: %d %v", code, st)
	}
	// Mutations need the CSRF header.
	if code, _ = nodeCall(t, c, "PUT", base+"/node/api/device", false, map[string]string{"name": "x"}); code != 403 {
		t.Fatalf("no csrf header: %d", code)
	}
	if code, _ = nodeCall(t, c, "PUT", base+"/node/api/device", true, map[string]string{"name": " laptop "}); code != 200 {
		t.Fatalf("rename: %d", code)
	}
	if code, _ = nodeCall(t, c, "PUT", base+"/node/api/device", true, map[string]string{"name": ""}); code != 400 {
		t.Fatalf("empty name: %d", code)
	}
	if code, _ = nodeCall(t, c, "PUT", base+"/node/api/address", true, map[string]string{"manual": "nohostport"}); code != 400 {
		t.Fatalf("bad address: %d", code)
	}
	if code, _ = nodeCall(t, c, "PUT", base+"/node/api/address", true, map[string]string{"manual": "10.0.0.5:7710"}); code != 200 {
		t.Fatalf("address: %d", code)
	}
	_, st = nodeCall(t, c, "GET", base+"/node/api/state", false, nil)
	dev := st["device"].(map[string]any)
	if dev["name"] != "laptop" || dev["manual"] != "10.0.0.5:7710" {
		t.Fatalf("device: %v", dev)
	}

	// Pairing window: QR as PNG data URL, then closed.
	code, offer := nodeCall(t, c, "POST", base+"/node/api/pair", true, nil)
	if code != 200 || !strings.HasPrefix(offer["png"].(string), "data:image/png;base64,") || !n.Pairing.Open() {
		t.Fatalf("pair: %d %v", code, offer)
	}
	nodeCall(t, c, "DELETE", base+"/node/api/pair", true, nil)
	if n.Pairing.Open() {
		t.Fatal("window still open")
	}

	// Confirmation prompt.
	res := make(chan bool, 1)
	go func() { res <- n.prompts.ask("phone", "abcdef0123456789abcdef") }()
	var pid string
	for i := 0; i < 100 && pid == ""; i++ {
		_, st = nodeCall(t, c, "GET", base+"/node/api/state", false, nil)
		if p, ok := st["pending"].(map[string]any); ok {
			pid = p["id"].(string)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == "" {
		t.Fatal("no pending request")
	}
	if code, _ = nodeCall(t, c, "POST", base+"/node/api/pending", true, map[string]any{"id": "nope", "accept": true}); code != 404 {
		t.Fatalf("wrong id: %d", code)
	}
	nodeCall(t, c, "POST", base+"/node/api/pending", true, map[string]any{"id": pid, "accept": true})
	if !<-res {
		t.Fatal("not accepted")
	}

	// Revoke another member, never self.
	other, err := peer.LoadOrCreate(t.TempDir(), "bob")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = n.Group.Append(n.Identity, peer.ActionAdd, other.PublicKey(), "bob"); err != nil {
		t.Fatal(err)
	}
	_, st = nodeCall(t, c, "GET", base+"/node/api/state", false, nil)
	if len(st["members"].([]any)) != 2 {
		t.Fatalf("members: %v", st["members"])
	}
	if code, _ = nodeCall(t, c, "POST", base+"/node/api/members/"+n.Identity.DeviceID()+"/revoke", true, nil); code != 400 {
		t.Fatalf("revoke self: %d", code)
	}
	if code, _ = nodeCall(t, c, "POST", base+"/node/api/members/"+other.DeviceID()+"/revoke", true, nil); code != 200 {
		t.Fatalf("revoke: %d", code)
	}
	if n.Group.Allowed(other.PublicKey()) {
		t.Fatal("still allowed")
	}
	g, err := peer.LoadLog(dir + "/group.json")
	if err != nil || g.Allowed(other.PublicKey()) {
		t.Fatalf("not persisted: %v", err)
	}

	// Foreign Host header (DNS rebinding) is refused; in-process handler works.
	req, _ := http.NewRequest("GET", base+"/node/", nil)
	req.Host = "evil.example.com"
	if resp, _ := c.Do(req); resp.StatusCode != 403 {
		t.Fatalf("foreign host: %d", resp.StatusCode)
	}
	_ = context.Background
}

func TestNoListener(t *testing.T) {
	n, err := Start(context.Background(), Options{DataDir: t.TempDir(), NoMDNS: true, DeviceName: "x", ConfirmViaUI: true})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	if n.HTTPAddr != "" || n.Handler() == nil {
		t.Fatal("expected in-process only")
	}
	// The in-process handler signs the request in and ignores webview origins.
	h := n.Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://localhost/api/auth/me", nil))
	if rec.Code != 200 {
		t.Fatalf("me: %d", rec.Code)
	}
	req := httptest.NewRequest("POST", "http://localhost/api/projects", strings.NewReader(`{"slug":"p","name":"P","status":"active","stack":[]}`))
	req.Header.Set("Origin", "wails://localhost")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("post: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://localhost/node/api/state", nil))
	if rec.Code != 200 {
		t.Fatalf("node state: %d", rec.Code)
	}
}
