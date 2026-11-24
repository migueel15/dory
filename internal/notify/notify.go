// Package notify sends push notices when an agent finishes, fails or needs
// the user (requirements RF-42, RF-43). Thread state replicates, so the node
// that is always on (the server) notices the threads of every machine and
// pushes to the phones subscribed to it.
package notify

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
)

// Event kinds that produce a notice, with their text.
var titles = map[string]string{
	"thread.idle":         "El agente terminó",
	"thread.error":        "El agente falló",
	"thread.waiting":      "Un agente te necesita",
	"plan.ready":          "Plan listo para revisar",
	"plan.accepted":       "Tarea lista para implementar",
	"task.ready_to_merge": "Tarea lista para mergear",
}

// kinds lists the events in the order the settings show them.
var kinds = []string{"thread.idle", "thread.error", "thread.waiting", "plan.ready", "plan.accepted", "task.ready_to_merge"}

// fresh is how recent an event must be to notify. Older ones come from
// replication catching up and are already history.
const fresh = 2 * time.Minute

// retention is how long a read notice is kept.
const retention = 30 * 24 * time.Hour

// Column is the kanban column of a task and a stamp that changes when it
// reaches that column again.
type Column struct{ Name, Stamp string }

type Notifier struct {
	db      *sql.DB
	store   *data.Store
	node    string
	private string
	public  string
	// Columns returns the column of every open task; set by the server.
	Columns func(context.Context) (map[string]Column, error)

	// InfoChanged tells the peers this node's description changed.
	InfoChanged func()
	// PeerInfo returns the descriptions of the connected peers.
	PeerInfo func(context.Context) []json.RawMessage

	mu        sync.Mutex
	beats     map[string]beat
	announced string
	last      map[string]string
	columns   map[string]string
	pushed    map[string]bool
}

// New loads the VAPID keys of this node from dir, creating them once.
func New(ctx context.Context, store *data.Store, node, dir string) (*Notifier, error) {
	path := filepath.Join(dir, "vapid.json")
	var keys struct{ Private, Public string }
	raw, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(raw, &keys)
	}
	if err != nil {
		if keys.Private, keys.Public, err = webpush.GenerateVAPIDKeys(); err != nil {
			return nil, err
		}
		raw, _ = json.Marshal(keys)
		if err = os.WriteFile(path, raw, 0o600); err != nil {
			return nil, err
		}
	}
	n := &Notifier{db: store.LocalDB(), store: store, node: node, private: keys.Private, public: keys.Public,
		last: map[string]string{}, pushed: map[string]bool{}}
	// Start from the current states, so a restart does not notify old news.
	threads, err := store.Threads().List(ctx, data.Filter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	for _, t := range threads {
		n.last[t.ID] = t.Status
	}
	return n, nil
}

