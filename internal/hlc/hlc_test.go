package hlc

import (
	"testing"
	"time"
)

func fixed(ms int64) func() time.Time { return func() time.Time { return time.UnixMilli(ms) } }

func TestNowIncreasesWithinMillisecond(t *testing.T) {
	c := New("a", Timestamp{})
	c.now = fixed(1000)
	first, second := c.Now(), c.Now()
	if !first.Less(second) || second.Counter != first.Counter+1 {
		t.Fatalf("got %v then %v", first, second)
	}
}

func TestNowSurvivesClockGoingBack(t *testing.T) {
	c := New("a", Timestamp{Wall: 5000, Counter: 3, Node: "a"})
	c.now = fixed(1000)
	if got := c.Now(); got.Wall != 5000 || got.Counter != 4 {
		t.Fatalf("got %v", got)
	}
}

func TestObserveMovesPastRemote(t *testing.T) {
	c := New("a", Timestamp{})
	c.now = fixed(1000)
	remote := Timestamp{Wall: 2000, Counter: 7, Node: "b"}
	c.Observe(remote)
	if got := c.Now(); !remote.Less(got) {
		t.Fatalf("local %v not after remote %v", got, remote)
	}
}

func TestStringOrderMatchesLess(t *testing.T) {
	a := Timestamp{Wall: 999, Counter: 12, Node: "z"}
	b := Timestamp{Wall: 1000, Counter: 1, Node: "a"}
	if !(a.String() < b.String()) || !a.Less(b) {
		t.Fatalf("%s should sort before %s", a, b)
	}
	parsed, err := Parse(b.String())
	if err != nil || parsed != b {
		t.Fatalf("parse %q: %v %v", b.String(), parsed, err)
	}
}
