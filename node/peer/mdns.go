package peer

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

// MDNSService is the DNS-SD service type of Dory peers.
const MDNSService = "_dory._tcp"

// LocalIPs lists the non-loopback unicast IPs of the up interfaces. It returns
// nothing where the platform forbids enumeration (Android 11+): inject the
// addresses with MDNS.IPs instead.
func LocalIPs() []string {
	var out []string
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLinkLocalUnicast() {
				out = append(out, ipn.IP.String())
			}
		}
	}
	return out
}

// MDNS announces this device as _dory._tcp and browses for the others.
type MDNS struct {
	Identity *Identity
	Port     int
	// IPs are announced explicitly (zeroconf.RegisterProxy); empty means
	// LocalIPs(). Android must inject them.
	IPs []string
	// Ifaces restricts multicast to these interfaces; nil means all.
	Ifaces []net.Interface

	mu    sync.Mutex
	srv   *zeroconf.Server
	found map[string]mdnsEntry // device id -> last announcement
}

type mdnsEntry struct {
	addrs []string
	at    time.Time
}

// Announce publishes the device until Stop.
func (m *MDNS) Announce() error {
	ips := m.IPs
	if len(ips) == 0 {
		ips = LocalIPs()
	}
	if len(ips) == 0 {
		return fmt.Errorf("peer: no local IPs to announce")
	}
	id := m.Identity.DeviceID()
	srv, err := zeroconf.RegisterProxy("dory-"+id[:12], MDNSService, "local.", m.Port,
		"dory-"+id[:12], ips, []string{"device=" + id, "name=" + m.Identity.Name}, m.Ifaces)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.srv = srv
	m.mu.Unlock()
	return nil
}

// Stop withdraws the announcement.
func (m *MDNS) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.srv != nil {
		m.srv.Shutdown()
		m.srv = nil
	}
}

// Browse keeps the cache of discovered peers fresh until ctx ends. It blocks.
func (m *MDNS) Browse(ctx context.Context) error {
	var opts []zeroconf.ClientOption
	if m.Ifaces != nil {
		opts = append(opts, zeroconf.SelectIfaces(m.Ifaces))
	}
	res, err := zeroconf.NewResolver(opts...)
	if err != nil {
		return err
	}
	for ctx.Err() == nil {
		entries := make(chan *zeroconf.ServiceEntry, 16)
		bctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err = res.Browse(bctx, MDNSService, "local.", entries); err != nil {
			cancel()
			return err
		}
		for e := range entries {
			m.record(e)
		}
		cancel()
	}
	return ctx.Err()
}

func (m *MDNS) record(e *zeroconf.ServiceEntry) {
	var dev string
	for _, t := range e.Text {
		if strings.HasPrefix(t, "device=") {
			dev = t[7:]
		}
	}
	if dev == "" {
		return
	}
	var addrs []string
	for _, ip := range e.AddrIPv4 {
		addrs = append(addrs, net.JoinHostPort(ip.String(), strconv.Itoa(e.Port)))
	}
	for _, ip := range e.AddrIPv6 {
		if !ip.IsLinkLocalUnicast() {
			addrs = append(addrs, net.JoinHostPort(ip.String(), strconv.Itoa(e.Port)))
		}
	}
	if len(addrs) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.found == nil {
		m.found = map[string]mdnsEntry{}
	}
	m.found[dev] = mdnsEntry{addrs, time.Now()}
}

// Lookup returns the addresses last announced by deviceID.
func (m *MDNS) Lookup(deviceID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.found[deviceID].addrs...)
}
