package peer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/migueel15/dory/node/syncx"
)

// Pairing defaults.
const (
	DefaultPairWindow  = 5 * time.Minute
	DefaultMaxFailures = 3
	pairVersion        = 1
	groupFile          = "group.json"
)

// Pairing errors.
var (
	ErrPairClosed   = errors.New("peer: pairing window is closed")
	ErrPairRejected = errors.New("peer: pairing rejected by the host")
)

// Offer is what A shows to B while the window is open.
type Offer struct {
	Payload string    // JSON to encode in the QR code
	Expires time.Time // end of the window
}

// PNG renders the QR code, size in pixels per side.
func (o Offer) PNG(size int) ([]byte, error) {
	return qrcode.Encode(o.Payload, qrcode.Medium, size)
}

// QRPayload is the content of the QR code.
type QRPayload struct {
	V       int      `json:"v"`
	GroupID string   `json:"group_id"`
	APubKey string   `json:"a_pubkey"`
	Addrs   []string `json:"addrs"`
	Token   string   `json:"token"`
}

// Pairing is the host side (A) of the QR pairing. Set the exported fields,
// assign it to Server.Pairing and call Start to open a window.
type Pairing struct {
	Identity *Identity
	Group    *Log       // the membership log shared with the Server
	Data     *syncx.Log // data log served as snapshot
	Addrs    []string   // host:port values B can dial
	// Confirm asks the user of A to accept the device. Nil rejects everyone.
	Confirm func(name, pubkeyHex string) bool
	// Persist is called after the new member is added to Group (optional).
	Persist func(*Log) error
	// MaxFailures is the number of bad attempts that close the window (3).
	MaxFailures int

	now      func() time.Time
	mu       sync.Mutex
	token    string
	expires  time.Time
	open     bool
	failures int
}

func (p *Pairing) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// Start opens a window (DefaultPairWindow if window <= 0) with a fresh
// single-use 128-bit token, replacing any previous one.
func (p *Pairing) Start(window time.Duration) (Offer, error) {
	if window <= 0 {
		window = DefaultPairWindow
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return Offer{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.token = hex.EncodeToString(raw)
	p.expires = p.clock().Add(window)
	p.open = true
	p.failures = 0
	b, _ := json.Marshal(QRPayload{V: pairVersion, GroupID: p.Group.GroupID,
		APubKey: p.Identity.PublicKeyHex(), Addrs: p.Addrs, Token: p.token})
	return Offer{Payload: string(b), Expires: p.expires}, nil
}

// Close shuts the window.
func (p *Pairing) Close() {
	p.mu.Lock()
	p.closeLocked()
	p.mu.Unlock()
}

func (p *Pairing) closeLocked() {
	p.open = false
	p.token = ""
}

// Open reports whether a window is currently open.
func (p *Pairing) Open() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.openLocked()
}

func (p *Pairing) openLocked() bool {
	if p.open && !p.clock().Before(p.expires) {
		p.closeLocked()
	}
	return p.open
}

type pairRequest struct {
	Token  string `json:"token"`
	PubKey string `json:"pubkey"`
	Name   string `json:"name"`
}

type pairResponse struct {
	Log      *Log         `json:"log"`
	Snapshot SnapshotBody `json:"snapshot"`
}

func (p *Pairing) fail(w http.ResponseWriter, code int, msg string) {
	http.Error(w, msg, code)
}

func (p *Pairing) handle(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		p.fail(w, 400, "tls required")
		return
	}
	cpub, ok := r.TLS.PeerCertificates[0].PublicKey.(ed25519.PublicKey)
	if !ok {
		p.fail(w, 400, "bad client key")
		return
	}
	var req pairRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		p.fail(w, 400, "bad request")
		return
	}
	if req.PubKey != hex.EncodeToString(cpub) || req.Name == "" {
		p.fail(w, 400, "pubkey must match the TLS certificate and name is required")
		return
	}

	p.mu.Lock()
	if !p.openLocked() {
		p.mu.Unlock()
		p.fail(w, 404, ErrPairClosed.Error())
		return
	}
	if p.token == "" || subtle.ConstantTimeCompare([]byte(req.Token), []byte(p.token)) != 1 {
		p.failures++
		max := p.MaxFailures
		if max <= 0 {
			max = DefaultMaxFailures
		}
		if p.failures >= max {
			p.closeLocked()
		}
		p.mu.Unlock()
		p.fail(w, 403, "invalid token")
		return
	}
	p.closeLocked() // single use: the token and the window end here
	p.mu.Unlock()

	if p.Confirm == nil || !p.Confirm(req.Name, req.PubKey) {
		p.fail(w, 403, ErrPairRejected.Error())
		return
	}
	if _, err := p.Group.Append(p.Identity, ActionAdd, cpub, req.Name); err != nil {
		p.fail(w, 409, err.Error())
		return
	}
	if p.Persist != nil {
		if err := p.Persist(p.Group); err != nil {
			p.fail(w, 500, err.Error())
			return
		}
	}
	resp := pairResponse{Log: p.Group.Clone(), Snapshot: SnapshotBody{Ops: []syncx.Op{}}}
	if p.Data != nil {
		vv, ops, err := p.Data.Snapshot(r.Context())
		if err != nil {
			p.fail(w, 500, err.Error())
			return
		}
		if ops == nil {
			ops = []syncx.Op{}
		}
		resp.Snapshot = SnapshotBody{Vector: vv, Ops: ops}
	}
	writeJSON(w, resp)
}

