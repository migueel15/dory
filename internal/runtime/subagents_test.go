package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/outbox"
)

func TestSpawnRunsChildAndWakesParent(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	claude := capturedProvider{"claude", make(chan Turn, 2), make(chan chan Event, 2)}
	codex := capturedProvider{"codex", make(chan Turn, 1), make(chan chan Event, 1)}
	m.registry.Register(claude)
	m.registry.Register(codex)
	parent, err := m.Create(ctx, NewThread{Provider: claude.id, Model: "opus", RuntimeMode: "approval-required"})
	if err != nil {
		t.Fatal(err)
	}

	child, err := m.Spawn(ctx, parent.ID, Subagent{Prompt: "Revisa el login\ncon detalle", Provider: "codex", Model: "gpt-6.2-sol"})
	if err != nil {
		t.Fatal(err)
	}
	turn := <-codex.turns
	if child.ParentID == nil || *child.ParentID != parent.ID || child.Title != "Revisa el login" || child.RuntimeMode != "approval-required" || turn.Model != "gpt-6.2-sol" {
		t.Fatalf("child %+v turn %+v", child, turn)
	}
	if parentDir, _ := m.Dir(ctx, parent.ID); turn.Dir != parentDir {
		t.Fatalf("child dir %q, parent %q", turn.Dir, parentDir)
	}
	if _, err = m.Spawn(ctx, child.ID, Subagent{Prompt: "nieto"}); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("grandchild accepted: %v", err)
	}

	events := <-codex.ends
	events <- Event{Message: &Message{ID: turn.RunID + ":a", Role: "assistant", Text: "Login correcto \uE200cite\uE202turn0search0\uE201"}}
	events <- Event{Done: true}
	close(events)

	wake := <-claude.turns
	if strings.ContainsRune(wake.Text, '\uE200') {
		t.Fatalf("harness markers reached the parent: %q", wake.Text)
	}
	if !strings.Contains(wake.Text, "Revisa el login") || !strings.Contains(wake.Text, "terminó") || !strings.Contains(wake.Text, "Login correcto") {
		t.Fatalf("parent woke with %q", wake.Text)
	}
	close(<-claude.ends)
	wait(t, store, parent.ID, "idle")
	msgs, _ := m.Messages(ctx, parent.ID)
	if len(msgs) == 0 || msgs[0].Role != "user" || msgs[0].SenderThreadID != child.ID {
		t.Fatalf("wake message not attributed to the child: %+v", msgs)
	}
}

