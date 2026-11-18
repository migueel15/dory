package mesh

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/oplog"
)

const (
	// DefaultPort is where nodes listen for each other.
	DefaultPort = 7710
	pairTTL     = 5 * time.Minute
	syncEvery   = 10 * time.Second
	// offlineAfter: a peer not heard from for this long shows as disconnected.
	offlineAfter = 30 * time.Second
	pageSize     = 500
)

// Peer is a paired node as the interface shows it.
type Peer struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	PublicKey   string          `json:"public_key"`
	Fingerprint string          `json:"fingerprint"`
	Address     string          `json:"address"`
	Connected   bool            `json:"connected"`
	LastSeen    string          `json:"last_seen"`
	Info        json.RawMessage `json:"info"`
	Revoked     bool            `json:"revoked"`
}

// Mesh is the peer side of a node.
type Mesh struct {
	db   *sql.DB
	log  *oplog.Log
	id   Identity
	name string
	port int
	// Info describes this node to its peers (OS, version, harnesses).
	Info func(ctx context.Context) any
	// API serves calls forwarded from peers for threads this node owns.
	API http.Handler
	// Web serves the interface and the authenticated API to paired phones.
	Web http.Handler
	// IssueDevice creates the token of a phone that showed the pairing code.
	IssueDevice func(ctx context.Context, name string) (string, error)
	// DeviceSession trades the token of a phone for a session cookie.
	DeviceSession http.HandlerFunc

	mu        sync.Mutex
	pairCode  string
	pairExp   time.Time
	pairFails int
	wake      map[string]chan struct{}
	stop      map[string]context.CancelFunc
	found     map[string]Found
	streams   map[chan struct{}]struct{}
}

// Found is a node seen on the network that is not paired yet.
type Found struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
	Seen    time.Time `json:"seen"`
}

func New(db *sql.DB, log *oplog.Log, id Identity, name string, port int) *Mesh {
	return &Mesh{db: db, log: log, id: id, name: name, port: port, wake: map[string]chan struct{}{}, stop: map[string]context.CancelFunc{}, found: map[string]Found{}, streams: map[chan struct{}]struct{}{}}
}

