package runtime

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// Delegation is one piece of work a thread handed to a subagent, kept on the
// parent's node: what T3 Code calls a delegated task.
type Delegation struct {
	ID             string `json:"task_id"`
	ParentThreadID string `json:"parent_thread_id"`
	ChildThreadID  string `json:"child_thread_id"`
	ChildNodeID    string `json:"child_node_id"`
	Title          string `json:"title"`
	// Status is running, completed, failed, waiting, interrupted or cancelled.
	Status    string `json:"status"`
	Result    string `json:"result"`
	Delivered bool   `json:"delivered"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Outcome is how a subagent turn ended, sent to the parent's node.
type Outcome struct {
	ParentThreadID string `json:"parent_thread_id"`
	ChildThreadID  string `json:"child_thread_id"`
	ChildNodeID    string `json:"child_node_id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	Result         string `json:"result"`
	// DeliveryID is the id of the outbox delivery that carried the outcome;
	// the parent's node applies each id once, so a retry changes nothing.
	DeliveryID string `json:"delivery_id,omitempty"`
}

// DelegationComplete is the outbox kind of a subagent outcome sent to the
// node of its parent.
const DelegationComplete = "delegation.complete"

var stateLabel = map[string]string{"completed": "terminó", "failed": "falló", "waiting": "espera una respuesta", "interrupted": "se detuvo", "cancelled": "se canceló"}

// Terminal reports whether a delegation status no longer changes by itself.
func Terminal(status string) bool { return status != "running" }

const delegationCols = `id, parent_thread_id, child_thread_id, child_node_id, title, status, result, delivered, created_at, updated_at`

func scanDelegation(row interface{ Scan(...any) error }) (Delegation, error) {
	var d Delegation
	var delivered int
	err := row.Scan(&d.ID, &d.ParentThreadID, &d.ChildThreadID, &d.ChildNodeID, &d.Title, &d.Status, &d.Result, &delivered, &d.CreatedAt, &d.UpdatedAt)
	d.Delivered = delivered == 1
	return d, err
}

// RecordDelegation registers that parent delegated work to child; a running
// delegation of the pair is reused.
func (m *Manager) RecordDelegation(ctx context.Context, parent, child, node, title string) (Delegation, error) {
	if d, err := m.LatestDelegation(ctx, parent, child); err == nil && d.Status == "running" {
		return d, nil
	}
	at := now()
	d := Delegation{ID: "task-" + strings.ToLower(rand.Text()[:12]), ParentThreadID: parent, ChildThreadID: child, ChildNodeID: node, Title: title, Status: "running", CreatedAt: at, UpdatedAt: at}
	_, err := m.db.ExecContext(ctx, `INSERT INTO thread_delegations(`+delegationCols+`) VALUES (?, ?, ?, ?, ?, ?, '', 0, ?, ?)`,
		d.ID, d.ParentThreadID, d.ChildThreadID, d.ChildNodeID, d.Title, d.Status, d.CreatedAt, d.UpdatedAt)
	return d, err
}

// LatestDelegation is the newest delegation from parent to child.
func (m *Manager) LatestDelegation(ctx context.Context, parent, child string) (Delegation, error) {
	return scanDelegation(m.db.QueryRowContext(ctx, `SELECT `+delegationCols+` FROM thread_delegations
		WHERE parent_thread_id = ? AND child_thread_id = ? ORDER BY created_at DESC LIMIT 1`, parent, child))
}

