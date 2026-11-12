package nodeapp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/node/localstore"
	"github.com/migueel15/dory/node/peer"
)

const (
	shellCookie = "dory_shell"
	gateHeader  = "X-Dory-Gate"
	joinAPI     = "/node/api/join"
	netAPI      = "/__net"
	enterPath   = "/__enter"
)

// NetIface is a network interface as the Java side of the Android app lists
// it (Go cannot enumerate interfaces on Android 11+).
type NetIface struct {
	Name  string   `json:"name"`
	Index int      `json:"index"`
	MTU   int      `json:"mtu"`
	IPs   []string `json:"ips"`
}

// netFrom converts the interface list into what Options and MDNS take.
func netFrom(list []NetIface) (ips []string, ifaces []net.Interface) {
	for _, f := range list {
		ips = append(ips, f.IPs...)
		ifaces = append(ifaces, net.Interface{Index: f.Index, MTU: f.MTU, Name: f.Name,
			Flags: net.FlagUp | net.FlagMulticast | net.FlagBroadcast})
	}
	return
}

// Shell hosts a node for a webview that cannot serve it in-process (Android:
// its asset server drops POST bodies and query strings). It listens on
// 127.0.0.1 on a random port and the webview navigates to EnterURL.
//
// Any app on the phone can open a loopback port, so every request needs the
// shell cookie, handed out only by EnterURL (its token is known to the app
// alone) or the gate header. Behind the gate the node answers as the local
// user, as in Node.Handler. Until the device belongs to a group the shell
// serves only the «Unirse a un grupo» page; joining starts the node.
type Shell struct {
	Options Options // Listen must be empty; the shell owns the listener

	token string
	ln    net.Listener
	srv   *http.Server

	mu      sync.Mutex
	node    *Node
	ips     []string
	ifaces  []net.Interface
	joining bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// StartShell binds the loopback listener and, when the device already belongs
// to a group, starts the node.
func StartShell(ctx context.Context, o Options) (*Shell, error) {
	if o.Listen != "" {
		return nil, errors.New("nodeapp: Shell owns the HTTP listener")
	}
	if err := os.MkdirAll(o.DataDir, 0o700); err != nil {
		return nil, err
	}
	s := &Shell{Options: o, token: randomSecret()}
	s.ctx, s.cancel = context.WithCancel(ctx)
	var err error
	if s.ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
		return nil, err
	}
	s.srv = &http.Server{Handler: http.HandlerFunc(s.serve), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		if err := s.srv.Serve(s.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("shell: %v", err)
		}
	}()
	return s, nil
}

// Joined reports whether the data dir already belongs to a group.
func Joined(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "group.json"))
	return err == nil
}

// Addr is the bound loopback address.
func (s *Shell) Addr() string { return s.ln.Addr().String() }

// EnterURL is the address the webview opens: it sets the cookie and lands on
// the app (or the join page).
func (s *Shell) EnterURL() string { return "http://" + s.Addr() + enterPath + "?t=" + s.token }

// Gate is the secret for the gate header (the Java side pushes network changes
// with it).
func (s *Shell) Gate() string { return s.token }

// Node is the running node, nil until the device has joined a group.
func (s *Shell) Node() *Node {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.node
}

// StartNode starts the node if the device belongs to a group and it is not
// running yet.
func (s *Shell) StartNode() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.node != nil || !Joined(s.Options.DataDir) {
		return nil
	}
	o := s.Options
	o.IPs, o.Ifaces = s.ips, s.ifaces
	n, err := Start(s.ctx, o)
	if err != nil {
		return err
	}
	s.node = n
	return nil
}

// SetNetwork records the current interfaces and applies them to the node.
func (s *Shell) SetNetwork(list []NetIface) error {
	ips, ifaces := netFrom(list)
	s.mu.Lock()
	same := slices.Equal(s.ips, ips)
	s.ips, s.ifaces = ips, ifaces
	n := s.node
	s.mu.Unlock()
	if n == nil {
		return nil
	}
	if same {
		n.Kick() // nothing moved; the caller only woke us up
		return nil
	}
	return n.SetNetwork(ips, ifaces)
}