// Changed is called after every local write: peers syncing with this node
// are told at once, and this node pushes to its peers.
func (m *Mesh) Changed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for ch := range m.streams {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	for _, ch := range m.wake {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Peers lists the paired nodes with their presence.
func (m *Mesh) Peers(ctx context.Context) ([]Peer, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT node_id, name, public_key, address, info, COALESCE(last_seen, ''), revoked_at IS NOT NULL FROM peers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Peer{}
	for rows.Next() {
		var p Peer
		var info string
		if err = rows.Scan(&p.ID, &p.Name, &p.PublicKey, &p.Address, &info, &p.LastSeen, &p.Revoked); err != nil {
			return nil, err
		}
		p.Info = json.RawMessage(info)
		p.Fingerprint = Fingerprint(p.PublicKey)
		if t, err := time.Parse(time.RFC3339Nano, p.LastSeen); err == nil {
			p.Connected = !p.Revoked && time.Since(t) < offlineAfter
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (m *Mesh) member(ctx context.Context, publicKey string) (string, bool) {
	var id string
	err := m.db.QueryRowContext(ctx, `SELECT node_id FROM peers WHERE public_key = ? AND revoked_at IS NULL`, publicKey).Scan(&id)
	return id, err == nil
}

func (m *Mesh) seen(ctx context.Context, node, address string, info json.RawMessage) {
	q := `UPDATE peers SET last_seen = ?` + map[bool]string{true: `, address = ?`, false: ``}[address != ""] + map[bool]string{true: `, info = ?`, false: ``}[len(info) > 0] + ` WHERE node_id = ?`
	args := []any{time.Now().UTC().Format(time.RFC3339Nano)}
	if address != "" {
		args = append(args, address)
	}
	if len(info) > 0 {
		args = append(args, string(info))
	}
	if _, err := m.db.ExecContext(ctx, q, append(args, node)...); err != nil {
		log.Printf("mesh: presence of %s: %v", node, err)
	}
}

// --- server side ------------------------------------------------------------

// Serve listens for peers on the mesh port until ctx ends.
func (m *Mesh) Serve(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /peer/v1/hello", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]string{"node_id": m.log.Node(), "name": m.name, "public_key": m.id.PublicKey()})
	})
	mux.HandleFunc("POST /peer/v1/pair", m.acceptPair)
	mux.HandleFunc("GET /peer/v1/info", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
		reply(w, m.Info(r.Context()))
	}))
	mux.HandleFunc("GET /peer/v1/vector", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
		v, err := m.log.Vector(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		reply(w, v)
	}))
	mux.HandleFunc("POST /peer/v1/ops", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
		var since oplog.Vector
		if json.NewDecoder(r.Body).Decode(&since) != nil {
			http.Error(w, "bad vector", 400)
			return
		}
		ops, err := m.log.Since(r.Context(), since, pageSize)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		reply(w, ops)
	}))
	mux.HandleFunc("POST /peer/v1/apply", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
		var ops []oplog.Op
		if json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&ops) != nil {
			http.Error(w, "bad ops", 400)
			return
		}
		if err := m.log.Apply(r.Context(), ops); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	// stream tells a peer each time this node has something new.
	mux.HandleFunc("GET /peer/v1/stream", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
		flusher, _ := w.(http.Flusher)
		ch := make(chan struct{}, 1)
		m.mu.Lock()
		m.streams[ch] = struct{}{}
		m.mu.Unlock()
		defer func() { m.mu.Lock(); delete(m.streams, ch); m.mu.Unlock() }()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": ready\n\n")
		flusher.Flush()
		ping := time.NewTicker(15 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ch:
				fmt.Fprint(w, "event: change\ndata: {}\n\n")
			case <-ping.C:
				fmt.Fprint(w, ": ping\n\n")
			}
			flusher.Flush()
		}
	}))
	// proxy runs an API call of a peer's client here, for threads this node owns.
	mux.HandleFunc("/peer/v1/proxy/", m.members(func(w http.ResponseWriter, r *http.Request, node string) {
		r2 := r.Clone(auth.AsPeer(r.Context(), node))
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/peer/v1/proxy")
		r2.RequestURI = ""
		m.API.ServeHTTP(w, r2)
	}))
	// Phones pair with the same code as nodes and then use the interface
	// over this TLS port; they present no client certificate.
	mux.HandleFunc("POST /device/v1/pair", m.pairDevice)
	mux.HandleFunc("POST /device/v1/session", func(w http.ResponseWriter, r *http.Request) {
		if m.DeviceSession == nil {
			http.NotFound(w, r)
			return
		}
		m.DeviceSession(w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if m.Web == nil {
			http.NotFound(w, r)
			return
		}
		m.Web.ServeHTTP(w, r)
	})
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", m.port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// Discovery probes open and drop connections; their errors are noise.
		ErrorLog: log.New(io.Discard, "", 0),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{m.id.Cert},
			ClientAuth:   tls.RequestClientCert,
			MinVersion:   tls.VersionTLS13,
		},
	}
	go func() { <-ctx.Done(); srv.Close() }()
	err := srv.ListenAndServeTLS("", "")
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// members lets through only paired, unrevoked nodes.
func (m *Mesh) members(h func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := peerKey(rawCerts(r))
		if err != nil {
			http.Error(w, "no client certificate", 401)
			return
		}
		node, ok := m.member(r.Context(), key)
		if !ok {
			http.Error(w, "not paired", 403)
			return
		}
		m.seen(r.Context(), node, "", nil)
		h(w, r, node)
	}
}