// JoinResult is what B receives after a successful pairing.
type JoinResult struct {
	Group    *Log         // verified membership log that includes B
	HostAddr string       // address that answered
	HostKey  string       // A's public key (hex)
	Snapshot SnapshotBody // A's data at pairing time
}

// ApplySnapshot loads the snapshot into the local data log.
func (j *JoinResult) ApplySnapshot(ctx context.Context, l *syncx.Log) error {
	ops := j.Snapshot.Ops
	for len(ops) > 0 {
		n := min(len(ops), syncx.DefaultBatch)
		if _, err := l.ApplyNew(ctx, ops[:n]); err != nil {
			return err
		}
		ops = ops[n:]
	}
	return nil
}

// ParseQR decodes and validates a QR payload.
func ParseQR(payload string) (QRPayload, error) {
	var q QRPayload
	if err := json.Unmarshal([]byte(payload), &q); err != nil {
		return q, fmt.Errorf("peer: bad QR payload: %w", err)
	}
	k, err := hex.DecodeString(q.APubKey)
	if q.V != pairVersion || err != nil || len(k) != ed25519.PublicKeySize ||
		q.Token == "" || q.GroupID == "" || len(q.Addrs) == 0 {
		return q, errors.New("peer: invalid QR payload")
	}
	return q, nil
}

// Join pairs this device (id, name) with the host described by the QR payload.
// The host's TLS key must equal the one in the QR. On success the caller must
// persist res.Group and apply the snapshot.
func Join(ctx context.Context, qrPayload string, id *Identity, name string) (*JoinResult, error) {
	q, err := ParseQR(qrPayload)
	if err != nil {
		return nil, err
	}
	apub, _ := hex.DecodeString(q.APubKey)
	cfg, err := id.pinnedClientTLS(apub)
	if err != nil {
		return nil, err
	}
	cfg.NextProtos = []string{"http/1.1"}
	hc := &http.Client{Timeout: 5 * time.Minute, Transport: &http.Transport{
		TLSClientConfig: cfg, DisableKeepAlives: true, TLSHandshakeTimeout: 10 * time.Second}}
	body, _ := json.Marshal(pairRequest{Token: q.Token, PubKey: id.PublicKeyHex(), Name: name})

	var lastErr error
	for _, addr := range q.Addrs {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+addr+PathPair, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := hc.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		res, err := readJoin(resp, q, id, addr)
		if err != nil {
			return nil, err
		}
		return res, nil
	}
	return nil, fmt.Errorf("peer: could not reach host: %w", lastErr)
}

func readJoin(resp *http.Response, q QRPayload, id *Identity, addr string) (*JoinResult, error) {
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("peer: pairing failed: %s %s", resp.Status, string(bytes.TrimSpace(b)))
	}
	var pr pairResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, err
	}
	if pr.Log == nil || pr.Log.GroupID != q.GroupID {
		return nil, errors.New("peer: host sent a log of another group")
	}
	if err := pr.Log.Verify(); err != nil {
		return nil, err
	}
	apub, _ := hex.DecodeString(q.APubKey)
	if !pr.Log.Allowed(id.PublicKey()) || !pr.Log.Allowed(apub) {
		return nil, errors.New("peer: log does not include both devices")
	}
	return &JoinResult{Group: pr.Log, HostAddr: addr, HostKey: q.APubKey, Snapshot: pr.Snapshot}, nil
}

// EnsureGroup loads the membership log from dir/group.json or, when the node
// belongs to no group, creates a new one with id as its first member and saves
// it. created reports whether a new group was made.
func EnsureGroup(dir string, id *Identity) (g *Log, created bool, err error) {
	path := filepath.Join(dir, groupFile)
	g, err = LoadLog(path)
	if err == nil {
		return g, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	if g, err = NewGroup(id); err != nil {
		return nil, false, err
	}
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, err
	}
	if err = g.SaveFile(path); err != nil {
		return nil, false, err
	}
	return g, true, nil
}

// SaveGroup stores the membership log where EnsureGroup finds it.
func SaveGroup(dir string, g *Log) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return g.SaveFile(filepath.Join(dir, groupFile))
}
