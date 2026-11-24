package data

import (
	"context"
	"testing"
	"time"
)

func TestThreadUnread(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	at := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return at }
	original, err := s.Threads().Create(ctx, Thread{NodeID: s.log.Node(), Provider: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if original.Status != "idle" || original.Unread || original.StateStale {
		t.Fatalf("new thread = %+v", original)
	}
	if _, err := s.MarkThreadRead(ctx, original.ID); err != nil || opCount(t, s) != 1 {
		t.Fatalf("mark without activity: %v", err)
	}
	if err := s.SetThreadState(ctx, original.ID, "idle", ""); err != nil {
		t.Fatal(err)
	}
	if thread, err := s.Threads().Get(ctx, original.ID); err != nil || !thread.Unread || thread.UpdatedAt != original.UpdatedAt {
		t.Fatalf("returned thread = %+v: %v", thread, err)
	}
	thread, err := s.MarkThreadRead(ctx, original.ID)
	if err != nil || thread.Unread || thread.ReadAt == nil || opCount(t, s) != 2 {
		t.Fatalf("marked thread = %+v: %v", thread, err)
	}
	if _, err := s.MarkThreadRead(ctx, original.ID); err != nil || opCount(t, s) != 2 {
		t.Fatalf("repeated read: %v", err)
	}
	at = at.Add(time.Nanosecond)
	if err := s.SetThreadState(ctx, original.ID, "working", ""); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.Threads().Get(ctx, original.ID)
	if err != nil || !fresh.Unread || fresh.ReadAt == nil || *fresh.ReadAt != *thread.ReadAt || opCount(t, s) != 2 {
		t.Fatalf("thread with new activity = %+v: %v", fresh, err)
	}
}
