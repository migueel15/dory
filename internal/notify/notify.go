// Package notify sends push notices when an agent finishes, fails or needs
// the user (requirements RF-42, RF-43). Thread state replicates, so the node
// that is always on (the server) notices the threads of every machine and
// pushes to the phones subscribed to it.
package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/migueel15/dory/internal/data"
)

type Notifier struct {
	db      *sql.DB
	store   *data.Store
	private string
	public  string

	mu   sync.Mutex
	last map[string]string
}

// New loads the VAPID keys of this node from dir, creating them once.
func New(ctx context.Context, store *data.Store, dir string) (*Notifier, error) {
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
	n := &Notifier{db: store.DB(), store: store, private: keys.Private, public: keys.Public, last: map[string]string{}}
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

// Changed is called after every write; it looks at thread state changes.
func (n *Notifier) Changed(entity, id string) {
	if entity != "thread" {
		return
	}
	go n.check(id)
}

func (n *Notifier) check(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t, err := n.store.Threads().Get(ctx, id)
	if err != nil {
		return
	}
	n.mu.Lock()
	prev := n.last[id]
	n.last[id] = t.Status
	n.mu.Unlock()
	if prev == t.Status || prev != "working" && t.Status != "waiting" {
		return
	}
	title := map[string]string{"idle": "El agente terminó", "waiting": "Un agente te necesita", "error": "El agente falló"}[t.Status]
	if title == "" {
		return
	}
	n.Send(ctx, title, t.Title, "/threads/"+t.ID, t.ID)
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
	mux.HandleFunc("GET /api/push/key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": n.public})
	})
	mux.HandleFunc("POST /api/push/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		var sub webpush.Subscription
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&sub); err != nil || sub.Endpoint == "" {
			http.Error(w, "invalid subscription", http.StatusBadRequest)
			return
		}
		raw, _ := json.Marshal(sub)
		_, err := n.db.ExecContext(r.Context(), `INSERT INTO push_subscriptions(endpoint, subscription, created_at) VALUES (?, ?, ?)
			ON CONFLICT(endpoint) DO UPDATE SET subscription = excluded.subscription`, sub.Endpoint, string(raw), time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/push/test", func(w http.ResponseWriter, r *http.Request) {
		n.Send(r.Context(), "Dory", "Los avisos funcionan en este dispositivo.", "/", "test")
		w.WriteHeader(http.StatusNoContent)
	})
}
