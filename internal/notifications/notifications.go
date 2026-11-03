// Package notifications persists global per-user delivery independently of clients.
package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"github.com/jackc/pgx/v5"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/t3"
)

type Manager struct {
	Pool            *pgxpool.Pool
	private, public string
}

func New(ctx context.Context, pool *pgxpool.Pool) (*Manager, error) {
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return nil, err
	}
	_, err = pool.Exec(ctx, "INSERT INTO notification_keys(private_key,public_key) VALUES($1,$2) ON CONFLICT DO NOTHING", private, public)
	if err != nil {
		return nil, err
	}
	m := &Manager{Pool: pool}
	err = pool.QueryRow(ctx, "SELECT private_key,public_key FROM notification_keys").Scan(&m.private, &m.public)
	return m, err
}

type threadState struct {
	Status                        string
	Attention                     bool
	RunID, CompletedAt, RequestID string
	FinishedRun                   string
}

func (m *Manager) Observe(ctx context.Context, instance string, threads []t3.Thread) error {
	tx, err := m.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", instance); err != nil {
		return err
	}
	var initialized bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM notification_instances WHERE instance_id=$1)", instance).Scan(&initialized); err != nil {
		return err
	}
	for _, t := range threads {
		state := threadState{Status: t.Status, Attention: t.Attention, RunID: t.RunID, CompletedAt: t.CompletedAt, RequestID: t.RequestID}

		var oldRaw []byte
		if err = tx.QueryRow(ctx, "SELECT state FROM notification_threads WHERE instance_id=$1 AND thread_id=$2", instance, t.ID).Scan(&oldRaw); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var old threadState
		if len(oldRaw) > 0 || initialized {
			if len(oldRaw) > 0 {
				if err = json.Unmarshal(oldRaw, &old); err != nil {
					return err
				}
			}
			state.FinishedRun = old.FinishedRun
			kinds := []string{}
			finished := state.CompletedAt != "" && (state.CompletedAt != old.CompletedAt || state.RunID != old.RunID) || terminal(state.Status) && (state.Status != old.Status || state.RunID != old.RunID)
			attention := state.Attention && (!old.Attention || state.Status != old.Status || state.RequestID != old.RequestID || state.RunID != old.RunID)
			if finished && state.RunID != "" && old.FinishedRun == state.RunID {
				finished = false
			}
			if finished {
				state.FinishedRun = state.RunID
				kinds = append(kinds, "thread_finished")
			}
			if attention || finished && state.Status == "failed" {
				kinds = append(kinds, "thread_attention")
			}
			if len(kinds) > 0 {
				heading := "Hilo terminado"
				if len(kinds) > 1 || attention {
					heading = "Requiere atención"
				}
				var project string
				err = tx.QueryRow(ctx, "SELECT p.name FROM item_threads it JOIN items i ON i.id=it.item_id JOIN projects p ON p.id=i.project_id WHERE it.instance_id=$1 AND it.thread_id=$2 AND it.archived_at IS NULL ORDER BY it.created_at DESC LIMIT 1", instance, t.ID).Scan(&project)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				if project != "" {
					heading += " · " + project
				}
				if _, err = tx.Exec(ctx, "SELECT emit_notification($1,$2,$3,$4)", heading, t.Title, t.URL, kinds); err != nil {
					return err
				}
			}
		}
		if len(oldRaw) == 0 && !initialized && terminal(state.Status) {
			state.FinishedRun = state.RunID
		}
		raw, _ := json.Marshal(state)
		if _, err = tx.Exec(ctx, "INSERT INTO notification_threads VALUES($1,$2,$3) ON CONFLICT(instance_id,thread_id) DO UPDATE SET state=excluded.state", instance, t.ID, raw); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO notification_instances VALUES($1) ON CONFLICT DO NOTHING", instance); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func terminal(s string) bool {
	return s == "completed" || s == "failed" || s == "interrupted" || s == "cancelled"
}
func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}
func (m *Manager) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/notifications", func(w http.ResponseWriter, r *http.Request) {
		before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
		rows, err := m.Pool.Query(r.Context(), "SELECT id,title,body,url,kinds,bell,toast,read_at IS NOT NULL,created_at FROM notifications WHERE user_id=$1 AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT 100", auth.Actor(r.Context()), before)
		if err != nil {
			http.Error(w, "notifications unavailable", 500)
			return
		}
		defer rows.Close()
		type notice struct {
			ID      int64     `json:"id"`
			Title   string    `json:"title"`
			Body    string    `json:"body"`
			URL     string    `json:"url"`
			Kinds   []string  `json:"kinds"`
			Bell    bool      `json:"bell"`
			Toast   bool      `json:"toast"`
			Read    bool      `json:"read"`
			Created time.Time `json:"created_at"`
		}
		out := []notice{}
		for rows.Next() {
			var n notice
			if err = rows.Scan(&n.ID, &n.Title, &n.Body, &n.URL, &n.Kinds, &n.Bell, &n.Toast, &n.Read, &n.Created); err != nil {
				http.Error(w, "read failed", 500)
				return
			}
			out = append(out, n)
		}
		if rows.Err() != nil {
			http.Error(w, "read failed", 500)
			return
		}
		var unread int
		if err = m.Pool.QueryRow(r.Context(), "SELECT count(*) FROM notifications WHERE user_id=$1 AND bell AND read_at IS NULL", auth.Actor(r.Context())).Scan(&unread); err != nil {
			http.Error(w, "read failed", 500)
			return
		}
		reply(w, map[string]any{"items": out, "unread": unread, "server_time": time.Now()})
	})
	mux.HandleFunc("DELETE /api/notifications", func(w http.ResponseWriter, r *http.Request) {
		if _, err := m.Pool.Exec(r.Context(), "DELETE FROM notifications WHERE user_id=$1", auth.Actor(r.Context())); err != nil {
			http.Error(w, "write failed", 500)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/notifications/{id}/read", func(w http.ResponseWriter, r *http.Request) {
		_, err := m.Pool.Exec(r.Context(), "UPDATE notifications SET read_at=now() WHERE user_id=$1 AND id::text=$2", auth.Actor(r.Context()), r.PathValue("id"))
		if err != nil {
			http.Error(w, "write failed", 500)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /api/notifications/preferences", func(w http.ResponseWriter, r *http.Request) {
		var raw json.RawMessage
		err := m.Pool.QueryRow(r.Context(), "SELECT COALESCE((SELECT settings FROM notification_preferences WHERE user_id=$1),'{}')", auth.Actor(r.Context())).Scan(&raw)
		if err != nil {
			http.Error(w, "read failed", 500)
			return
		}
		reply(w, map[string]any{"settings": raw, "public_key": m.public})
	})
	mux.HandleFunc("PUT /api/notifications/preferences", func(w http.ResponseWriter, r *http.Request) {
		var settings map[string]map[string]bool
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&settings) != nil {
			http.Error(w, "invalid settings", 400)
			return
		}
		for k, s := range settings {
			if k != "item_ready" && k != "item_blocked" && k != "thread_finished" && k != "thread_attention" {
				http.Error(w, "invalid event", 400)
				return
			}
			for c := range s {
				if c != "enabled" && c != "bell" && c != "toast" && c != "push" {
					http.Error(w, "invalid channel", 400)
					return
				}
			}
		}
		raw, _ := json.Marshal(settings)
		_, err := m.Pool.Exec(r.Context(), "INSERT INTO notification_preferences VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET settings=excluded.settings", auth.Actor(r.Context()), raw)
		if err != nil {
			http.Error(w, "write failed", 500)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/notifications/subscription", func(w http.ResponseWriter, r *http.Request) {
		var sub webpush.Subscription
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&sub) != nil {
			http.Error(w, "invalid subscription", 400)
			return
		}
		u, err := url.Parse(sub.Endpoint)
		// Browser push endpoints only; never allow arbitrary internal HTTP requests.
		if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || !(u.Hostname() == "fcm.googleapis.com" || u.Hostname() == "updates.push.services.mozilla.com" || strings.HasSuffix(u.Hostname(), ".notify.windows.com") || u.Hostname() == "web.push.apple.com") || sub.Keys.Auth == "" || sub.Keys.P256dh == "" {
			http.Error(w, "unsupported push endpoint", 400)
			return
		}
		raw, _ := json.Marshal(sub)
		_, err = m.Pool.Exec(r.Context(), "INSERT INTO notification_subscriptions VALUES($1,$2,$3) ON CONFLICT(endpoint) DO UPDATE SET subscription=excluded.subscription WHERE notification_subscriptions.user_id=excluded.user_id", auth.Actor(r.Context()), sub.Endpoint, raw)
		if err != nil {
			http.Error(w, "write failed", 500)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("DELETE /api/notifications/subscription", func(w http.ResponseWriter, r *http.Request) {
		var sub struct {
			Endpoint string `json:"endpoint"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&sub) != nil {
			http.Error(w, "invalid subscription", 400)
			return
		}
		_, err := m.Pool.Exec(r.Context(), "DELETE FROM notification_subscriptions WHERE user_id=$1 AND endpoint=$2", auth.Actor(r.Context()), sub.Endpoint)
		if err != nil {
			http.Error(w, "write failed", 500)
			return
		}
		w.WriteHeader(204)
	})
}
func (m *Manager) Run(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := m.deliver(ctx); err != nil {
				log.Printf("notifications: %v", err)
			}
		}
	}
}
func (m *Manager) deliver(ctx context.Context) error {
	rows, err := m.Pool.Query(ctx, "SELECT p.notification_id,p.endpoint,s.subscription,n.title,n.body,n.url FROM notification_push p JOIN notifications n ON n.id=p.notification_id JOIN notification_subscriptions s ON s.endpoint=p.endpoint WHERE p.next_at<=now() ORDER BY p.notification_id LIMIT 50")
	if err != nil {
		return err
	}
	type job struct {
		id         int64
		endpoint   string
		raw        []byte
		title, body, url string
	}
	jobs := []job{}
	for rows.Next() {
		var j job
		if err = rows.Scan(&j.id, &j.endpoint, &j.raw, &j.title, &j.body, &j.url); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		var sub webpush.Subscription
		if err = json.Unmarshal(j.raw, &sub); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"title": j.title, "body": j.body, "url": j.url, "tag": fmt.Sprint(j.id)})
		resp, sendErr := webpush.SendNotificationWithContext(ctx, payload, &sub, &webpush.Options{Subscriber: "https://dorlab.net", VAPIDPrivateKey: m.private, VAPIDPublicKey: m.public, TTL: 86400, HTTPClient: &http.Client{Timeout: 10 * time.Second}})
		status := 0
		detail := ""
		if resp != nil {
			status = resp.StatusCode
			if status < 200 || status >= 300 {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
				detail = strings.TrimSpace(string(body))
			}
			resp.Body.Close()
		}
		if status == 404 || status == 410 {
			log.Printf("notifications: push rechazado con %d, suscripción eliminada endpoint=%s", status, shortEndpoint(j.endpoint))
			_, err = m.Pool.Exec(ctx, "DELETE FROM notification_subscriptions WHERE endpoint=$1", j.endpoint)
		} else if sendErr == nil && status >= 200 && status < 300 {
			_, err = m.Pool.Exec(ctx, "DELETE FROM notification_push WHERE notification_id=$1 AND endpoint=$2", j.id, j.endpoint)
		} else {
			log.Printf("notifications: fallo al enviar push notification=%d endpoint=%s status=%d err=%v body=%q", j.id, shortEndpoint(j.endpoint), status, sendErr, detail)
			_, err = m.Pool.Exec(ctx, "UPDATE notification_push SET attempts=attempts+1,next_at=now()+ interval '5 minutes' WHERE notification_id=$1 AND endpoint=$2", j.id, j.endpoint)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func shortEndpoint(e string) string {
	if len(e) > 60 {
		return e[:60] + "…"
	}
	return e
}
