package notify

import (
	"context"
	"encoding/json"
	"github.com/migueel15/dory/internal/apperr"
	"net/http"
	"time"
)

// Interface states, from least to most attention.
const (
	Closed = "closed"
	Open   = "open"
	Focus  = "focus"
)

// beatTTL is how long an interface counts as open after its last heartbeat,
// so an abrupt close (a lost connection, a killed app) expires by itself.
const beatTTL = 45 * time.Second

type beat struct {
	state string
	at    time.Time
}

// Delivery is what the mesh knows about where the user is: whether some
// interface has the focus, whether any is open, and whether some node has
// subscriptions to push to.
type Delivery struct {
	Focus bool `json:"focus"`
	Open  bool `json:"open"`
	Push  bool `json:"push"`
}

// Beat records the state of one interface of this node. A "closed" beat, or
// no beat for beatTTL, removes it.
func (n *Notifier) Beat(id, state string) {
	n.mu.Lock()
	if n.beats == nil {
		n.beats = map[string]beat{}
	}
	if state == Closed {
		delete(n.beats, id)
	} else {
		n.beats[id] = beat{state, time.Now()}
	}
	n.mu.Unlock()
	n.presenceChanged()
}

// Presence is the best state among the live interfaces of this node.
func (n *Notifier) Presence() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	best := Closed
	for id, b := range n.beats {
		if time.Since(b.at) > beatTTL {
			delete(n.beats, id)
			continue
		}
		if b.state == Focus || best == Closed {
			best = b.state
		}
	}
	return best
}

// HasPush says whether this node holds push subscriptions.
func (n *Notifier) HasPush(ctx context.Context) bool {
	var count int
	n.db.QueryRowContext(ctx, `SELECT count(*) FROM push_subscriptions`).Scan(&count)
	return count > 0
}

// Info is what this node tells its peers about the user's interfaces.
func (n *Notifier) Info(ctx context.Context) map[string]any {
	return map[string]any{"presence": n.Presence(), "push": n.HasPush(ctx)}
}

// presenceChanged tells the peers when the state of this node changed.
func (n *Notifier) presenceChanged() {
	state := n.Presence()
	n.mu.Lock()
	changed := state != n.announced
	n.announced = state
	n.mu.Unlock()
	if changed && n.InfoChanged != nil {
		n.InfoChanged()
	}
}

// Delivery combines this node with the connected peers. A peer that is not
// connected counts as closed, like an interface whose heartbeat expired.
func (n *Notifier) Delivery(ctx context.Context) Delivery {
	state, push := n.Presence(), n.HasPush(ctx)
	if n.PeerInfo != nil {
		for _, raw := range n.PeerInfo(ctx) {
			var info struct {
				Presence string `json:"presence"`
				Push     bool   `json:"push"`
			}
			if json.Unmarshal(raw, &info) != nil {
				continue
			}
			push = push || info.Push
			if info.Presence == Focus || info.Presence == Open && state == Closed {
				state = info.Presence
			}
		}
	}
	return Delivery{Focus: state == Focus, Open: state != Closed, Push: push}
}

// expire announces the loss of an interface whose heartbeat stopped.
func (n *Notifier) expire(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n.presenceChanged()
		}
	}
}

func (n *Notifier) mountPresence(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/presence", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ ID, State string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil || body.ID == "" ||
			body.State != Closed && body.State != Open && body.State != Focus {
			apperr.WriteHTTP(w, r, apperr.New(apperr.Invalid, "invalid presence"))
			return
		}
		n.Beat(body.ID, body.State)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/presence", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(n.Delivery(r.Context()))
	})
}
