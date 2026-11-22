package mesh

import (
	"context"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

// mdnsService is the DNS-SD type under which Dory nodes announce themselves.
const mdnsService = "_dory._tcp"

// mdns announces this node over multicast DNS and lists the others. Like the
// beacon, it only reports where a node is; trust still comes from pairing. The
// announcement carries the node id, its name and the mesh port, nothing else.
func (m *Mesh) mdns(ctx context.Context) {
	id := m.log.Node()
	server, err := zeroconf.Register("dory-"+id, mdnsService, "local.", m.port, []string{"id=" + id, "name=" + m.name}, nil)
	if err != nil {
		log.Printf("mesh: mdns announce: %v", err)
	} else {
		defer server.Shutdown()
	}
	for ctx.Err() == nil {
		m.browse(ctx, id)
		select {
		case <-ctx.Done():
		case <-time.After(30 * time.Second):
		}
	}
}

// browse listens for one round of announcements.
func (m *Mesh) browse(ctx context.Context, self string) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Printf("mesh: mdns browse: %v", err)
		return
	}
	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		for e := range entries {
			node, name := txt(e.Text, "id"), txt(e.Text, "name")
			if node == "" || node == self || e.Port == 0 {
				continue
			}
			ips := append(append([]net.IP{}, e.AddrIPv4...), e.AddrIPv6...)
			if len(ips) == 0 {
				continue
			}
			m.spotted(ctx, node, name, net.JoinHostPort(ips[0].String(), strconv.Itoa(e.Port)))
		}
	}()
	round, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := resolver.Browse(round, mdnsService, "local.", entries); err != nil {
		log.Printf("mesh: mdns browse: %v", err)
		return
	}
	<-round.Done()
}

func txt(records []string, key string) string {
	for _, r := range records {
		if v, ok := strings.CutPrefix(r, key+"="); ok {
			return v
		}
	}
	return ""
}