// Run archives the notices read more than 30 days ago, every hour.
func (n *Notifier) Run(ctx context.Context) {
	go n.expire(ctx)
	for {
		cutoff := time.Now().Add(-retention)
		if err := n.store.ArchiveReadNotices(ctx, cutoff); err != nil && ctx.Err() == nil {
			log.Printf("notify: retention: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

// Changed is called after a replicated write.
func (n *Notifier) Changed(entity, id string) {
	switch entity {
	case "plan":
		go n.checkPlan(id)
	case "notice":
		go n.deliver(id)
	}
}

// Github is called when GitHub data changed: a task may have reached the
// Ready to merge column.
func (n *Notifier) Github() {
	if n.Columns == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		now, err := n.Columns(ctx)
		if err != nil {
			return
		}
		n.mu.Lock()
		prev := n.columns
		n.columns = map[string]string{}
		for id, c := range now {
			n.columns[id] = c.Name
		}
		n.mu.Unlock()
		if prev == nil {
			return
		}
		for id, c := range now {
			if c.Name != "ready_to_merge" || prev[id] == "ready_to_merge" {
				continue
			}
			if t, err := n.store.Tasks().Get(ctx, id); err == nil {
				n.Emit(ctx, "task.ready_to_merge", id, c.Stamp, t.Title, "/tasks")
			}
		}
	}()
}

// ThreadState captures each local transition before the next one can replace it.
func (n *Notifier) ThreadState(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t, err := n.store.Threads().Get(ctx, id)
	if err != nil || t.NodeID != n.node || t.LastActivityAt == nil {
		return
	}
	n.mu.Lock()
	prev := n.last[id]
	n.last[id] = t.Status
	n.mu.Unlock()
	// The node that runs the thread raises the event; the others get the notice replicated.
	if prev == t.Status || prev != "working" && t.Status != "waiting" {
		return
	}
	kind := "thread." + t.Status
	if titles[kind] == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if kind == "thread.idle" && t.TaskID != nil && n.planPending(ctx, *t.TaskID) {
			kind = "plan.ready"
		}
		n.Emit(ctx, kind, t.ID, *t.LastActivityAt, t.Title, "/threads/"+t.ID)
	}()
}

// planPending says whether the latest plan of the task waits to be accepted.
func (n *Notifier) planPending(ctx context.Context, task string) bool {
	plans, err := n.store.Plans().List(ctx, data.Filter{Fields: map[string]any{"task_id": task}, Limit: 1000})
	if err != nil || len(plans) == 0 {
		return false
	}
	latest := plans[0]
	for _, p := range plans {
		if p.Version > latest.Version {
			latest = p
		}
	}
	return latest.AcceptedAt == nil
}

func (n *Notifier) checkPlan(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, err := n.store.Plans().Get(ctx, id)
	if err != nil || p.AcceptedAt == nil {
		return
	}
	at, err := time.Parse(time.RFC3339Nano, *p.AcceptedAt)
	if err != nil || time.Since(at) > fresh {
		return
	}
	body := ""
	if t, err := n.store.Tasks().Get(ctx, p.TaskID); err == nil {
		body = t.Title
	}
	n.Emit(ctx, "plan.accepted", p.ID, *p.AcceptedAt, body, "/tasks")
}

// Emit stores one notice in the inbox. Its id derives from the event, so a
// node that sees the same event again, or another node that sees it too,
// finds the notice already there and writes nothing.
func (n *Notifier) Emit(ctx context.Context, kind, subject, stamp, body, url string) {
	title := titles[kind]
	if title == "" {
		return
	}
	if n.Mode(ctx, kind) == ModeOff {
		return
	}
	sum := sha256.Sum256([]byte(kind + "\x00" + subject + "\x00" + stamp))
	id := hex.EncodeToString(sum[:16])
	if _, err := n.store.Notices().Create(ctx, data.Notice{ID: id, Kind: kind, Title: title, Body: body, URL: url}); err != nil && !errors.Is(err, data.ErrConflict) {
		log.Printf("notify: %v", err)
	}
}

// deliver pushes a new unread notice once. It runs on every node, but only
// the ones with subscriptions have anyone to push to.
func (n *Notifier) deliver(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	nt, err := n.store.Notices().Get(ctx, id)
	if err != nil || nt.ReadAt != nil {
		return
	}
	if at, err := time.Parse(time.RFC3339Nano, nt.CreatedAt); err != nil || time.Since(at) > fresh {
		return
	}
	// With an interface in focus anywhere in the mesh the user is already at
	// Dory, so the notice shows there and no device gets a push.
	switch n.Mode(ctx, nt.Kind) {
	case ModeOff, ModeApp:
		return
	case ModeAuto:
		if n.Delivery(ctx).Focus {
			return
		}
	}
	n.mu.Lock()
	done := n.pushed[id]
	n.pushed[id] = true
	n.mu.Unlock()
	if !done {
		n.Send(ctx, nt.Title, nt.Body, nt.URL, nt.ID)
	}
}

// Send pushes one notice to every subscription; dead ones are dropped.
func (n *Notifier) Send(ctx context.Context, title, body, url, tag string) {
	rows, err := n.db.QueryContext(ctx, `SELECT endpoint, subscription FROM push_subscriptions`)
	if err != nil {
		log.Printf("notify: %v", err)
		return
	}
	type target struct{ endpoint, raw string }
	var targets []target
	for rows.Next() {
		var t target
		if rows.Scan(&t.endpoint, &t.raw) == nil {
			targets = append(targets, t)
		}
	}
	rows.Close()
	payload, _ := json.Marshal(map[string]string{"title": title, "body": body, "url": url, "tag": tag})
	for _, t := range targets {
		var sub webpush.Subscription
		if json.Unmarshal([]byte(t.raw), &sub) != nil {
			continue
		}
		resp, err := webpush.SendNotificationWithContext(ctx, payload, &sub, &webpush.Options{Subscriber: "https://dorlab.net",
			VAPIDPrivateKey: n.private, VAPIDPublicKey: n.public, TTL: 86400, HTTPClient: &http.Client{Timeout: 10 * time.Second}})
		if err != nil {
			log.Printf("notify: push: %v", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			n.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE endpoint = ?`, t.endpoint)
		}
	}
}

// Mount adds the subscription routes.
func (n *Notifier) Mount(mux *http.ServeMux) {
	n.mountPresence(mux)
	n.mountSettings(mux)
	mux.HandleFunc("GET /api/push/key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": n.public})
	})
	mux.HandleFunc("POST /api/push/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		var sub webpush.Subscription
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&sub); err != nil || sub.Endpoint == "" {
			apperr.WriteHTTP(w, r, apperr.New(apperr.Invalid, "invalid subscription"))
			return
		}
		raw, _ := json.Marshal(sub)
		_, err := n.db.ExecContext(r.Context(), `INSERT INTO push_subscriptions(endpoint, subscription, created_at) VALUES (?, ?, ?)
			ON CONFLICT(endpoint) DO UPDATE SET subscription = excluded.subscription`, sub.Endpoint, string(raw), time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			apperr.WriteHTTP(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		if n.InfoChanged != nil {
			n.InfoChanged()
		}
	})
	mux.HandleFunc("GET /api/notices", func(w http.ResponseWriter, r *http.Request) {
		list, err := n.store.Notices().List(r.Context(), data.Filter{Limit: 1000})
		if err != nil {
			apperr.WriteHTTP(w, r, err)
			return
		}
		slices.Reverse(list)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	})
	// POST /api/notices/read marks the given ids, or every notice when ids is empty, as read.
	mux.HandleFunc("POST /api/notices/read", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IDs []string `json:"ids"`
		}
		json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)
		list, err := n.store.Notices().List(r.Context(), data.Filter{Limit: 1000})
		if err != nil {
			apperr.WriteHTTP(w, r, err)
			return
		}
		at := time.Now().UTC().Format(time.RFC3339Nano)
		for _, nt := range list {
			if nt.ReadAt == nil && (len(body.IDs) == 0 || slices.Contains(body.IDs, nt.ID)) {
				if _, err := n.store.Notices().Patch(r.Context(), nt.ID, data.Fields{"read_at": at}); err != nil && !errors.Is(err, data.ErrNotFound) {
					apperr.WriteHTTP(w, r, err)
					return
				}
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/push/test", func(w http.ResponseWriter, r *http.Request) {
		n.Send(r.Context(), "Dory", "Los avisos funcionan en este dispositivo.", "/", "test")
		w.WriteHeader(http.StatusNoContent)
	})
}
