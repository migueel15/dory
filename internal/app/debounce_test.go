package app

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestDebounceFoldsCallsIntoOneRun(t *testing.T) {
	var d debounce
	var runs atomic.Int32
	done := make(chan struct{}, 2)
	for range 5 {
		d.call(20*time.Millisecond, func() { runs.Add(1); done <- struct{}{} })
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("never ran")
	}
	time.Sleep(60 * time.Millisecond)
	if n := runs.Load(); n != 1 {
		t.Fatalf("runs = %d, want 1", n)
	}
}
