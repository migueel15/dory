// Command dory-android is the Dory local-first node as an Android app.
//
// The webview loads a tiny bootstrap page from Wails' in-process asset server
// (GET only, no query: all that server can do on Android). The page reads the
// phone's network interfaces from the Java side, hands them to Go and gets back
// the address of the node's loopback shell (127.0.0.1, random port, cookie
// gate; see nodeapp.Shell), where the SPA and the /node/ pages run as on any
// browser. See README.md.
package main

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/migueel15/dory/node/nodeapp"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// frontend/dist/index.html is the bootstrap page (the only static asset).
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	dir := dataDir()
	shell, err := nodeapp.StartShell(context.Background(), nodeapp.Options{
		DataDir: dir, PeerPort: 7710, ConfirmViaUI: true, DeviceName: "Android"})
	if err != nil {
		log.Fatal(err)
	}
	defer shell.Stop()
	// The Java side reads this to push network changes (loopback only, gate secret).
	cfg, _ := json.Marshal(map[string]any{"port": portOf(shell.Addr()), "gate": shell.Gate()})
	if err := os.WriteFile(filepath.Join(dir, "shell.json"), cfg, 0o600); err != nil {
		log.Printf("shell.json: %v", err)
	}

	sub, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	static := application.AssetFileServerFS(sub)
	boot := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/boot/"):
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			json.NewEncoder(w).Encode(bootNode(shell, strings.TrimPrefix(r.URL.Path, "/boot/")))
		default:
			static.ServeHTTP(w, r)
		}
	})

	app := application.New(application.Options{
		Name:        "Dory",
		Description: "Dory local-first node",
		Assets:      application.AssetOptions{Handler: boot},
	})
	// Back in the foreground: reconnect to the peers now, not after the backoff.
	app.Event.OnApplicationEvent(events.Android.ActivityResumed, func(*application.ApplicationEvent) { shell.Kick() })
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "Dory", Width: 420, Height: 800, URL: "/"})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// bootNode applies the interfaces the page read, starts the node when the
// device already belongs to a group, and returns where the webview goes next.
func bootNode(shell *nodeapp.Shell, b64 string) map[string]string {
	raw, err := base64.RawURLEncoding.DecodeString(b64)
	var list []nodeapp.NetIface
	if err == nil {
		err = json.Unmarshal(raw, &list)
	}
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	if err := shell.SetNetwork(list); err != nil {
		log.Printf("set network: %v", err)
	}
	if err := shell.StartNode(); err != nil {
		return map[string]string{"error": err.Error()}
	}
	keepAlive()
	return map[string]string{"url": shell.EnterURL()}
}

func portOf(addr string) int {
	i := strings.LastIndex(addr, ":")
	n := 0
	for _, c := range addr[i+1:] {
		n = n*10 + int(c-'0')
	}
	return n
}
