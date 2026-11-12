package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/config"
	"github.com/migueel15/dory/internal/dictation"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/mcp"
	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
	"github.com/migueel15/dory/node/localstore"
	"github.com/migueel15/dory/node/peer"
	"github.com/migueel15/dory/node/syncx"
	"github.com/migueel15/dory/web"
)

const (
	sessionCookie = "dory_session"
	localUser     = "local"
	nodeActor     = "dory-node"
	defaultT3URL  = "http://localhost:3773"
	t3TokenEnv    = "DORY_T3_TOKEN"
)

// Options configures a node.
type Options struct {
	DataDir string
	// Listen is the host:port of the API, MCP and SPA; the host must be loopback.
	Listen string
	// PeerPort is the TCP port of the mTLS peer API (0 picks a free one).
	PeerPort int
	// Headless skips the SPA; API and MCP stay on localhost.
	Headless bool
	// NoMDNS disables the mDNS announcement and browsing.
	NoMDNS bool
	// T3URL is the local T3; empty means defaultT3URL when $DORY_T3_TOKEN is set.
	T3URL string
	// DeviceName is the name for a new identity; empty means the hostname.
	DeviceName string
	Intervals  peer.Intervals
	// Confirm asks the user to accept a device that scanned the pairing QR;
	// nil rejects everyone.
	Confirm func(name, pubkeyHex string) bool
}

// Node is a running dory-node.
type Node struct {
	Store    *localstore.Store
	Identity *peer.Identity
	Group    *peer.Log
	Presence *peer.Presence
	// Pairing opens the QR window (Pairing.Start) for another device to join.
	Pairing *peer.Pairing
	// HTTPAddr and PeerAddr are the bound addresses.
	HTTPAddr string
	PeerAddr string
	// TokenFile holds an admin bearer token for curl and MCP clients.
	TokenFile string

	cancel context.CancelFunc
	http   *http.Server
	peers  *peer.Server
	mdns   *peer.MDNS
	wg     sync.WaitGroup
}

