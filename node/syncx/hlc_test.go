package syncx

import (
	"testing"
	"time"
)

func TestHLCOrderAndParse(t *testing.T) {
	a := HLC{Wall: 10, Counter: 2, Device: "b"}
	for _, o := range []HLC{
		{Wall: 10, Counter: 2, Device: "c"},
		{Wall: 10, Counter: 3, Device: "a"},
		{Wall: 11, Counter: 0, Device: "a"},
		{Wall: 0xfffffffff, Counter: 0, Device: "a"},
	} {
		if a.Compare(o) != -1 || o.Compare(a) != 1 {
			t.Fatalf("%v should be below %v", a, o)
		}
	}
	got, err := ParseHLC(a.String())
	if err != nil || got != a {
		t.Fatalf("parse = %v, %v", got, err)
	}
	if _, err = ParseHLC("nope"); err == nil {
		t.Fatal("invalid hlc parsed")
	}
}

func TestClockMonotonicAndObserve(t *testing.T) {
	phys := time.UnixMilli(1000)
	c := NewClock("a", func() time.Time { return phys })
	h1, h2 := c.Now(), c.Now()
	if h1.Compare(h2) != -1 || h1.Wall != 1000 || h2.Counter != 1 {
		t.Fatalf("same millisecond: %v %v", h1, h2)
	}
	phys = time.UnixMilli(900) // the wall clock went backwards
	if h3 := c.Now(); h3.Compare(h2) != 1 {
		t.Fatalf("clock went back: %v after %v", h3, h2)
	}
	remote := HLC{Wall: 5000, Counter: 7, Device: "z"}
	got := c.Observe(remote)
	if got.Compare(remote) != 1 || got.Wall != 5000 || got.Counter != 8 {
		t.Fatalf("observe = %v", got)
	}
	if next := c.Now(); next.Compare(got) != 1 {
		t.Fatalf("now after observe = %v", next)
	}
	phys = time.UnixMilli(9000)
	if h := c.Observe(remote); h.Wall != 9000 || h.Counter != 0 {
		t.Fatalf("physical time wins: %v", h)
	}
}

func TestClockCounterOverflow(t *testing.T) {
	c := NewClock("a", func() time.Time { return time.UnixMilli(1) })
	c.Seed(HLC{Wall: 1, Counter: maxCounter})
	h := c.Now()
	if h.Wall != 2 || h.Counter != 0 {
		t.Fatalf("overflow = %v", h)
	}
}
