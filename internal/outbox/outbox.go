// Package outbox keeps the deliveries to other nodes that must not be lost
// when the target is not connected. A delivery is saved in the local database
// and retried with backoff until the target accepts it, so a result computed
// while a node is offline arrives once it is back. The receiver deduplicates
// by the delivery id, which makes retries safe.
package outbox

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math/rand/v2"
	"strings"
	"sync"
	"time"
)

const (
	pollEvery  = 30 * time.Second
	minBackoff = 5 * time.Second
	maxBackoff = 10 * time.Minute
	maxAge     = 7 * 24 * time.Hour
	keepDone   = 24 * time.Hour
	stamp      = "2006-01-02T15:04:05.000000000Z" // fixed width, so text order is time order
)

// Sender hands a delivery to another node. id is the delivery id the receiver
// uses as its idempotency key. An error wrapped with Permanent is never retried.
type Sender interface {
	Deliver(ctx context.Context, node, kind, id string, payload json.RawMessage) error
}

type permanent struct{ err error }

func (p permanent) Error() string { return p.err.Error() }
func (p permanent) Unwrap() error { return p.err }

// Permanent marks an error that retrying cannot fix (an invalid payload, a
// refusal): the delivery fails at once and is kept for diagnosis.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanent{err}
}

// Delivery is one row of the outbox.
type Delivery struct {
	ID        string
	Node      string
	Kind      string
	Payload   json.RawMessage
	Status    string
	Attempts  int
	NextAt    string
	LastError string
	CreatedAt string
}

// Outbox persists and retries deliveries to other nodes.
type Outbox struct {
	db   *sql.DB
	send Sender
	now  func() time.Time
	wake chan struct{}
	mu   sync.Mutex // one pass at a time
}

func New(db *sql.DB, send Sender) *Outbox {
	return &Outbox{db: db, send: send, now: time.Now, wake: make(chan struct{}, 1)}
}

func (o *Outbox) at() string { return o.now().UTC().Format(stamp) }

// Execer is what Enqueue needs from a database or a transaction.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Enqueue saves a delivery for node and wakes the sender. It returns the
// delivery id.
func (o *Outbox) Enqueue(ctx context.Context, node, kind string, payload any) (string, error) {
	id, err := o.EnqueueTx(ctx, o.db, node, kind, payload)
	if err == nil {
		o.Kick()
	}
	return id, err
}

// EnqueueTx saves the delivery in tx, so it commits together with the change
// that caused it. Call Kick after the commit to send it at once; otherwise
// the next pass does.
func (o *Outbox) EnqueueTx(ctx context.Context, tx Execer, node, kind string, payload any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	id := "dlv-" + strings.ToLower(crand.Text()[:16])
	at := o.at()
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox(id, node_id, kind, payload, next_attempt_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, node, kind, string(raw), at, at, at)
	return id, err
}

// Kick asks the sender to look for due deliveries now.
func (o *Outbox) Kick() {
	select {
	case o.wake <- struct{}{}:
	default:
	}
}

// KickNode brings forward the retry of every pending delivery to node, for
// when the node has just become reachable.
func (o *Outbox) KickNode(ctx context.Context, node string) {
	at := o.at()
	if _, err := o.db.ExecContext(ctx, `UPDATE outbox SET next_attempt_at = ? WHERE node_id = ? AND status = 'pending' AND next_attempt_at > ?`, at, node, at); err != nil {
		log.Printf("outbox: kick %s: %v", node, err)
		return
	}
	o.Kick()
}

// Run delivers until ctx ends: every 30 seconds and whenever it is kicked.
func (o *Outbox) Run(ctx context.Context) {
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		if err := o.Flush(ctx); err != nil && ctx.Err() == nil {
			log.Printf("outbox: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-o.wake:
		}
	}
}

// Flush runs one pass: it expires old deliveries, sends the due ones in the
// order they were created, and drops what was delivered long ago. After a
// transient failure the rest of that node waits for the next pass, so order
// is kept and an unreachable node is not hammered.
func (o *Outbox) Flush(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := o.now()
	if _, err := o.db.ExecContext(ctx, `UPDATE outbox SET status = 'failed', updated_at = ?, last_error = 'expired after 7 days' || CASE WHEN last_error = '' THEN '' ELSE ': ' || last_error END
		WHERE status = 'pending' AND created_at <= ?`, o.at(), now.Add(-maxAge).UTC().Format(stamp)); err != nil {
		return err
	}
	if _, err := o.db.ExecContext(ctx, `DELETE FROM outbox WHERE status = 'delivered' AND updated_at <= ?`, now.Add(-keepDone).UTC().Format(stamp)); err != nil {
		return err
	}
	due, err := o.due(ctx)
	if err != nil {
		return err
	}
	blocked := map[string]bool{}
	for _, d := range due {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if blocked[d.Node] {
			continue
		}
		err := o.send.Deliver(ctx, d.Node, d.Kind, d.ID, d.Payload)
		var perm permanent
		switch {
		case err == nil:
			err = o.finish(ctx, d.ID, "delivered", d.Attempts+1, "", "")
		case errors.As(err, &perm):
			log.Printf("outbox: %s to %s failed for good: %v", d.Kind, d.Node, err)
			err = o.finish(ctx, d.ID, "failed", d.Attempts+1, "", err.Error())
		default:
			blocked[d.Node] = true
			next := o.now().Add(Backoff(d.Attempts + 1)).UTC().Format(stamp)
			err = o.finish(ctx, d.ID, "pending", d.Attempts+1, next, err.Error())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *Outbox) due(ctx context.Context) ([]Delivery, error) {
	return o.query(ctx, `WHERE status = 'pending' AND next_attempt_at <= ?`, o.at())
}

func (o *Outbox) query(ctx context.Context, where string, args ...any) ([]Delivery, error) {
	rows, err := o.db.QueryContext(ctx, `SELECT id, node_id, kind, payload, status, attempts, next_attempt_at, last_error, created_at FROM outbox `+where+` ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Delivery
	for rows.Next() {
		var d Delivery
		var payload string
		if err := rows.Scan(&d.ID, &d.Node, &d.Kind, &payload, &d.Status, &d.Attempts, &d.NextAt, &d.LastError, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Payload = json.RawMessage(payload)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (o *Outbox) finish(ctx context.Context, id, status string, attempts int, next, lastErr string) error {
	if next == "" {
		next = o.at()
	}
	_, err := o.db.ExecContext(ctx, `UPDATE outbox SET status = ?, attempts = ?, next_attempt_at = ?, last_error = ?, updated_at = ? WHERE id = ?`,
		status, attempts, next, lastErr, o.at(), id)
	return err
}

// Backoff is the wait before attempt n+1 after n failures: 5 seconds doubling
// up to 10 minutes, with up to 20% of jitter either way.
func Backoff(failures int) time.Duration {
	d := maxBackoff
	if failures < 8 {
		d = min(minBackoff<<max(failures-1, 0), maxBackoff)
	}
	return time.Duration(float64(d) * (0.8 + 0.4*rand.Float64()))
}

// List returns the deliveries in a status for diagnosis ("" for all).
func (o *Outbox) List(ctx context.Context, status string) ([]Delivery, error) {
	if status == "" {
		return o.query(ctx, ``)
	}
	return o.query(ctx, `WHERE status = ?`, status)
}