func TestSpawnLimitsActiveChildren(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	parent, _ := m.Create(ctx, NewThread{Provider: "replay", Model: "grabacion"})
	for range MaxActiveChildren {
		child, err := m.Create(ctx, NewThread{Provider: "replay", ParentID: &parent.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err = store.SetThreadState(ctx, child.ID, "working", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Spawn(ctx, parent.ID, Subagent{Prompt: "uno más"}); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("limit not enforced: %v", err)
	}
}

func TestChildOnAnotherNodeNotifiesItsParentThroughThePeer(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	codex := capturedProvider{"codex", make(chan Turn, 1), make(chan chan Event, 1)}
	m.registry.Register(codex)
	parent, err := store.Threads().Create(ctx, data.Thread{NodeID: "other-node", Provider: "claude", Title: "Padre"})
	if err != nil {
		t.Fatal(err)
	}
	sender := &outboxSender{offline: true}
	m.Outbox = outbox.New(m.db, sender)
	child, err := m.Create(ctx, NewThread{Provider: codex.id, ParentID: &parent.ID, Title: "Prueba el binario"})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Send(ctx, child.ID, "prueba"); err != nil {
		t.Fatal(err)
	}
	turn := <-codex.turns
	events := <-codex.ends
	events <- Event{Message: &Message{ID: turn.RunID + ":a", Role: "assistant", Text: "Funciona"}}
	events <- Event{Done: true}
	close(events)
	// The parent's node is not connected: the outcome waits in the outbox.
	var queued []outbox.Delivery
	for range 200 {
		if queued, err = m.Outbox.List(ctx, "pending"); err == nil && len(queued) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(queued) != 1 || queued[0].Node != "other-node" || queued[0].Kind != DelegationComplete {
		t.Fatalf("queued %+v (%v)", queued, err)
	}
	var got Outcome
	if err = json.Unmarshal(queued[0].Payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.ParentThreadID != parent.ID || got.ChildThreadID != child.ID || got.Status != "completed" || got.Result != "Funciona" {
		t.Fatalf("peer outcome %+v", got)
	}
	// Once the node is back it arrives, and only once.
	sender.offline = false
	m.Outbox.KickNode(ctx, "other-node")
	if err = m.Outbox.Flush(ctx); err != nil || len(sender.delivered) != 1 || sender.delivered[0] != queued[0].ID {
		t.Fatalf("delivered %v (%v)", sender.delivered, err)
	}
	if err = m.Outbox.Flush(ctx); err != nil || len(sender.delivered) != 1 {
		t.Fatalf("delivered twice: %v (%v)", sender.delivered, err)
	}
}

type outboxSender struct {
	offline   bool
	delivered []string
}

func (s *outboxSender) Deliver(_ context.Context, _, _, id string, _ json.RawMessage) error {
	if s.offline {
		return errors.New("offline")
	}
	s.delivered = append(s.delivered, id)
	return nil
}

func TestCompleteIsIdempotentByDeliveryID(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	claude := capturedProvider{"claude", make(chan Turn, 4), make(chan chan Event, 4)}
	m.providers[claude.id] = claude
	parent, err := m.Create(ctx, NewThread{Provider: claude.id, Model: "opus"})
	if err != nil {
		t.Fatal(err)
	}
	o := Outcome{ParentThreadID: parent.ID, ChildThreadID: "child", ChildNodeID: "other", Title: "Tarea", Status: "completed", Result: "hecho", DeliveryID: "dlv-1"}
	if err = m.Complete(ctx, o); err != nil {
		t.Fatal(err)
	}
	if err = m.Complete(ctx, o); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = m.db.QueryRow(`SELECT count(*) FROM thread_delegations WHERE parent_thread_id = ?`, parent.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("delegations %d (%v)", count, err)
	}
	msgs, err := m.Messages(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	woken := 0
	for _, msg := range msgs {
		if msg.Role == "user" && strings.Contains(msg.Text, "hecho") {
			woken++
		}
	}
	if woken != 1 {
		t.Fatalf("parent woken %d times: %+v", woken, msgs)
	}
	// A failed attempt does not consume the id.
	bad := Outcome{ParentThreadID: "missing", Status: "completed", DeliveryID: "dlv-2"}
	if err = m.Complete(ctx, bad); err == nil {
		t.Fatal("unknown parent accepted")
	}
	var applied int
	m.db.QueryRow(`SELECT count(*) FROM applied_deliveries WHERE id = 'dlv-2'`).Scan(&applied)
	if applied != 0 {
		t.Fatal("failed delivery was recorded as applied")
	}
	_ = store
}

func TestDelegationRecordsOutcomeAndFollowUps(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	claude := capturedProvider{"claude", make(chan Turn, 2), make(chan chan Event, 2)}
	m.registry.Register(claude)
	parent, _ := m.Create(ctx, NewThread{Provider: claude.id})
	child, _ := m.Create(ctx, NewThread{Provider: "replay", ParentID: &parent.ID, Title: "Hijo"})
	d, err := m.RecordDelegation(ctx, parent.ID, child.ID, m.node, "Hijo")
	if err != nil || d.Status != "running" {
		t.Fatalf("record %+v %v", d, err)
	}
	if again, _ := m.RecordDelegation(ctx, parent.ID, child.ID, m.node, "Hijo"); again.ID != d.ID {
		t.Fatal("a running delegation was not reused")
	}
	if err = m.Complete(ctx, Outcome{ParentThreadID: parent.ID, ChildThreadID: child.ID, ChildNodeID: m.node, Title: "Hijo", Status: "completed", Result: "hecho"}); err != nil {
		t.Fatal(err)
	}
	wake := <-claude.turns
	if !strings.Contains(wake.Text, d.ID) || !strings.Contains(wake.Text, "hecho") {
		t.Fatalf("wake %q", wake.Text)
	}
	close(<-claude.ends)
	wait(t, store, parent.ID, "idle")
	got, _ := m.Delegation(ctx, d.ID, true)
	if got.Status != "completed" || got.Result != "hecho" || got.Delivered {
		t.Fatalf("delegation %+v", got)
	}
	if got, _ = m.Delegation(ctx, d.ID, false); !got.Delivered {
		t.Fatal("reading the result did not mark it delivered")
	}
	// The child ran again: a follow-up delegation, and a cancelled one stays quiet.
	if err = m.Complete(ctx, Outcome{ParentThreadID: parent.ID, ChildThreadID: child.ID, Title: "Hijo", Status: "failed"}); err != nil {
		t.Fatal(err)
	}
	<-claude.turns
	close(<-claude.ends)
	wait(t, store, parent.ID, "idle")
	list, _ := m.Delegations(ctx, parent.ID)
	if len(list) != 2 || list[0].Status != "failed" {
		t.Fatalf("follow-up %+v", list)
	}
	running, _ := m.RecordDelegation(ctx, parent.ID, child.ID, m.node, "Hijo")
	if c, err := m.CancelDelegation(ctx, running.ID); err != nil || c.Status != "cancelled" {
		t.Fatalf("cancel %+v %v", c, err)
	}
	if err = m.Complete(ctx, Outcome{ParentThreadID: parent.ID, ChildThreadID: child.ID, Title: "Hijo", Status: "interrupted"}); err != nil {
		t.Fatal(err)
	}
	select {
	case turn := <-claude.turns:
		t.Fatalf("a cancelled task woke the parent: %q", turn.Text)
	default:
	}
}
