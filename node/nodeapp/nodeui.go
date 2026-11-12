package nodeapp

import (
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/node/localstore"
	"github.com/migueel15/dory/node/peer"
)

//go:embed ui
var uiFS embed.FS

const (
	nodePrefix    = "/node/"
	nodeAPIPrefix = "/node/api/"
	// csrfHeader must be present on every mutation: a custom header cannot be
	// sent cross-origin without a CORS preflight, which this API never grants.
	csrfHeader = "X-Dory-Node"
	promptTTL  = 2 * time.Minute
)

// prompts holds the one pending "accept this device?" question.
type prompts struct {
	mu      sync.Mutex
	pending *pendingJoin
	offer   *peer.Offer
}

type pendingJoin struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	PubKey string `json:"public_key"`
	answer chan bool
}

func newPrompts() *prompts { return &prompts{} }

// ask is the peer.Pairing Confirm callback: it blocks until the UI answers or
// the prompt expires. Only one question is open at a time.
func (p *prompts) ask(name, pub string) bool {
	p.mu.Lock()
	if p.pending != nil {
		p.mu.Unlock()
		return false
	}
	pj := &pendingJoin{ID: randomSecret()[:12], Name: name, PubKey: pub, answer: make(chan bool, 1)}
	p.pending = pj
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		if p.pending == pj {
			p.pending = nil
		}
		p.mu.Unlock()
	}()
	select {
	case ok := <-pj.answer:
		return ok
	case <-time.After(promptTTL):
		return false
	}
}

func (p *prompts) current() *pendingJoin {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pending
}

func (p *prompts) decide(id string, accept bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending == nil || p.pending.ID != id {
		return false
	}
	select {
	case p.pending.answer <- accept:
	default:
	}
	return true
}

// mountNodeUI registers the /node/ pages and the /node/api/ JSON endpoints.
// They are reachable only through the loopback listener or the in-process
// handler: the peer link never routes here (ownerProxy only forwards /api/t3/).
func (n *Node) mountNodeUI(mux *http.ServeMux, svc service.Service, session string) {
	sub, _ := fs.Sub(uiFS, "ui")
	pages := autoLogin(svc, session, http.StripPrefix("/node", http.FileServerFS(sub)))
	mux.Handle(nodePrefix, localOnly(pages))
	api := n.nodeAPI()
	mux.Handle(nodeAPIPrefix, localOnly(guard(svc, api)))
	mux.Handle("/node", http.RedirectHandler(nodePrefix, http.StatusMovedPermanently))
}

// localOnly rejects requests whose Host is not a loopback name (DNS rebinding).
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		ip := net.ParseIP(host)
		if host != "localhost" && !strings.HasSuffix(host, ".localhost") && (ip == nil || !ip.IsLoopback()) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// guard requires the local session cookie or the local bearer token, and on