func rawCerts(r *http.Request) [][]byte {
	if r.TLS == nil {
		return nil
	}
	out := make([][]byte, len(r.TLS.PeerCertificates))
	for i, c := range r.TLS.PeerCertificates {
		out[i] = c.Raw
	}
	return out
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// --- pairing ----------------------------------------------------------------

// PairCode opens pairing for a few minutes and returns the code to type on
// the other node.
func (m *Mesh) PairCode() (string, time.Time) {
	b := make([]byte, 4)
	rand.Read(b)
	code := fmt.Sprintf("%04d-%04d", (int(b[0])<<8|int(b[1]))%10000, (int(b[2])<<8|int(b[3]))%10000)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pairCode, m.pairExp = code, time.Now().Add(pairTTL)
	return code, m.pairExp
}

type pairRequest struct {
	NodeID  string `json:"node_id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Proof   string `json:"proof"`
}

type pairResponse struct {
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
	Proof  string `json:"proof"`
}

// proof binds the code to both keys, so a node in the middle that does not
// know the code cannot pose as either side.
func proof(code, from, to string) string {
	mac := hmac.New(sha256.New, []byte(code))
	mac.Write([]byte(from + "|" + to))
	return hex.EncodeToString(mac.Sum(nil))
}

func (m *Mesh) acceptPair(w http.ResponseWriter, r *http.Request) {
	key, err := peerKey(rawCerts(r))
	if err != nil {
		http.Error(w, "no client certificate", 401)
		return
	}
	var req pairRequest
	if json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req) != nil || req.NodeID == "" {
		http.Error(w, "bad request", 400)
		return
	}
	m.mu.Lock()
	code, valid := m.pairCode, time.Now().Before(m.pairExp)
	m.mu.Unlock()
	if !valid || !hmac.Equal([]byte(req.Proof), []byte(proof(code, key, m.id.PublicKey()))) {
		http.Error(w, "wrong or expired code", 403)
		return
	}
	address := req.Address
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && address != "" {
		if _, port, err := net.SplitHostPort(address); err == nil {
			address = net.JoinHostPort(host, port)
		}
	}
	if err = m.addPeer(r.Context(), req.NodeID, req.Name, key, address); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	m.mu.Lock()
	m.pairCode = ""
	m.mu.Unlock()
	reply(w, pairResponse{NodeID: m.log.Node(), Name: m.name, Proof: proof(code, m.id.PublicKey(), key)})
	go m.Start(context.Background(), req.NodeID)
}

func (m *Mesh) addPeer(ctx context.Context, node, name, key, address string) error {
	_, err := m.db.ExecContext(ctx, `INSERT INTO peers(node_id, name, public_key, address, paired_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET name = excluded.name, public_key = excluded.public_key, address = excluded.address, revoked_at = NULL`,
		node, name, key, address, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// Pair joins the node at address with the code it shows.
func (m *Mesh) Pair(ctx context.Context, address, code string) (Peer, error) {
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, fmt.Sprint(DefaultPort))
	}
	var theirKey string
	hello := m.client("", &theirKey)
	resp, err := hello.Get("https://" + address + "/peer/v1/hello")
	if err != nil {
		return Peer{}, fmt.Errorf("no se pudo conectar con %s: %w", address, err)
	}
	resp.Body.Close()
	body, _ := json.Marshal(pairRequest{NodeID: m.log.Node(), Name: m.name, Address: fmt.Sprintf(":%d", m.port), Proof: proof(code, m.id.PublicKey(), theirKey)})
	resp, err = m.client(theirKey, nil).Post("https://"+address+"/peer/v1/pair", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return Peer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return Peer{}, fmt.Errorf("el otro nodo rechazó el emparejamiento: %s", strings.TrimSpace(string(msg)))
	}
	var out pairResponse
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Peer{}, err
	}
	if !hmac.Equal([]byte(out.Proof), []byte(proof(code, theirKey, m.id.PublicKey()))) {
		return Peer{}, errors.New("el otro nodo no demostró conocer el código")
	}
	if err = m.addPeer(ctx, out.NodeID, out.Name, theirKey, address); err != nil {
		return Peer{}, err
	}
	go m.Start(context.Background(), out.NodeID)
	return Peer{ID: out.NodeID, Name: out.Name, PublicKey: theirKey, Fingerprint: Fingerprint(theirKey), Address: address}, nil
}

// Revoke stops trusting a peer.
func (m *Mesh) Revoke(ctx context.Context, node string) error {
	_, err := m.db.ExecContext(ctx, `UPDATE peers SET revoked_at = ? WHERE node_id = ?`, time.Now().UTC().Format(time.RFC3339Nano), node)
	m.mu.Lock()
	if cancel, ok := m.stop[node]; ok {
		cancel()
		delete(m.stop, node)
		delete(m.wake, node)
	}
	m.mu.Unlock()
	return err
}

// devicePairMaxFails closes pairing after this many wrong codes, so the
// code cannot be guessed by trying.
const devicePairMaxFails = 5

// pairDevice gives a phone that knows the code a token for this node. The
// proof binds the code to the key of this node, so the phone can tell that
// nobody in the middle answered.
func (m *Mesh) pairDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if m.IssueDevice == nil || json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req) != nil || req.Code == "" {
		http.Error(w, "bad request", 400)
		return
	}
	m.mu.Lock()
	valid := m.pairCode != "" && time.Now().Before(m.pairExp) && hmac.Equal([]byte(req.Code), []byte(m.pairCode))
	if valid {
		m.pairCode = ""
	} else if m.pairFails++; m.pairFails >= devicePairMaxFails {
		m.pairCode = ""
	}
	code := req.Code
	m.mu.Unlock()
	if !valid {
		http.Error(w, "wrong or expired code", 403)
		return
	}
	token, err := m.IssueDevice(r.Context(), req.Name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	reply(w, map[string]string{"node_id": m.log.Node(), "name": m.name, "public_key": m.id.PublicKey(), "token": token, "proof": proof(code, m.id.PublicKey(), "device")})
}

// --- client side ------------------------------------------------------------

func (m *Mesh) client(want string, got *string) *http.Client {
	return &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: m.id.clientTLS(want, got), ResponseHeaderTimeout: 15 * time.Second}}
}

func (m *Mesh) peer(ctx context.Context, node string) (address, key string, err error) {
	err = m.db.QueryRowContext(ctx, `SELECT address, public_key FROM peers WHERE node_id = ? AND revoked_at IS NULL`, node).Scan(&address, &key)
	return address, key, err
}

// Run keeps every paired node in sync until ctx ends.
func (m *Mesh) Run(ctx context.Context) {
	peers, err := m.Peers(ctx)
	if err != nil {
		log.Printf("mesh: %v", err)
	}
	for _, p := range peers {
		if !p.Revoked {
			go m.Start(ctx, p.ID)
		}
	}
	go m.discover(ctx)
}

// Start follows one peer: it syncs every few seconds, at once after a local
// change, and whenever the peer announces one.
func (m *Mesh) Start(ctx context.Context, node string) {
	m.mu.Lock()
	if _, running := m.wake[node]; running {
		m.mu.Unlock()
		return
	}
	wake := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(ctx)
	m.wake[node], m.stop[node] = wake, cancel
	m.mu.Unlock()
	go m.listen(ctx, node, wake)
	timer := time.NewTimer(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-timer.C:
		}
		if err := m.sync(ctx, node); err != nil && ctx.Err() == nil {
			log.Printf("mesh: sync with %s: %v", node, err)
		}
		timer.Reset(syncEvery)
	}
}

// listen holds the peer's change stream open and wakes the sync loop on
// each change.
func (m *Mesh) listen(ctx context.Context, node string, wake chan struct{}) {
	for ctx.Err() == nil {
		address, key, err := m.peer(ctx, node)
		if err != nil {
			return
		}
		if address != "" {
			c := m.client(key, nil)
			c.Timeout = 0
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+address+"/peer/v1/stream", nil)
			if resp, err := c.Do(req); err == nil {
				buf := make([]byte, 512)
				for {
					n, err := resp.Body.Read(buf)
					if n > 0 && strings.Contains(string(buf[:n]), "event: change") {
						select {
						case wake <- struct{}{}:
						default:
						}
					}
					if err != nil {
						break
					}
				}
				resp.Body.Close()
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
}

// sync exchanges what each side is missing: it pulls the peer's new ops and
// pushes its own, so one reachable direction is enough.
func (m *Mesh) sync(ctx context.Context, node string) error {
	address, key, err := m.peer(ctx, node)
	if err != nil || address == "" {
		return err
	}
	c := m.client(key, nil)
	base := "https://" + address + "/peer/v1"
	var info json.RawMessage
	if err = getJSON(ctx, c, base+"/info", &info); err != nil {
		return err
	}
	m.seen(ctx, node, "", info)
	for range 1000 {
		mine, err := m.log.Vector(ctx)
		if err != nil {
			return err
		}
		var ops []oplog.Op
		if err = postJSON(ctx, c, base+"/ops", mine, &ops); err != nil {
			return err
		}
		if len(ops) > 0 {
			if err = m.log.Apply(ctx, ops); err != nil {
				return err
			}
		}
		if len(ops) < pageSize {
			break
		}
	}
	for range 1000 {
		var theirs oplog.Vector
		if err = getJSON(ctx, c, base+"/vector", &theirs); err != nil {
			return err
		}
		ops, err := m.log.Since(ctx, theirs, pageSize)
		if err != nil || len(ops) == 0 {
			return err
		}
		if err = postJSON(ctx, c, base+"/apply", ops, nil); err != nil {
			return err
		}
		if len(ops) < pageSize {
			return nil
		}
	}
	return nil
}

func getJSON(ctx context.Context, c *http.Client, url string, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	return do(c, req, out)
}

func postJSON(ctx context.Context, c *http.Client, url string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	return do(c, req, out)
}

func do(c *http.Client, req *http.Request, out any) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s: %d %s", req.URL.Path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Forward sends an API call to the node that owns it and copies the answer,
// streams included. It reports false when that node cannot be reached.
func (m *Mesh) Forward(w http.ResponseWriter, r *http.Request, node string) bool {
	address, key, err := m.peer(r.Context(), node)
	if err != nil || address == "" {
		return false
	}
	target := &url.URL{Scheme: "https", Host: address}
	failed := false
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = "/peer/v1/proxy" + r.URL.Path
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Authorization")
		},
		Transport:     &http.Transport{TLSClientConfig: m.id.clientTLS(key, nil), ResponseHeaderTimeout: 15 * time.Second},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			failed = true
			log.Printf("mesh: forward to %s: %v", node, err)
		},
	}
	proxy.ServeHTTP(w, r)
	return !failed
}
