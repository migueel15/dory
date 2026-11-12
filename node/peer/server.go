package peer

import (
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/node/syncx"
)

// DefaultPort is the TCP port of the peer listener.
const DefaultPort = 7710

// Peer API paths.
const (
	PathVector   = "/peer/v1/vector"
	PathOps      = "/peer/v1/ops"
	PathStream   = "/peer/v1/ops/stream"
	PathPresence = "/peer/v1/presence"
	PathSnapshot = "/peer/v1/snapshot"
	PathPair     = "/peer/v1/pair"
)

// Server serves the peer API over mutual TLS 1.3 to group members only.
type Server struct {
	Identity  *Identity
	Members   Membership
	Log       *syncx.Log
	Presence  *Presence // optional; created if nil
	Intervals Intervals
	// Pairing, if set, enables the pairing route while its window is open.
	Pairing *Pairing

	mu  sync.Mutex
	srv *http.Server
}

// Handler is the peer API without TLS, for tests and embedding.
func (s *Server) Handler() http.Handler {
	if s.Presence == nil {
		s.Presence = NewPresence(s.Intervals)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+PathVector, s.vector)
	mux.HandleFunc("GET "+PathOps, s.ops)
	mux.HandleFunc("GET "+PathStream, s.stream)
	mux.HandleFunc("GET "+PathPresence, s.presence)
	mux.HandleFunc("GET "+PathSnapshot, s.snapshot)
	if s.Pairing != nil {
		mux.HandleFunc("POST "+PathPair, s.Pairing.handle)
	}
	return s.membersOnly(mux)
}

// membersOnly rejects TLS clients that are not members on every route but the
// pairing one (non-members only get past the handshake while a window is open).
func (s *Server) membersOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && r.URL.Path != PathPair {
			ok := false
			if len(r.TLS.PeerCertificates) > 0 {
				pub, isEd := r.TLS.PeerCertificates[0].PublicKey.(ed25519.PublicKey)
				ok = isEd && s.Members.Allowed(pub)
			}
			if !ok {
				http.Error(w, ErrNotMember.Error(), http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Serve accepts connections on ln until Close. It returns nil after Close.
func (s *Server) Serve(ln net.Listener) error {
	var open func() bool
	if s.Pairing != nil {
		open = s.Pairing.Open
	}
	cfg, err := s.Identity.ServerTLSGated(s.Members, open)
	if err != nil {
		return err
	}
	cfg.NextProtos = []string{"http/1.1"}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	s.mu.Lock()
	s.srv = srv
	s.mu.Unlock()
	err = srv.Serve(tls.NewListener(ln, cfg))
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// ListenAndServe listens on addr (":7710" if empty).
func (s *Server) ListenAndServe(addr string) error {
	if addr == "" {
		addr = fmt.Sprintf(":%d", DefaultPort)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Close stops the listener and drops every open connection.
func (s *Server) Close() error {
	s.mu.Lock()
	srv := s.srv
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Close()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) vector(w http.ResponseWriter, r *http.Request) {
	vv, err := s.Log.Vector(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, vv)
}

func (s *Server) ops(w http.ResponseWriter, r *http.Request) {
	since, err := ParseVector(r.URL.Query().Get("since"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ops, err := s.Log.Since(r.Context(), since, limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if ops == nil {
		ops = []syncx.Op{}
	}
	writeJSON(w, ops)
}

// SnapshotBody is the response of the snapshot endpoint.
type SnapshotBody struct {
	Vector syncx.VersionVector `json:"vector"`
	Ops    []syncx.Op          `json:"ops"`
}

func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	vv, ops, err := s.Log.Snapshot(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if ops == nil {
		ops = []syncx.Op{}
	}
	writeJSON(w, SnapshotBody{vv, ops})
}

// PresenceBody is the response of the presence endpoint.
type PresenceBody struct {
	DeviceID string   `json:"device_id"`
	Name     string   `json:"name"`
	Now      string   `json:"now"`
	Peers    []Status `json:"peers"`
}

func (s *Server) presence(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, PresenceBody{s.Identity.DeviceID(), s.Identity.Name,
		time.Now().UTC().Format(time.RFC3339Nano), s.Presence.All()})
}

// stream is a Server-Sent Events feed: "ops" events carry a JSON batch,
// "heartbeat" events keep the link observable.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	since, err := ParseVector(r.URL.Query().Get("since"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	ctx := r.Context()
	remote := peerDevice(r.TLS)
	if remote != "" {
		s.Presence.Up(remote)
		defer s.Presence.Down(remote)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(200)
	send := func(event string, data []byte) bool {
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		fl.Flush()
		if remote != "" {
			s.Presence.Beat(remote)
		}
		return true
	}
	if !send("heartbeat", []byte("{}")) {
		return
	}
	pos := syncx.VersionVector{}
	for d, n := range since {
		pos[d] = n
	}
	wake, cancel := s.Log.Watch()
	defer cancel()
	tick := time.NewTicker(s.Intervals.heartbeat())
	defer tick.Stop()
	for {
		for {
			ops, err := s.Log.Since(ctx, pos, syncx.DefaultBatch)
			if err != nil || len(ops) == 0 {
				break
			}
			moved := false
			for _, o := range ops {
				if o.Seq == pos[o.DeviceID]+1 {
					pos[o.DeviceID] = o.Seq
					moved = true
				}
			}
			b, _ := json.Marshal(ops)
			if !send("ops", b) {
				return
			}
			if !moved {
				break
			}
		}
		select {
		case <-wake:
		case <-tick.C:
			if !send("heartbeat", []byte("{}")) {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// FormatVector encodes a version vector as "dev:seq,dev:seq".
func FormatVector(vv syncx.VersionVector) string {
	parts := make([]string, 0, len(vv))
	for d, n := range vv {
		parts = append(parts, d+":"+strconv.FormatInt(n, 10))
	}
	return strings.Join(parts, ",")
}

// ParseVector decodes FormatVector.
func ParseVector(s string) (syncx.VersionVector, error) {
	vv := syncx.VersionVector{}
	if s == "" {
		return vv, nil
	}
	for _, p := range strings.Split(s, ",") {
		i := strings.LastIndexByte(p, ':')
		if i < 1 {
			return nil, fmt.Errorf("peer: bad vector entry %q", p)
		}
		n, err := strconv.ParseInt(p[i+1:], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("peer: bad vector entry %q", p)
		}
		vv[p[:i]] = n
	}
	return vv, nil
}
