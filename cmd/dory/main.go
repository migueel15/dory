package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"syscall"
	"time"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/compress"
	"github.com/migueel15/dory/internal/config"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/dictation"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/mcp"
	"github.com/migueel15/dory/internal/mesh"
	"github.com/migueel15/dory/internal/notify"
	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/runtime"
	"github.com/migueel15/dory/internal/skills"
	"github.com/migueel15/dory/internal/version"
	"github.com/migueel15/dory/web"
)

const usage = `usage: dory <command>

  serve                  run the node: API, MCP, web interface and agents
  status | start | stop | restart   control the core of this machine
  pair [ADDRESS CODE]    show a pairing code, or pair with another node
  open                   start the core if needed and open the interface
  service install|uninstall         run the core as a user service
  update [--channel stable|nightly] install the latest release
  user create --username NAME   create a local user (password in DORY_PASSWORD)
  token create --username NAME --name N [--scopes read,write]
  version                print the version`

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New(usage)
	}
	if os.Args[1] == "version" {
		v := version.Read()
		fmt.Println(v.Short, v.Time)
		return nil
	}
	if handled, err := control(os.Args[1:]); handled {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err = os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	db, err := localdb.Open(ctx, filepath.Join(cfg.DataDir, "dory.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	log_, err := oplog.Open(ctx, db)
	if err != nil {
		return err
	}
	store := data.New(db, log_)
	if os.Args[1] != "serve" {
		handled, err := auth.Command(ctx, os.Args[1:], store.Accounts())
		if !handled && err == nil {
			err = fmt.Errorf("unknown command %q\n%s", os.Args[1], usage)
		}
		return err
	}
	return serve(ctx, cfg, store, log_)
}

func serve(ctx context.Context, cfg config.Config, store *data.Store, log_ *oplog.Log) error {
	providers := []runtime.Provider{runtime.Claude{}, runtime.Codex{}, runtime.OpenCode{}}
	if file := os.Getenv("DORY_REPLAY_FILE"); file != "" {
		providers = append(providers, runtime.Replay{File: file, Delay: 30 * time.Millisecond})
	}
	rt := runtime.NewManager(store.DB(), store, log_.Node(), cfg.DataDir, providers...)
	agentToken := auth.Secret()
	rt.MCPURL = "http://" + loopbackAddr(cfg.Listen) + "/mcp"
	rt.MCPToken = agentToken
	if err := rt.Recover(ctx); err != nil {
		return err
	}
	events := api.NewEvents(rt)
	id, err := mesh.LoadIdentity(cfg.DataDir)
	if err != nil {
		return err
	}
	net := mesh.New(store.DB(), log_, id, cfg.NodeName, cfg.MeshPort)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	reconciler := &skills.Reconciler{Store: store, Node: log_.Node(), Home: home, Manifest: filepath.Join(cfg.DataDir, "skills-manifest.json"),
		Installed: func(ctx context.Context) []string {
			var ids []string
			for _, p := range rt.Providers(ctx) {
				if p.Installed && p.ID != "replay" {
					ids = append(ids, p.ID)
				}
			}
			return ids
		}}
	notices, err := notify.New(ctx, store, cfg.DataDir)
	if err != nil {
		return err
	}
	log_.OnChange = func(entity, id string) {
		events.Changed(entity, id)
		net.Changed()
		reconciler.Changed(entity)
		notices.Changed(entity, id)
	}
	go reconciler.Run(ctx)
	go rt.Follow(ctx, home)
	gh := github.New(func(ctx context.Context) (map[string]string, error) { return followed(ctx, store) }, githubToken())
	srv := &api.Server{
		Store: store, Runtime: rt, GitHub: gh, Dictation: dictationManager(), Events: events,
		SpeechModels: dictation.NewLocalModels(ctx, filepath.Join(cfg.DataDir, "dictation-models"), os.Getenv("DORY_NEMO_SPEECH_BIN")),
		Node: api.Node{ID: log_.Node(), Name: cfg.NodeName, OS: goruntime.GOOS, Version: version.Read().Short,
			DataDir: cfg.DataDir, Listen: cfg.Listen, Platform: goruntime.GOOS + "/" + goruntime.GOARCH},
	}
	srv.Remote = net
	srv.Peers = func() any {
		peers, _ := net.Peers(context.Background())
		return peers
	}
	mux := http.NewServeMux()
	net.Mount(mux)
	notices.Mount(mux)
	srv.Register(mux)
	auth.Register(mux, store.Accounts(), cfg)
	auth.RegisterDevices(mux, store.Accounts())
	mux.Handle("/mcp", mcp.Handler(srv))
	mux.Handle("/", web.Handler())
	control, err := controlToken(cfg.DataDir)
	if err != nil {
		return err
	}
	authed := auth.Middleware(store.Accounts(), cfg, agentToken)(mux)
	server := &http.Server{Addr: cfg.Listen, Handler: compress.Middleware(meshControl(control, mux, authed)),
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	net.API = mux
	net.Web = compress.Middleware(authed)
	net.IssueDevice = func(ctx context.Context, name string) (string, error) {
		return auth.IssueDevice(ctx, store.Accounts(), name)
	}
	net.DeviceSession = func(w http.ResponseWriter, r *http.Request) { auth.DeviceSession(w, r, store.Accounts(), cfg) }
	net.Info = func(ctx context.Context) any {
		return map[string]any{"node": srv.Node, "providers": rt.Providers(ctx)}
	}
	done := make(chan error, 2)
	go func() { done <- server.ListenAndServe() }()
	if cfg.MeshPort > 0 {
		go func() {
			if err := net.Serve(ctx); err != nil {
				done <- fmt.Errorf("mesh: %w", err)
			}
		}()
		net.Run(ctx)
	}
	log.Printf("dory: node %s (%s) listening on %s, data in %s", cfg.NodeName, log_.Node(), cfg.Listen, cfg.DataDir)
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

// followed maps the remote of every repository of an active project to it.
func followed(ctx context.Context, store *data.Store) (map[string]string, error) {
	repos, err := store.Repositories().List(ctx, data.Filter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(repos))
	for _, r := range repos {
		if _, err := store.Projects().Get(ctx, r.ProjectID); err == nil {
			out[r.RemoteURL] = r.ProjectID
		}
	}
	return out, nil
}

// githubToken uses DORY_GITHUB_TOKEN, or the session of the gh CLI on this
// machine. The token stays on this node.
func githubToken() string {
	if t := os.Getenv("DORY_GITHUB_TOKEN"); t != "" {
		return t
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		log.Printf("dory: no GitHub token (DORY_GITHUB_TOKEN unset and gh not logged in)")
		return ""
	}
	return strings.TrimSpace(string(out))
}

// controlToken writes a fresh secret readable only by this user, so the CLI
// of this machine can pair nodes without an interface (dory pair).
func controlToken(dataDir string) (string, error) {
	t := auth.Secret()
	return t, os.WriteFile(filepath.Join(dataDir, "control.token"), []byte(t), 0o600)
}

// meshControl lets requests carrying the control token reach the mesh routes.
func meshControl(token string, mux http.Handler, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/mesh") && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) == 1 {
			mux.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackAddr is where local agents reach this node.
func loopbackAddr(listen string) string {
	if strings.HasPrefix(listen, "0.0.0.0:") || strings.HasPrefix(listen, ":") {
		_, port, _ := strings.Cut(listen, ":")
		if strings.HasPrefix(listen, "0.0.0.0:") {
			port = strings.TrimPrefix(listen, "0.0.0.0:")
		}
		return "127.0.0.1:" + port
	}
	return listen
}

// dictationManager uses the whisper-server at DORY_WHISPER_URL; without it the
// dictation API answers 503.
func dictationManager() *dictation.Manager {
	url := os.Getenv("DORY_WHISPER_URL")
	if url == "" {
		return dictation.New(nil)
	}
	return dictation.New(dictation.Whisper{URL: url, Language: "es"})
}