// Start brings the node up. It returns once the listeners are bound.
func Start(ctx context.Context, o Options) (*Node, error) {
	if err := requireLoopback(o.Listen); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(o.DataDir, 0o700); err != nil {
		return nil, err
	}
	id, err := peer.LoadOrCreate(o.DataDir, o.DeviceName)
	if err != nil {
		return nil, err
	}
	device := id.DeviceID()
	store, err := localstore.Open(filepath.Join(o.DataDir, "dory-node.db"), localstore.Options{Device: device, Local: true})
	if err != nil {
		return nil, err
	}
	n := &Node{Store: store, Identity: id}
	ok := false
	defer func() {
		if !ok {
			n.Stop()
		}
	}()
	ctx, n.cancel = context.WithCancel(ctx)

	// Membership: a single-device group on first start; `dory-node join`
	// replaces it with the group of another device before the first start.
	n.Group, _, err = peer.EnsureGroup(o.DataDir, id)
	if err != nil {
		return nil, fmt.Errorf("membership: %w", err)
	}

	cfg := config.Config{Listen: o.Listen, CookieSecure: false, SessionTTL: 10 * 365 * 24 * time.Hour}
	session, _, err := bootstrapAuth(ctx, store, o.DataDir)
	if err != nil {
		return nil, err
	}
	n.TokenFile = filepath.Join(o.DataDir, "local-token")
	if err := setupT3(ctx, store, id, o); err != nil {
		return nil, err
	}

	// Local services. Notifications (Postgres) and dictation stay off.
	var svc service.Service = store
	monitor := t3.NewMonitor(ownInstances{Service: svc, env: environmentID(device)})
	n.go_(func() { monitor.Run(ctx) })
	svc = service.WithItemThreadArchiving(svc, func(ctx context.Context, instanceID, threadID string) error {
		client, err := monitor.InstanceClient(ctx, instanceID)
		if err != nil {
			return err
		}
		return client.ArchiveThread(ctx, threadID)
	})
	gh := github.New(svc, os.Getenv("DORY_GITHUB_TOKEN"))
	mux := http.NewServeMux()
	api.Register(mux, svc, monitor, gh, dictation.New(nil))
	auth.Register(mux, svc, cfg)
	mux.Handle("/mcp", mcp.Handler(svc, monitor.Locate))
	if !o.Headless {
		mux.Handle("/", autoLogin(svc, session, web.Handler()))
	}
	n.http = &http.Server{Handler: auth.Middleware(svc, cfg)(mux), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	httpLn, err := net.Listen("tcp", o.Listen)
	if err != nil {
		return nil, err
	}
	n.HTTPAddr = httpLn.Addr().String()
	n.go_(func() {
		if err := n.http.Serve(httpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http: %v", err)
		}
	})

	// Peer side: mTLS server, address record, mDNS, connector.
	peerLn, err := net.Listen("tcp", ":"+strconv.Itoa(o.PeerPort))
	if err != nil {
		return nil, err
	}
	port := peerLn.Addr().(*net.TCPAddr).Port
	n.PeerAddr = peerLn.Addr().String()
	n.Presence = peer.NewPresence(o.Intervals)
	n.peers = &peer.Server{Identity: id, Members: n.Group, Log: store.Log(), Presence: n.Presence, Intervals: o.Intervals}
	var local []string
	for _, ip := range peer.LocalIPs() {
		local = append(local, net.JoinHostPort(ip, strconv.Itoa(port)))
	}
	n.Pairing = &peer.Pairing{Identity: id, Group: n.Group, Data: store.Log(), Addrs: local, Confirm: o.Confirm,
		Persist: func(g *peer.Log) error { return peer.SaveGroup(o.DataDir, g) }}
	n.peers.Pairing = n.Pairing
	n.go_(func() {
		if err := n.peers.Serve(peerLn); err != nil {
			log.Printf("peer: %v", err)
		}
	})
	if err := store.PublishAddress(ctx, localstore.DeviceAddress{DeviceID: device, Name: id.Name, Local: local}); err != nil {
		return nil, fmt.Errorf("publish address: %w", err)
	}
	if !o.NoMDNS {
		n.mdns = &peer.MDNS{Identity: id, Port: port}
		if err := n.mdns.Announce(); err != nil {
			log.Printf("mdns announce: %v", err)
		}
		n.go_(func() {
			if err := n.mdns.Browse(ctx); err != nil && ctx.Err() == nil {
				log.Printf("mdns browse: %v", err)
			}
		})
	}
	conn := &peer.Connector{Identity: id, Group: n.Group, Engine: &syncx.Engine{Log: store.Log()},
		Presence: n.Presence, Book: store, MDNS: n.mdns, Intervals: o.Intervals,
		OnConnect: func(dev string, c peer.Candidate) { log.Printf("peer %s connected via %s (%s)", dev, c.Source, c.Addr) }}
	n.go_(func() { conn.Run(ctx) })
	ok = true
	return n, nil
}

func (n *Node) go_(f func()) {
	n.wg.Add(1)
	go func() { defer n.wg.Done(); f() }()
}

// Stop shuts everything down and closes the database.
func (n *Node) Stop() {
	if n.cancel != nil {
		n.cancel()
	}
	if n.http != nil {
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		n.http.Shutdown(sctx)
		c()
	}
	if n.peers != nil {
		n.peers.Close()
	}
	if n.mdns != nil {
		n.mdns.Stop()
	}
	n.wg.Wait()
	n.Store.Close()
}

func requireLoopback(listen string) error {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("--listen: %w", err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("--listen %q: API, MCP and SPA only listen on localhost", listen)
}

func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// secretFile returns the secret stored in dir/name, creating it (0600) once.
func secretFile(dir, name string) (string, error) {
	p := filepath.Join(dir, name)
	if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
		return strings.TrimSpace(string(b)), nil
	}
	s := randomSecret()
	return s, os.WriteFile(p, []byte(s+"\n"), 0o600)
}

// bootstrapAuth makes sure the local user exists with a long-lived session
// (for the in-process UI) and an admin token (for curl and MCP clients). The
// secrets live in 0600 files of the data dir. The user has no usable password.
func bootstrapAuth(ctx context.Context, svc service.Service, dir string) (session, token string, err error) {
	if session, err = secretFile(dir, "local-session"); err != nil {
		return
	}
	if token, err = secretFile(dir, "local-token"); err != nil {
		return
	}
	u, err := svc.FindUser(ctx, localUser)
	if errors.Is(err, service.ErrNotFound) {
		u, err = svc.Users().Put(ctx, nodeActor, service.User{Username: localUser, PasswordHash: "!"})
	}
	if err != nil {
		return
	}
	if _, err = svc.FindSession(ctx, digest(session)); errors.Is(err, service.ErrNotFound) {
		_, err = svc.Sessions().Put(ctx, u.ID, service.Session{UserID: u.ID, SecretHash: digest(session), ExpiresAt: time.Now().Add(10 * 365 * 24 * time.Hour)})
	}
	if err != nil {
		return
	}
	if _, err = svc.FindToken(ctx, digest(token)); errors.Is(err, service.ErrNotFound) {
		_, err = svc.Tokens().Put(ctx, u.ID, service.Token{UserID: u.ID, Name: "dory-node local", SecretHash: digest(token), Permissions: []string{"admin"}})
	}
	return
}

// autoLogin gives the in-process UI its session: a browser that opens the SPA
// without a valid session cookie receives the local one.
func autoLogin(svc service.Service, session string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		valid := false
		if c, err := r.Cookie(sessionCookie); err == nil {
			_, err = svc.FindSession(r.Context(), digest(c.Value))
			valid = err == nil
		}
		if !valid && r.Header.Get("Sec-Fetch-Site") != "cross-site" {
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 10 * 365 * 24 * 3600})
		}
		next.ServeHTTP(w, r)
	})
}