// mutations the CSRF header.
func guard(svc service.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := false
		if h := r.Header.Get("Authorization"); h != "" {
			if f := strings.Fields(h); len(f) == 2 && strings.EqualFold(f[0], "Bearer") && len(f[1]) <= 256 {
				_, err := svc.FindToken(r.Context(), digest(f[1]))
				ok = err == nil
			}
		} else if c, err := r.Cookie(sessionCookie); err == nil {
			_, err = svc.FindSession(r.Context(), digest(c.Value))
			ok = err == nil
		}
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("unauthorized"))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" || subtle.ConstantTimeCompare([]byte(r.Header.Get(csrfHeader)), []byte("1")) != 1 {
				writeJSON(w, http.StatusForbidden, errBody("forbidden"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func errBody(msg string) map[string]string { return map[string]string{"error": msg} }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

type memberView struct {
	DeviceID  string `json:"device_id"`
	PublicKey string `json:"public_key"`
	Name      string `json:"name"`
	Self      bool   `json:"self"`
	State     string `json:"state"`
	AgoSecs   int    `json:"ago_seconds,omitempty"`
	Manual    string `json:"manual,omitempty"`
}

func (n *Node) nodeAPI() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /node/api/state", func(w http.ResponseWriter, r *http.Request) {
		self := n.Identity.DeviceID()
		var members []memberView
		for _, m := range n.Group.Members() {
			v := memberView{DeviceID: m.DeviceID, PublicKey: m.PublicKey, Name: m.Name, Self: m.DeviceID == self, State: "self"}
			if rec, ok, _ := n.Store.AddressOf(r.Context(), m.DeviceID); ok {
				v.Manual = rec.Manual
				if rec.Name != "" {
					v.Name = rec.Name
				}
			}
			if v.Self {
				v.Name = n.Identity.Name
			} else {
				st := n.Presence.Status(m.DeviceID)
				v.State, v.AgoSecs = string(st.State), int(st.Ago.Seconds())
			}
			members = append(members, v)
		}
		out := map[string]any{
			"device":  map[string]any{"device_id": self, "name": n.Identity.Name, "peer_addrs": n.localAddrs()},
			"members": members,
			"pairing": map[string]any{"open": n.Pairing.Open()},
		}
		if rec, ok, _ := n.Store.AddressOf(r.Context(), self); ok {
			out["device"].(map[string]any)["manual"] = rec.Manual
		}
		if pj := n.prompts.current(); pj != nil {
			out["pending"] = pj
		}
		writeJSON(w, http.StatusOK, out)
	})

	mux.HandleFunc("POST /node/api/pair", func(w http.ResponseWriter, r *http.Request) {
		offer, err := n.Pairing.Start(0)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
			return
		}
		png, err := offer.PNG(320)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"payload": offer.Payload, "expires": offer.Expires,
			"png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		})
	})
	mux.HandleFunc("DELETE /node/api/pair", func(w http.ResponseWriter, r *http.Request) {
		n.Pairing.Close()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /node/api/pending", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     string `json:"id"`
			Accept bool   `json:"accept"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil || !n.prompts.decide(in.ID, in.Accept) {
			writeJSON(w, http.StatusNotFound, errBody("no such pending request"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /node/api/members/{device}/revoke", func(w http.ResponseWriter, r *http.Request) {
		dev := r.PathValue("device")
		if dev == n.Identity.DeviceID() {
			writeJSON(w, http.StatusBadRequest, errBody("cannot revoke this device"))
			return
		}
		for _, m := range n.Group.Members() {
			if m.DeviceID != dev {
				continue
			}
			pub, err := hex.DecodeString(m.PublicKey)
			if err == nil {
				_, err = n.Group.Append(n.Identity, peer.ActionRevoke, pub, "")
			}
			if err == nil {
				err = peer.SaveGroup(n.dir, n.Group)
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		writeJSON(w, http.StatusNotFound, errBody("not a member"))
	})
	mux.HandleFunc("PUT /node/api/address", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Manual string `json:"manual"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, errBody("invalid json"))
			return
		}
		in.Manual = strings.TrimSpace(in.Manual)
		if in.Manual != "" {
			if _, _, err := net.SplitHostPort(in.Manual); err != nil {
				writeJSON(w, http.StatusBadRequest, errBody("expected host:port"))
				return
			}
		}
		if err := n.republish(r, func(a *localstore.DeviceAddress) { a.Manual = in.Manual }); err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("PUT /node/api/device", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, errBody("invalid json"))
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" || len(in.Name) > 80 {
			writeJSON(w, http.StatusBadRequest, errBody("name must be 1-80 characters"))
			return
		}
		if err := n.Identity.SetName(in.Name); err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
			return
		}
		if err := n.republish(r, func(a *localstore.DeviceAddress) { a.Name = in.Name }); err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	return mux
}

// republish rewrites this device's address record after edit.
func (n *Node) republish(r *http.Request, edit func(*localstore.DeviceAddress)) error {
	self := n.Identity.DeviceID()
	rec, _, err := n.Store.AddressOf(r.Context(), self)
	if err != nil {
		return err
	}
	rec.DeviceID, rec.Name = self, n.Identity.Name
	rec.Local = n.localAddrs()
	edit(&rec)
	return n.Store.PublishAddress(r.Context(), rec)
}
