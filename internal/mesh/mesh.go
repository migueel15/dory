package mesh

import (
	"bytes"
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
	"github.com/migueel15/dory/internal/i18n"
	"io"
	"log"
	"maps"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/schema"
)

const (
	// ProtocolVersion is the version of the node-to-node HTTP protocol.
	ProtocolVersion      = 2
	peerPrefix           = "/peer/v2"
	incompatibleProtocol = "incompatible_protocol"
	// DefaultPort is where nodes listen for each other.
	DefaultPort = 7710
	pairTTL     = 5 * time.Minute
	syncEvery   = 10 * time.Second
	pageSize    = oplog.BatchSize
)

// Peer is a paired node as the interface shows it.
type Peer struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	PublicKey    string          `json:"public_key"`
	Fingerprint  string          `json:"fingerprint"`
	Address      string          `json:"address"`
	Connected    bool            `json:"connected"`
	LastSeen     string          `json:"last_seen"`
	Info         json.RawMessage `json:"info"`
	Revoked      bool            `json:"revoked"`
	Protocol     int             `json:"protocol"`
	Incompatible bool            `json:"incompatible"`
	Disabled     bool            `json:"disabled"`
}

// Mesh is the peer side of a node.
type Mesh struct {
	db   *sql.DB
	log  *oplog.Log
	id   Identity
	name string
	port int
	// Info describes this node to its peers (OS, version, harnesses).
	Info func(ctx context.Context) map[string]any
	// OnThreadState is called when an owner publishes a changed thread state.
	OnThreadState func(threadID string)
	// API serves calls forwarded from peers for threads this node owns.
	API http.Handler
	// Web serves the interface and the authenticated API to paired phones.
	Web http.Handler
	// IssueDevice creates the token of a phone that showed the pairing code.
	IssueDevice func(ctx context.Context, name string) (string, error)
	// Connected is called when a peer becomes reachable: the first successful
	// sync after the peer was unknown or failing.
	Connected func(node string)
	// DeviceSession trades the token of a phone for a session cookie.
	DeviceSession http.HandlerFunc

	mu         sync.Mutex
	pairCode   string
	pairExp    time.Time
	pairFails  int
	wake       map[string]chan struct{}
	stop       map[string]context.CancelFunc
	found      map[string]Found
	streams    map[chan struct{}]struct{}
	threadWake *time.Timer
	transports map[string]peerTransport
}

// Found is a node seen on the network that is not paired yet.
type Found struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
	Seen    time.Time `json:"seen"`
}

