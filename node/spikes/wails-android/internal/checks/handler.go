package checks

import (
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var processStart = time.Now()

// Result is what every check returns to the page.
type Result struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Ms     int64  `json:"ms"`
}

// App wires the checks together and is the http.Handler served in-process.
type App struct {
	Env  *Env
	MTLS *MTLS
	HB   *Heartbeat
}

func NewApp(env *Env) *App {
	return &App{Env: env, MTLS: NewMTLS(env, 0), HB: NewHeartbeat(env)}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func run(name string, f func() (string, error)) Result {
	t := time.Now()
	trace("start " + name)
	detail, err := f()
	trace("done " + name)
	r := Result{Name: name, OK: err == nil, Detail: detail, Ms: time.Since(t).Milliseconds()}
	if err != nil {
		r.Detail = err.Error()
	}
	return r
}

// Handler returns the single http.Handler of the app. Wails calls it directly
// for every request the webview makes to https://wails.localhost/ (Android:
// JNI shouldInterceptRequest -> Go). No TCP port is opened for it.
func (a *App) Handler(static fs.FS) http.Handler {
	mux := http.NewServeMux()
	files := http.FileServerFS(static)
	// http.FileServer answers 301 for /index.html (-> "./"). Wails' Android asset
	// loader asks for /index.html and treats any non-200 as "not found", so serve
	// the index directly. Found while running the spike on an emulator.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			b, err := fs.ReadFile(static, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(b)
			return
		}
		files.ServeHTTP(w, r)
	})

	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"ok": true, "host": r.Host, "remoteAddr": r.RemoteAddr, "proto": r.Proto,
			"tlsOnWire": r.TLS != nil, "goos": runtime.GOOS, "goarch": runtime.GOARCH,
			"goVersion": runtime.Version(), "sinceProcessStartMs": time.Since(processStart).Milliseconds(),
			"now": time.Now().UTC().Format(time.RFC3339Nano),
		})
	})
	mux.HandleFunc("GET /api/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"dataDir": a.Env.DataDir, "pkiDir": PKIDir(a.Env),
			"mtlsPort": a.MTLS.Port(), "ifaces": a.Env.Ifaces()})
	})
	// Input travels in the URL path, on purpose. On Android the in-process
	// handler never sees request bodies (POST bodies are lost) nor query strings
	// (WebViewAssetLoader strips them for every path outside /wails/).
	mux.HandleFunc("GET /api/ifaces/set/{b64}", func(w http.ResponseWriter, r *http.Request) {
		var in []Iface
		raw, err := base64.RawURLEncoding.DecodeString(r.PathValue("b64"))
		if err == nil {
			err = json.Unmarshal(raw, &in)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.Env.SetIfaces(in)
		// Re-issue the server certificate so its SANs cover the device IPs.
		err = a.MTLS.Start()
		writeJSON(w, map[string]any{"ok": err == nil, "count": len(in), "mtlsRestart": errString(err)})
	})

	mux.HandleFunc("GET /api/checks/handler", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, run("handler", func() (string, error) {
			return "served by in-process http.Handler, Host=" + r.Host + " (no listener involved)", nil
		}))
	})
	mux.HandleFunc("GET /api/checks/sqlite", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, run("sqlite", func() (string, error) { return CheckSQLite(a.Env) }))
	})
	mux.HandleFunc("GET /api/checks/mdns", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, run("mdns", func() (string, error) { return CheckMDNS(a.Env, a.MTLS.Port()) }))
	})
	mux.HandleFunc("GET /api/checks/mtls", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, run("mtls", a.MTLS.SelfTest))
	})
	mux.HandleFunc("GET /api/checks/background", func(w http.ResponseWriter, r *http.Request) {
		a.HB.Start()
		writeJSON(w, run("background", a.HB.Verdict))
	})
	mux.HandleFunc("GET /api/background", func(w http.ResponseWriter, r *http.Request) {
		a.HB.Start()
		writeJSON(w, map[string]any{"heartbeat": a.HB.Snapshot(),
			"mtlsConns": a.MTLS.Conns.Load(), "mtlsReqs": a.MTLS.Reqs.Load()})
	})
	mux.HandleFunc("GET /api/foreground/{state}", func(w http.ResponseWriter, r *http.Request) {
		on := r.PathValue("state") == "on"
		if a.Env.SetForeground == nil {
			writeJSON(w, map[string]any{"ok": false, "detail": "no foreground service on this platform"})
			return
		}
		a.Env.SetForeground(on, "Dory spike", "Keeping the node alive")
		writeJSON(w, map[string]any{"ok": true, "on": on})
	})
	return mux
}

// TraceDir, when set, receives a trace.log with one line per check start/end.
// stderr is invisible on Android, and a native crash (SIGSYS) leaves nothing else.
var TraceDir string

func trace(msg string) {
	if TraceDir == "" {
		return
	}
	if f, err := os.OpenFile(filepath.Join(TraceDir, "trace.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		f.WriteString(time.Now().Format("15:04:05.000 ") + msg + "\n")
		f.Close()
	}
}
