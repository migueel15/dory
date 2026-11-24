package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/localdb"
)

type sent struct{ node, kind, id, payload string }

type fakeSender struct {
	err  error
	sent []sent
}

func (f *fakeSender) Deliver(_ context.Context, node, kind, id string, payload json.RawMessage) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, sent{node, kind, id, string(payload)})
	return nil
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func setup(t *testing.T) (*Outbox, *fakeSender, *clock) {
	t.Helper()
	db, err := localdb.Open(context.Background(), filepath.Join(t.TempDir(), "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &fakeSender{}
	o := New(db, f)
	c := &clock{time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	o.now = c.now
	return o, f, c
}

func statusOf(t *testing.T, o *Outbox, id string) Delivery {
	t.Helper()
	all, err := o.List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range all {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("delivery %s gone", id)
	return Delivery{}
}

func TestDeliversInCreationOrder(t *testing.T) {
	ctx := context.Background()
	o, f, c := setup(t)
	a, _ := o.Enqueue(ctx, "n1", "delegation.complete", map[string]string{"n": "1"})
	c.t = c.t.Add(time.Millisecond)
	b, _ := o.Enqueue(ctx, "n1", "delegation.complete", map[string]string{"n": "2"})
	if err := o.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 2 || f.sent[0].id != a || f.sent[1].id != b || f.sent[0].payload != `{"n":"1"}` || f.sent[0].node != "n1" {
		t.Fatalf("sent %+v", f.sent)
	}
	if d := statusOf(t, o, a); d.Status != "delivered" || d.Attempts != 1 {
		t.Fatalf("%+v", d)
	}
	f.sent = nil
	if err := o.Flush(ctx); err != nil || len(f.sent) != 0 {
		t.Fatalf("delivered twice: %v %+v", err, f.sent)
	}
}

func TestRetriesWithBackoffAndKickBringsItForward(t *testing.T) {
	ctx := context.Background()
	o, f, c := setup(t)
	f.err = errors.New("offline")
	id, _ := o.Enqueue(ctx, "n1", "k", 1)
	if err := o.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	d := statusOf(t, o, id)
	if d.Status != "pending" || d.Attempts != 1 || d.LastError != "offline" {
		t.Fatalf("%+v", d)
	}
	f.err = nil
	if err := o.Flush(ctx); err != nil || len(f.sent) != 0 {
		t.Fatalf("retried before its time: %v %+v", err, f.sent)
	}
	o.KickNode(ctx, "other")
	if o.Flush(ctx); len(f.sent) != 0 {
		t.Fatal("kick of another node brought it forward")
	}
	o.KickNode(ctx, "n1")
	if err := o.Flush(ctx); err != nil || len(f.sent) != 1 {
		t.Fatalf("kick did not bring it forward: %v %+v", err, f.sent)
	}
	if d := statusOf(t, o, id); d.Status != "delivered" || d.Attempts != 2 {
		t.Fatalf("%+v", d)
	}
	_ = c
}

func TestBackoffGrowsUpToTenMinutes(t *testing.T) {
	for n, want := range map[int]time.Duration{1: 5 * time.Second, 2: 10 * time.Second, 5: 80 * time.Second, 9: 10 * time.Minute, 50: 10 * time.Minute} {
		for range 20 {
			if got := Backoff(n); got < want*8/10 || got > want*12/10 {
				t.Fatalf("Backoff(%d) = %v, want about %v", n, got, want)
			}
		}
	}
}

func TestFailedDelayAdvancesWithTheClock(t *testing.T) {
	ctx := context.Background()
	o, f, c := setup(t)
	f.err = errors.New("offline")
	id, _ := o.Enqueue(ctx, "n1", "k", 1)
	o.Flush(ctx)
	f.err = nil
	c.t = c.t.Add(time.Minute)
	if err := o.Flush(ctx); err != nil || len(f.sent) != 1 {
		t.Fatalf("not retried after the backoff: %v %+v", err, f.sent)
	}
	if statusOf(t, o, id).Status != "delivered" {
		t.Fatal("not delivered")
	}
}

func TestTransientFailureKeepsTheOrderOfTheNode(t *testing.T) {
	ctx := context.Background()
	o, f, c := setup(t)
	f.err = errors.New("offline")
	o.Enqueue(ctx, "n1", "k", 1)
	c.t = c.t.Add(time.Millisecond)
	o.Enqueue(ctx, "n1", "k", 2)
	o.Flush(ctx)
	all, _ := o.List(ctx, "pending")
	if len(all) != 2 || all[0].Attempts != 1 || all[1].Attempts != 0 {
		t.Fatalf("second one was tried out of order: %+v", all)
	}
}

func TestPermanentErrorFailsAtOnceAndKeepsTheRow(t *testing.T) {
	ctx := context.Background()
	o, f, _ := setup(t)
	f.err = Permanent(errors.New("invalid"))
	id, _ := o.Enqueue(ctx, "n1", "k", 1)
	o.Flush(ctx)
	d := statusOf(t, o, id)
	if d.Status != "failed" || d.LastError != "invalid" || d.Attempts != 1 {
		t.Fatalf("%+v", d)
	}
}

func TestExpiresAfterSevenDaysAndPrunesDelivered(t *testing.T) {
	ctx := context.Background()
	o, f, c := setup(t)
	f.err = errors.New("offline")
	stuck, _ := o.Enqueue(ctx, "n1", "k", 1)
	o.Flush(ctx)
	f.err = nil
	c.t = c.t.Add(7*24*time.Hour + time.Second)
	f.err = errors.New("still offline")
	o.Flush(ctx)
	d := statusOf(t, o, stuck)
	if d.Status != "failed" || d.LastError != "expired after 7 days: offline" {
		t.Fatalf("%+v", d)
	}
	f.err = nil
	ok, _ := o.Enqueue(ctx, "n1", "k", 2)
	o.Flush(ctx)
	if statusOf(t, o, ok).Status != "delivered" {
		t.Fatal("not delivered")
	}
	c.t = c.t.Add(25 * time.Hour)
	o.Flush(ctx)
	all, _ := o.List(ctx, "")
	if len(all) != 1 || all[0].ID != stuck {
		t.Fatalf("delivered row not pruned or failed one lost: %+v", all)
	}
}

func TestEnqueueTxCommitsWithTheTransaction(t *testing.T) {
	ctx := context.Background()
	o, f, _ := setup(t)
	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = o.EnqueueTx(ctx, tx, "n1", "k", 1); err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
	o.Flush(ctx)
	if len(f.sent) != 0 {
		t.Fatalf("rolled back delivery was sent: %+v", f.sent)
	}
}
