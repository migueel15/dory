package oplog

import (
	"errors"
	"maps"
	"testing"
	"time"
)

func TestCompactKeepsRowsAndOriginSequence(t *testing.T) {
	ctx := t.Context()
	db, l := node(t)
	_, peer := node(t)
	if _, err := db.Exec(`INSERT INTO peers(node_id, public_key, paired_at) VALUES (?, ?, '')`, peer.Node(), peer.Node()); err != nil {
		t.Fatal(err)
	}
	write(t, l, "project", "p1", map[string]any{"slug": "demo", "name": "Uno", "created_at": "", "updated_at": ""})
	write(t, l, "project", "p2", map[string]any{"slug": "demo", "name": "Dos", "created_at": "", "updated_at": ""})
	write(t, l, "memory", "m1", map[string]any{"scope": "global", "content": "original", "created_at": "", "updated_at": ""})
	sync(t, l, peer)
	write(t, peer, "memory", "m1", map[string]any{"content": "texto del par"})
	write(t, l, "memory", "m1", map[string]any{"content": "texto local"})
	sync(t, peer, l)

	// No recent acknowledgement covers the latest local edit.
	if err := l.RecordPeerVector(ctx, peer.Node(), Vector{l.Node(): 1}); err != nil {
		t.Fatal(err)
	}
	if err := l.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ops`).Scan(&count); err != nil || count != 4 {
		t.Fatalf("partially compacted log = %d, %v", count, err)
	}

	vector, err := l.Vector(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tail, err := l.Since(ctx, Vector{l.Node(): 1}, BatchSize)
	if err != nil {
		t.Fatal(err)
	}
	beforeText, beforeSlugs := memory(t, db), slugs(t, db)
	if err := l.RecordPeerVector(ctx, peer.Node(), vector); err != nil {
		t.Fatal(err)
	}
	if err := l.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM ops`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("compacted log = %d, %v", count, err)
	}
	if v, err := l.Vector(ctx); err != nil || !maps.Equal(v, vector) {
		t.Fatalf("vector = %v, %v, want %v", v, err, vector)
	}
	if _, err := l.Since(ctx, Vector{}, BatchSize); !errors.Is(err, ErrSnapshotRequired) {
		t.Fatalf("empty vector: %v", err)
	}
	if err := l.Apply(ctx, tail); err != nil {
		t.Fatal(err)
	}
	if beforeText != memory(t, db) || !maps.Equal(beforeSlugs, slugs(t, db)) {
		t.Fatal("compaction or replay changed materialized rows")
	}
	// A write with no memory fields rematerializes the text from the snapshot.
	write(t, l, "memory", "m1", map[string]any{"title": "renombrada"})
	if got := memory(t, db); got != beforeText {
		t.Fatalf("rematerialized = %q, want %q", got, beforeText)
	}
	op, err := l.Write(ctx, "memory", "m1", map[string]any{"content": "resuelto"})
	if err != nil || op.Seq != vector[l.Node()]+2 || memory(t, db) != "resuelto" {
		t.Fatalf("edit after checkpoint = %+v, %v", op, err)
	}
}

func TestCompactKeepsUnknownOperationAndItsTail(t *testing.T) {
	ctx := t.Context()
	db, l := node(t)
	_, peer := node(t)
	if _, err := db.Exec(`INSERT INTO peers(node_id, public_key, paired_at) VALUES (?, ?, '')`, peer.Node(), peer.Node()); err != nil {
		t.Fatal(err)
	}
	write(t, peer, "task", "t1", map[string]any{"title": "antes", "created_at": "", "updated_at": ""})
	if _, err := peer.db.Exec(`INSERT INTO ops (node_id, seq, hlc, entity, entity_id, fields)
		VALUES (?, 2, ?, 'future', 'id', '{}')`, peer.Node(), peer.clock.Now().String()); err != nil {
		t.Fatal(err)
	}
	write(t, peer, "task", "t1", map[string]any{"title": "después"})
	sync(t, peer, l)
	if err := l.RecordPeerVector(ctx, peer.Node(), Vector{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE peer_vectors SET synced_at = ?`, time.Now().Add(-31*24*time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err := l.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := l.SnapshotPage(ctx, 0)
	if err != nil || page.Horizon[peer.Node()] != 1 || len(page.Rows) != 1 {
		t.Fatalf("snapshot = %+v, %v", page, err)
	}
	ops, err := l.Since(ctx, page.Horizon, BatchSize)
	if err != nil || len(ops) != 2 || ops[0].Entity != "future" || ops[1].Seq != 3 {
		t.Fatalf("retained tail = %+v, %v", ops, err)
	}
}