// Delegation reads one delegation of this node; reading its outcome marks it
// delivered.
func (m *Manager) Delegation(ctx context.Context, id string, read bool) (Delegation, error) {
	d, err := scanDelegation(m.db.QueryRowContext(ctx, `SELECT `+delegationCols+` FROM thread_delegations WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	if err == nil && read && Terminal(d.Status) && !d.Delivered {
		_, err = m.db.ExecContext(ctx, `UPDATE thread_delegations SET delivered = 1 WHERE id = ?`, id)
	}
	return d, err
}

// Delegations lists what a thread delegated, newest first.
func (m *Manager) Delegations(ctx context.Context, parent string) ([]Delegation, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT `+delegationCols+` FROM thread_delegations WHERE parent_thread_id = ? ORDER BY created_at DESC`, parent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Delegation{}
	for rows.Next() {
		d, err := scanDelegation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CancelDelegation marks a running delegation cancelled; its child is stopped
// by the caller and its end does not wake the parent.
func (m *Manager) CancelDelegation(ctx context.Context, id string) (Delegation, error) {
	d, err := m.Delegation(ctx, id, false)
	if err != nil || Terminal(d.Status) {
		return d, err
	}
	d.Status, d.UpdatedAt = "cancelled", now()
	_, err = m.db.ExecContext(ctx, `UPDATE thread_delegations SET status = ?, updated_at = ? WHERE id = ?`, d.Status, d.UpdatedAt, d.ID)
	return d, err
}

// Complete records how a subagent turn ended on the parent's node and wakes
// the parent with the result. A child that runs again after its delegation
// ended opens a follow-up delegation, as T3 Code does.
//
// It is idempotent by DeliveryID: an outcome whose id was already applied is
// ignored. The id is kept in applied_deliveries, a small local table, rather
// than in thread_delegations, because a delivery may find no delegation row
// to hold it (a follow-up turn opens a new one) and cancelled or archived
// parents must not apply it later either.
func (m *Manager) Complete(ctx context.Context, o Outcome) (err error) {
	parent, err := m.local(ctx, o.ParentThreadID)
	if err != nil {
		return err
	}
	if o.DeliveryID != "" {
		first, err := m.claimDelivery(ctx, o.DeliveryID)
		if err != nil || !first {
			return err
		}
		// A failed attempt is retried by the sender, so it must not count.
		defer func() {
			if err != nil {
				m.db.ExecContext(context.WithoutCancel(ctx), `DELETE FROM applied_deliveries WHERE id = ?`, o.DeliveryID)
			}
		}()
	}
	if parent.ArchivedAt != nil {
		return nil
	}
	d, err := m.LatestDelegation(ctx, o.ParentThreadID, o.ChildThreadID)
	switch {
	case err == nil && d.Status == "cancelled":
		return nil
	case err == nil && d.Status == "running":
	case err == nil || errors.Is(err, sql.ErrNoRows):
		if d, err = m.RecordDelegation(ctx, o.ParentThreadID, o.ChildThreadID, o.ChildNodeID, o.Title); err != nil {
			return err
		}
	default:
		return err
	}
	if _, err = m.db.ExecContext(ctx, `UPDATE thread_delegations SET status = ?, result = ?, title = ?, delivered = 0, updated_at = ? WHERE id = ?`,
		o.Status, o.Result, o.Title, now(), d.ID); err != nil {
		return err
	}
	return m.SendFrom(ctx, o.ParentThreadID, o.ChildThreadID, wakeText(d.ID, o))
}

// claimDelivery records that a delivery is being applied; false means it
// already was. Ids older than a month are dropped on the way.
func (m *Manager) claimDelivery(ctx context.Context, id string) (bool, error) {
	at := time.Now().UTC()
	m.db.ExecContext(ctx, `DELETE FROM applied_deliveries WHERE applied_at < ?`, at.AddDate(0, -1, 0).Format(time.RFC3339Nano))
	res, err := m.db.ExecContext(ctx, `INSERT OR IGNORE INTO applied_deliveries(id, applied_at) VALUES (?, ?)`, id, at.Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// wakeText is the message that wakes the parent. Its first line is the header
// the interface recognises a subagent result by.
func wakeText(task string, o Outcome) string {
	text := fmt.Sprintf("[Subagente «%s» (%s) %s]", o.Title, o.ChildThreadID, stateLabel[o.Status])
	if o.Status == "waiting" {
		text += "\nTiene una pregunta pendiente: respóndela con thread_send o déjala al usuario."
	}
	if o.Result != "" {
		text += "\n\n" + o.Result
	}
	return text + fmt.Sprintf("\n\n(task_id %s: subagent_status para releerlo, subagent_cancel para pararlo)", task)
}

// deliver is the one way out of an outcome. On the parent's node it is
// applied at once; on another node it goes through the outbox, so it arrives
// when that node is connected again even if it is not now.
func (m *Manager) deliver(ctx context.Context, node string, o Outcome) {
	var err error
	switch {
	case node == m.node:
		err = m.Complete(ctx, o)
	case m.Outbox == nil:
		err = errors.New("no outbox to reach another node")
	default:
		_, err = m.Outbox.Enqueue(ctx, node, DelegationComplete, o)
	}
	if err != nil {
		log.Printf("runtime: subagent %s outcome: %v", o.ChildThreadID, err)
	}
}