func New(db *sql.DB, log *oplog.Log, id Identity, name string, port int) *Mesh {
	return &Mesh{db: db, log: log, id: id, name: name, port: port, wake: map[string]chan struct{}{}, stop: map[string]context.CancelFunc{}, found: map[string]Found{}, streams: map[chan struct{}]struct{}{}, transports: map[string]peerTransport{}}
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
	rows, err := m.db.QueryContext(ctx, `SELECT node_id, name, public_key, address, info, COALESCE(last_seen, ''), revoked_at IS NOT NULL, disabled_at IS NOT NULL, protocol, last_error, COALESCE((`+data.PeerConnectedSQL+`), 0) FROM peers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Peer{}
	for rows.Next() {
		var p Peer
		var info, lastError string
		if err = rows.Scan(&p.ID, &p.Name, &p.PublicKey, &p.Address, &info, &p.LastSeen, &p.Revoked, &p.Disabled, &p.Protocol, &lastError, &p.Connected); err != nil {
			return nil, err
		}
		p.Info = json.RawMessage(info)
		p.Incompatible = strings.HasPrefix(lastError, incompatibleProtocol+":")
		p.Fingerprint = Fingerprint(p.PublicKey)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (m *Mesh) member(ctx context.Context, publicKey string) (string, bool) {
	var id string
	err := m.db.QueryRowContext(ctx, `SELECT node_id FROM peers WHERE public_key = ? AND revoked_at IS NULL AND disabled_at IS NULL`, publicKey).Scan(&id)
	return id, err == nil
}

func (m *Mesh) seen(ctx context.Context, node string) {
	if _, err := m.db.ExecContext(ctx, `UPDATE peers SET last_seen = ? WHERE node_id = ?`, time.Now().UTC().Format(time.RFC3339Nano), node); err != nil {
		log.Printf("mesh: presence of %s: %v", node, err)
	}
}

// --- server side ------------------------------------------------------------

// Serve listens for peers on the mesh port until ctx ends.
func (m *Mesh) Serve(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+peerPrefix+"/hello", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]string{"node_id": m.log.Node(), "name": m.name, "public_key": m.id.PublicKey(), "device_pin": m.id.DevicePin})
	})
	mux.HandleFunc("POST "+peerPrefix+"/pair", m.acceptPair)
	mux.HandleFunc("GET "+peerPrefix+"/info", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
		info := map[string]any{}
		if m.Info != nil {
			maps.Copy(info, m.Info(r.Context()))
		}
		threads, err := m.ownedThreadStates(r.Context())
		if err != nil {
			apperr.WriteHTTP(w, r, err)
			return
		}
		info["threads"] = threads
		info["protocol"] = ProtocolVersion
		info["schema"] = schema.Hash()
		info["snapshots"] = true
		reply(w, info)
	}))
	m.mountReplication(mux)
	// stream tells a peer each time this node has something new.
	mux.HandleFunc("GET "+peerPrefix+"/stream", m.members(func(w http.ResponseWriter, r *http.Request, _ string) {
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
	mux.HandleFunc(proxyPrefix+"/", m.members(func(w http.ResponseWriter, r *http.Request, node string) {
		r2 := r.Clone(auth.AsPeer(r.Context(), node))
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, proxyPrefix)
		r2.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, proxyPrefix)
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
	mux.HandleFunc("/peer/", http.NotFound)
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
			// Nodes ask for the peer protocol and get the Ed25519 key; phones
			// (Android has no Ed25519 in TLS) get the ECDSA one.
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				for _, p := range hello.SupportedProtos {
					if p == peerProto {
						return &m.id.Cert, nil
					}
				}
				return &m.id.DeviceCert, nil
			},
			NextProtos: []string{"h2", "http/1.1"},
			ClientAuth: tls.RequestClientCert,
			MinVersion: tls.VersionTLS13,
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
			apperr.WriteHTTP(w, r, apperr.New(apperr.ForStatus(401), "no client certificate"))
			return
		}
		node, ok := m.member(r.Context(), key)
		if !ok {
			apperr.WriteHTTP(w, r, apperr.New(apperr.ForStatus(403), "not paired"))
			return
		}
		m.seen(r.Context(), node)
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
		apperr.WriteHTTP(w, r, apperr.New(apperr.ForStatus(401), "no client certificate"))
		return
	}
	var req pairRequest
	if json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req) != nil || req.NodeID == "" {
		apperr.WriteHTTP(w, r, apperr.New(apperr.ForStatus(400), "bad request"))
		return
	}
	m.mu.Lock()
	code, valid := m.pairCode, time.Now().Before(m.pairExp)
	m.mu.Unlock()
	if !valid || !hmac.Equal([]byte(req.Proof), []byte(proof(code, key, m.id.PublicKey()))) {
		apperr.WriteHTTP(w, r, apperr.New(apperr.ForStatus(403), "wrong or expired code"))
		return
	}
	address := req.Address
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && address != "" {
		if _, port, err := net.SplitHostPort(address); err == nil {
			address = net.JoinHostPort(host, port)
		}
	}
	if err = m.addPeer(r.Context(), req.NodeID, req.Name, key, address); err != nil {
		apperr.WriteHTTP(w, r, err)
		return
	}
	m.mu.Lock()
	m.pairCode = ""
	m.mu.Unlock()
	reply(w, pairResponse{NodeID: m.log.Node(), Name: m.name, Proof: proof(code, m.id.PublicKey(), key)})
	go m.Start(context.Background(), req.NodeID)
}

func (m *Mesh) addPeer(ctx context.Context, node, name, key, address string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.ExecContext(ctx, `INSERT INTO peers(node_id, name, public_key, address, paired_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET name = excluded.name, public_key = excluded.public_key, address = excluded.address, revoked_at = NULL, disabled_at = NULL, protocol = 0, last_error = ''`,
		node, name, key, address, time.Now().UTC().Format(time.RFC3339Nano))
	if err == nil {
		m.invalidateTransport(node)
	}
	return err
}

// Pair joins the node at address with the code it shows.
func (m *Mesh) Pair(ctx context.Context, address, code string) (Peer, error) {
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, fmt.Sprint(DefaultPort))
	}
	var theirKey string
	hello := m.probeClient("", &theirKey)
	defer hello.CloseIdleConnections()
	var info json.RawMessage
	if err := getJSON(ctx, hello, "https://"+address+peerPrefix+"/hello", &info); err != nil {
		return Peer{}, fmt.Errorf(i18n.Text("mesh.no_pudo_conectar"), address, err)
	}
	body, _ := json.Marshal(pairRequest{NodeID: m.log.Node(), Name: m.name, Address: fmt.Sprintf(":%d", m.port), Proof: proof(code, m.id.PublicKey(), theirKey)})
	client := m.probeClient(theirKey, nil)
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+address+peerPrefix+"/pair", strings.NewReader(string(body)))
	if err != nil {
		return Peer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return Peer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return Peer{}, apperr.New(apperr.Forbidden, "the other node rejected the pairing").With("reason", "pair_rejected").With("detail", strings.TrimSpace(string(msg)))
	}
	var out pairResponse
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Peer{}, err
	}
	if !hmac.Equal([]byte(out.Proof), []byte(proof(code, theirKey, m.id.PublicKey()))) {
		return Peer{}, apperr.New(apperr.Forbidden, "the other node did not prove it knows the code").With("reason", "pair_proof")
	}
	if err = m.addPeer(ctx, out.NodeID, out.Name, theirKey, address); err != nil {
		return Peer{}, err
	}
	go m.Start(context.Background(), out.NodeID)
	return Peer{ID: out.NodeID, Name: out.Name, PublicKey: theirKey, Fingerprint: Fingerprint(theirKey), Address: address}, nil
}

// Revoke stops trusting a peer and forgets it. Pairing again adds it back.
func (m *Mesh) Revoke(ctx context.Context, node string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.ExecContext(ctx, `DELETE FROM peers WHERE node_id = ?`, node)
	if err == nil {
		m.halt(node)
	}
	return err
}

// SetEnabled pauses or resumes the sync with a paired node, keeping the
// pairing. A paused node is neither synced with nor accepted.
func (m *Mesh) SetEnabled(ctx context.Context, node string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var at any
	if !enabled {
		at = time.Now().UTC().Format(time.RFC3339Nano)
	}
	res, err := m.db.ExecContext(ctx, `UPDATE peers SET disabled_at = ? WHERE node_id = ? AND revoked_at IS NULL`, at, node)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if enabled {
		go m.Start(context.Background(), node)
	} else {
		m.halt(node)
	}
	return nil
}

// halt stops following a peer. The caller holds m.mu.
func (m *Mesh) halt(node string) {
	m.invalidateTransport(node)
	if cancel, ok := m.stop[node]; ok {
		cancel()
		delete(m.stop, node)
		delete(m.wake, node)
	}
}

// devicePairMaxFails closes pairing after this many wrong codes, so the
// code cannot be guessed by trying.
const devicePairMaxFails = 5

// pairDevice gives a phone that knows the code a token for this node. The
// proof binds the code to the phone key of this node, so the phone can tell that
// nobody in the middle answered.
func (m *Mesh) pairDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if m.IssueDevice == nil || json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req) != nil || req.Code == "" {
		apperr.WriteHTTP(w, r, apperr.New(apperr.ForStatus(400), "bad request"))
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
		apperr.WriteHTTP(w, r, apperr.New(apperr.ForStatus(403), "wrong or expired code"))
		return
	}
	token, err := m.IssueDevice(r.Context(), req.Name)
	if err != nil {
		apperr.WriteHTTP(w, r, err)
		return
	}
	reply(w, map[string]string{"node_id": m.log.Node(), "name": m.name, "pin": m.id.DevicePin, "token": token, "proof": proof(code, m.id.DevicePin, "device")})
}

// --- client side ------------------------------------------------------------

// Run keeps every paired node in sync until ctx ends.
func (m *Mesh) Run(ctx context.Context) {
	peers, err := m.Peers(ctx)
	if err != nil {
		log.Printf("mesh: %v", err)
	}
	for _, p := range peers {
		if !p.Revoked && !p.Disabled {
			go m.Start(ctx, p.ID)
		}
	}
	go m.discover(ctx)
	go m.compactHourly(ctx)
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
	connected := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-timer.C:
		}
		synced, err := m.sync(ctx, node)
		if err != nil && ctx.Err() == nil {
			log.Printf("mesh: sync with %s: %v", node, err)
		}
		// A peer that just became reachable gets what was waiting for it.
		if synced && !connected && m.Connected != nil {
			m.Connected(node)
		}
		connected = synced
		timer.Reset(syncEvery)
	}
}

// listen holds the peer's change stream open and wakes the sync loop on
// each change.
func (m *Mesh) listen(ctx context.Context, node string, wake chan struct{}) {
	for ctx.Err() == nil {
		peer, err := m.peer(ctx, node)
		if err != nil {
			return
		}
		if peer.address != "" {
			c := peer.client()
			c.Timeout = 0
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+peer.address+peerPrefix+"/stream", nil)
			if err != nil {
				return
			}
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
func (m *Mesh) sync(ctx context.Context, node string) (bool, error) {
	peer, err := m.peer(ctx, node)
	if err != nil || peer.address == "" {
		return false, err
	}
	c := peer.client()
	base := "https://" + peer.address + peerPrefix
	compatible, err := m.checkProtocol(ctx, c, base, node, peer.address, peer.key)
	if err != nil || !compatible {
		return false, err
	}
	if err := m.exchange(ctx, c, base, node); err != nil {
		return false, err
	}
	m.compact(ctx)
	return true, nil
}

func getJSON(ctx context.Context, c *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return do(c, req, out)
}

func postJSON(ctx context.Context, c *http.Client, url string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
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
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return err
		}
	}
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}
