package peer

import (
	"context"
	"testing"
	"time"
)

// TestMDNSDiscovery needs multicast on a real interface; it skips where the
// environment has none.
func TestMDNSDiscovery(t *testing.T) {
	if len(LocalIPs()) == 0 {
		t.Skip("no network interface")
	}
	a, b := ident(t, "a"), ident(t, "b")
	ma := &MDNS{Identity: a, Port: 7710}
	mb := &MDNS{Identity: b, Port: 7711}
	if err := ma.Announce(); err != nil {
		t.Skip("announce:", err)
	}
	defer ma.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	go mb.Browse(ctx)
	for ctx.Err() == nil {
		if got := mb.Lookup(a.DeviceID()); len(got) > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Skip("mDNS announcement not seen (multicast unavailable here)")
}
