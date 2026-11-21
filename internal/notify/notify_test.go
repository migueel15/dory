package notify

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

func open(t *testing.T) (*Notifier, *data.Store) {
	t.Helper()
	ctx := context.Background()
	db, err := localdb.Open(ctx, filepath.Join(t.TempDir(), "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	log, err := oplog.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	store := data.New(db, log)
	n, err := New(ctx, store, log.Node(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return n, store
}

func TestEmitIsIdempotentAndKeepsReadState(t *testing.T) {
	ctx := context.Background()
	n, store := open(t)
	n.Emit(ctx, "plan.ready", "thread-1", "stamp", "Tarea", "/threads/1")
	n.Emit(ctx, "bogus", "thread-1", "stamp", "", "")
	list, _ := store.Notices().List(ctx, data.Filter{})
	if len(list) != 1 || list[0].Title != "Plan listo para revisar" || list[0].ReadAt != nil {
		t.Fatalf("notices: %+v", list)
	}
	at := time.Now().UTC().Format(time.RFC3339Nano)
	list[0].ReadAt = &at
	store.Notices().Put(ctx, list[0])
	n.Emit(ctx, "plan.ready", "thread-1", "stamp", "Tarea", "/threads/1")
	list, _ = store.Notices().List(ctx, data.Filter{})
	if len(list) != 1 || list[0].ReadAt == nil {
		t.Fatalf("re-emit changed the notice: %+v", list)
	}
	n.Emit(ctx, "plan.ready", "thread-1", "other", "Tarea", "/threads/1")
	if list, _ = store.Notices().List(ctx, data.Filter{}); len(list) != 2 {
		t.Fatalf("a new event must add a notice: %d", len(list))
	}
}

func TestRetentionDeletesOldReadNotices(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n, store := open(t)
	n.Emit(ctx, "thread.idle", "a", "1", "", "")
	n.Emit(ctx, "thread.idle", "b", "1", "", "")
	list, _ := store.Notices().List(ctx, data.Filter{})
	old := time.Now().Add(-31 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	list[0].ReadAt = &old
	store.Notices().Put(ctx, list[0])
	done := make(chan struct{})
	go func() { n.Run(ctx); close(done) }()
	time.Sleep(300 * time.Millisecond)
	cancel()
	<-done
	list, err := store.Notices().List(context.Background(), data.Filter{})
	if err != nil || len(list) != 1 || list[0].ReadAt != nil {
		t.Fatalf("after retention: %+v %v", list, err)
	}
}

func TestDeliveryCombinesLocalAndPeers(t *testing.T) {
	ctx := context.Background()
	n, _ := open(t)
	if d := n.Delivery(ctx); d.Focus || d.Open || d.Push {
		t.Fatalf("nothing open: %+v", d)
	}
	n.Beat("a", Idle)
	if d := n.Delivery(ctx); d.Focus || d.Open {
		t.Fatalf("only an idle interface counts as away: %+v", d)
	}
	n.Beat("a", Open)
	n.Beat("b", Idle)
	if d := n.Delivery(ctx); d.Focus || !d.Open {
		t.Fatalf("open without focus beats idle: %+v", d)
	}
	n.Beat("b", Closed)
	n.PeerInfo = func(context.Context) []json.RawMessage {
		return []json.RawMessage{json.RawMessage(`{"presence":"focus","push":true}`)}
	}
	if d := n.Delivery(ctx); !d.Focus || !d.Push {
		t.Fatalf("a peer has the focus and push: %+v", d)
	}
	n.PeerInfo = nil
	n.Beat("a", Focus)
	if n.Presence() != Focus {
		t.Fatal("focus expected")
	}
	n.mu.Lock()
	n.beats["a"] = beat{Focus, time.Now().Add(-time.Minute)}
	n.mu.Unlock()
	if n.Presence() != Closed {
		t.Fatal("an expired heartbeat must close the interface")
	}
	n.Beat("a", Open)
	n.Beat("a", Closed)
	if n.Presence() != Closed {
		t.Fatal("closed beat must remove the interface")
	}
}

func TestSettingsMode(t *testing.T) {
	ctx := context.Background()
	n, store := open(t)
	if n.Mode(ctx, "thread.idle") != ModeAuto {
		t.Fatal("default must be automatic")
	}
	store.Settings().Put(ctx, data.Setting{ID: settingPrefix + "thread.idle", Value: ModeOff})
	store.Settings().Put(ctx, data.Setting{ID: settingPrefix + "plan.ready", Value: ModePush})
	n.Emit(ctx, "thread.idle", "t", "1", "", "")
	n.Emit(ctx, "plan.ready", "t", "1", "", "")
	list, _ := store.Notices().List(ctx, data.Filter{})
	if len(list) != 1 || list[0].Kind != "plan.ready" || n.Mode(ctx, "plan.ready") != ModePush {
		t.Fatalf("a silenced kind must not create notices: %+v", list)
	}
}
