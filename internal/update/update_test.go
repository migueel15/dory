package update

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/version"
)

// fakeReleases serves a manifest and a release with every asset of this system.
func fakeReleases(t *testing.T) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/contents/"):
			manifest, _ := json.Marshal(map[string]string{"tag": "nightly-1", "sha": "0123456789abcdef"})
			json.NewEncoder(w).Encode(map[string]string{"content": base64.StdEncoding.EncodeToString(manifest)})
		case strings.HasPrefix(r.URL.Path, "/releases/"):
			names := append(desktopAssets(), "dory-"+runtime.GOOS+"-"+runtime.GOARCH+".exe", "dory-"+runtime.GOOS+"-"+runtime.GOARCH)
			var assets []map[string]string
			for _, n := range names {
				n = strings.ReplaceAll(n, "*", "Dory-Setup")
				assets = append(assets, map[string]string{"name": n, "browser_download_url": srv.URL + "/files/" + n})
			}
			json.NewEncoder(w).Encode(map[string]any{"assets": assets})
		default:
			w.Write([]byte("bin"))
		}
	}))
	t.Cleanup(srv.Close)
	prevAPI, prevChannel, prevReplace := api, version.Channel, replaceCore
	api, version.Channel = srv.URL, "nightly"
	t.Cleanup(func() { api, version.Channel, replaceCore = prevAPI, prevChannel, prevReplace })
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
}

func TestRunInAppDoesNotReplace(t *testing.T) {
	fakeReleases(t)
	t.Setenv(DesktopEnv, "1")
	replaced, restarted := false, false
	replaceCore = func(string) error { replaced = true; return nil }
	u := &Updater{DataDir: t.TempDir(), Restart: func() { restarted = true }}
	if err := u.run(context.Background(), Options{}); err != nil {
		t.Fatal(err)
	}
	if replaced || restarted {
		t.Fatalf("replaced=%v restarted=%v; the app brings the core", replaced, restarted)
	}
}

func TestRunServiceReplaceFailureDoesNotRestart(t *testing.T) {
	fakeReleases(t)
	t.Setenv(DesktopEnv, "")
	restarted := false
	replaceCore = func(string) error { return errors.New("locked") }
	u := &Updater{DataDir: t.TempDir(), Restart: func() { restarted = true }}
	if err := u.run(context.Background(), Options{}); err == nil || restarted {
		t.Fatalf("err=%v restarted=%v", err, restarted)
	}
}
