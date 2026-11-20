package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fakeLogin(t *testing.T, replies ...string) *Auth {
	t.Helper()
	i := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/device/code" {
			json.NewEncoder(w).Encode(map[string]any{"device_code": "d", "user_code": "ABCD-1234", "verification_uri": "https://github.com/login/device", "expires_in": 60, "interval": 1})
			return
		}
		reply := replies[min(i, len(replies)-1)]
		i++
		if reply == "token" {
			json.NewEncoder(w).Encode(map[string]any{"access_token": "secret"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"error": reply, "interval": 1})
	}))
	t.Cleanup(srv.Close)
	a := NewAuth(filepath.Join(t.TempDir(), "github.token"), nil)
	a.LoginBase = srv.URL
	return a
}

func waitDevice(t *testing.T, a *Auth) DeviceStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s := a.Device(); s != nil && s.State != DevicePending {
			return *s
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("device flow did not finish")
	return DeviceStatus{}
}

func TestDeviceFlow(t *testing.T) {
	t.Setenv("DORY_GITHUB_TOKEN", "")
	cases := []struct {
		replies []string
		state   string
	}{
		{[]string{"authorization_pending", "slow_down", "token"}, DeviceDone},
		{[]string{"expired_token"}, DeviceExpired},
		{[]string{"access_denied"}, DeviceDenied},
	}
	for _, c := range cases {
		a := fakeLogin(t, c.replies...)
		code, err := a.StartDevice(context.Background())
		if err != nil || code.UserCode != "ABCD-1234" {
			t.Fatalf("start: %v %+v", err, code)
		}
		if s := waitDevice(t, a); s.State != c.state {
			t.Fatalf("%v: got %+v", c.replies, s)
		}
		tok, src := a.Token(context.Background())
		if c.state == DeviceDone && (tok != "secret" || src != SourceUI) {
			t.Fatalf("token %q from %s", tok, src)
		}
		if c.state != DeviceDone && tok != "" {
			t.Fatalf("unexpected token %q", tok)
		}
	}
}

func TestTokenPriority(t *testing.T) {
	dir := t.TempDir()
	a := NewAuth(filepath.Join(dir, "github.token"), func(context.Context) string { return "gh" })
	check := func(want, source string) {
		t.Helper()
		a.Invalidate()
		if tok, src := a.Token(context.Background()); tok != want || src != source {
			t.Fatalf("got %q %s, want %q %s", tok, src, want, source)
		}
	}
	t.Setenv("DORY_GITHUB_TOKEN", "")
	check("gh", SourceGh)
	if err := os.WriteFile(a.File, []byte("ui\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	check("ui", SourceUI)
	t.Setenv("DORY_GITHUB_TOKEN", "env")
	check("env", SourceEnv)
	t.Setenv("DORY_GITHUB_TOKEN", "")
	if err := a.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	check("gh", SourceGh)
}
