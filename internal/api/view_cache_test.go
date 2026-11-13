package api

import (
	"context"
	"testing"

	"github.com/migueel15/dory/internal/t3"
)

func TestViewCacheReusesUntilForgotten(t *testing.T) {
	c := &viewCache{entries: map[string]viewEntry{}, gen: map[string]uint64{}}
	m := t3.NewMonitor(nil)
	calls := 0
	fetch := func(context.Context) (threadRead, error) {
		calls++
		return threadRead{cursor: "c"}, nil
	}
	for i := 0; i < 3; i++ {
		if r, err := c.get(context.Background(), m, "th", fetch); err != nil || r.cursor != "c" {
			t.Fatalf("get: %+v %v", r, err)
		}
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	c.forget("th")
	if _, err := c.get(context.Background(), m, "th", fetch); err != nil || calls != 2 {
		t.Fatalf("after forget: calls = %d err = %v", calls, err)
	}
}