// stableID derives a UUID from a label, so the same device always uses the
// same record id.
func stableID(label string) string {
	sum := sha256.Sum256([]byte(label))
	b := sum[:16]
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func environmentID(device string) string { return stableID("dory-node-environment:" + device) }
func t3InstanceID(device string) string  { return stableID("dory-node-t3:" + device) }

// setupT3 registers the local T3 as a replicated t3_instance owned by this
// device (the store stamps owner_device). Without $DORY_T3_TOKEN or --t3-url
// there is nothing to monitor.
func setupT3(ctx context.Context, svc service.Service, id *peer.Identity, o Options) error {
	if o.T3URL == "" && os.Getenv(t3TokenEnv) == "" {
		return nil
	}
	url := o.T3URL
	if url == "" {
		url = defaultT3URL
	}
	device := id.DeviceID()
	envID := environmentID(device)
	if _, err := svc.Environments().Get(ctx, envID); errors.Is(err, service.ErrNotFound) {
		_, err = svc.Environments().Put(ctx, nodeActor, service.Environment{Record: service.Record{ID: envID}, Name: id.Name, Os: runtime.GOOS, Access: "local", AgentsAvailable: []string{}})
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	_, err := svc.T3Instances().Put(ctx, nodeActor, service.T3Instance{Record: service.Record{ID: t3InstanceID(device)}, EnvironmentID: envID, BaseUrl: url, TokenEnv: t3TokenEnv})
	return err
}

// ownInstances limits the T3 monitor to this device's environment: instances
// replicated from other devices point at their localhost, not ours.
type ownInstances struct {
	service.Service
	env string
}

func (o ownInstances) T3Instances() service.Resource[service.T3Instance] {
	return ownResource{o.Service.T3Instances(), o.env}
}

type ownResource struct {
	service.Resource[service.T3Instance]
	env string
}

func (r ownResource) Get(ctx context.Context, id string) (service.T3Instance, error) {
	in, err := r.Resource.Get(ctx, id)
	if err == nil && in.EnvironmentID != r.env {
		return service.T3Instance{}, service.ErrNotFound
	}
	return in, err
}

func (r ownResource) List(ctx context.Context, f service.Filter) ([]service.T3Instance, error) {
	all, err := r.Resource.List(ctx, f)
	out := all[:0]
	for _, in := range all {
		if in.EnvironmentID == r.env {
			out = append(out, in)
		}
	}
	return out, err
}