// Kick makes the node retry its peers now (app back in the foreground).
func (s *Shell) Kick() {
	if n := s.Node(); n != nil {
		n.Kick()
	}
}

// Stop closes the listener and the node.
func (s *Shell) Stop() {
	s.cancel()
	sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	s.srv.Shutdown(sctx)
	c()
	if n := s.Node(); n != nil {
		n.Stop()
	}
}

func (s *Shell) authorized(r *http.Request) bool {
	if g := r.Header.Get(gateHeader); g != "" {
		return subtle.ConstantTimeCompare([]byte(g), []byte(s.token)) == 1
	}
	c, err := r.Cookie(shellCookie)
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.token)) == 1
}

func (s *Shell) serve(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.Host)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}
	if r.URL.Path == enterPath && r.Method == http.MethodGet {
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("t")), []byte(s.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: shellCookie, Value: s.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 10 * 365 * 24 * 3600})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch {
	case r.URL.Path == netAPI && r.Method == http.MethodPost:
		var list []NetIface
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&list); err != nil {
			writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
			return
		}
		if err := s.SetNetwork(list); err != nil {
			log.Printf("shell: set network: %v", err)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if n := s.Node(); n != nil {
		n.Handler().ServeHTTP(w, r)
		return
	}
	s.serveJoin(w, r)
}

var joinFiles = map[string]bool{"/node/join.html": true, "/node/join.js": true, "/node/jsqr.js": true}

// serveJoin is everything the shell answers before the device has a group.
func (s *Shell) serveJoin(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/" || r.URL.Path == "/node" || r.URL.Path == "/node/":
		http.Redirect(w, r, "/node/join.html", http.StatusSeeOther)
	case joinFiles[r.URL.Path] && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		sub, _ := fs.Sub(uiFS, "ui")
		http.StripPrefix("/node", http.FileServerFS(sub)).ServeHTTP(w, r)
	case r.URL.Path == joinAPI && r.Method == http.MethodPost:
		s.handleJoin(w, r)
	default:
		writeJSON(w, http.StatusServiceUnavailable, errBody("este dispositivo aún no pertenece a un grupo"))
	}
}

func (s *Shell) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(csrfHeader) != "1" {
		writeJSON(w, http.StatusForbidden, errBody("forbidden"))
		return
	}
	var in struct {
		Payload string `json:"payload"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil || in.Payload == "" {
		writeJSON(w, http.StatusBadRequest, errBody("falta el contenido del QR"))
		return
	}
	s.mu.Lock()
	if s.joining {
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, errBody("ya hay un emparejamiento en curso"))
		return
	}
	s.joining = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.joining = false; s.mu.Unlock() }()

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = s.Options.DeviceName
	}
	if err := JoinGroup(r.Context(), s.Options.DataDir, name, in.Payload); err != nil {
		writeJSON(w, http.StatusBadGateway, errBody(err.Error()))
		return
	}
	if err := s.StartNode(); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// JoinGroup pairs a device that has no group yet with the one that shows the
// QR payload: it saves the verified membership and applies the snapshot to the
// local database. Start the node afterwards.
func JoinGroup(ctx context.Context, dir, name, payload string) error {
	if Joined(dir) {
		return errors.New("este dispositivo ya pertenece a un grupo")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	id, err := peer.LoadOrCreate(dir, name)
	if err != nil {
		return err
	}
	store, err := localstore.Open(filepath.Join(dir, "dory-node.db"), localstore.Options{Device: id.DeviceID(), Local: true})
	if err != nil {
		return err
	}
	defer store.Close()
	res, err := peer.Join(ctx, payload, id, id.Name)
	if err != nil {
		return err
	}
	if err := res.ApplySnapshot(ctx, store.Log()); err != nil {
		return fmt.Errorf("aplicar datos: %w", err)
	}
	// The group file marks the device as joined: save it last so a failure
	// above can be retried with the same (still valid) identity.
	return peer.SaveGroup(dir, res.Group)
}
