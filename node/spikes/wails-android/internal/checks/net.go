// Package checks contains the six spike checks. It has no Wails or Android
// dependency so it can be unit-tested on the desktop.
package checks

import (
	"net"
	"sync"
)

// Iface is a network interface as reported by the Java side (Go cannot list
// interfaces on Android 11+, netlink is blocked) or by the desktop net package.
type Iface struct {
	Name  string   `json:"name"`
	Index int      `json:"index"`
	MTU   int      `json:"mtu"`
	IPs   []string `json:"ips"`
}

// Env holds what the checks need from the host platform.
type Env struct {
	DataDir string // app private files dir (activity.getFilesDir() on Android)

	// SetForeground starts/stops the Android foreground service. Nil on desktop.
	SetForeground func(on bool, title, text string)

	mu     sync.Mutex
	ifaces []Iface
}

// SetIfaces stores the interface list pushed by the page.
func (e *Env) SetIfaces(i []Iface) {
	e.mu.Lock()
	e.ifaces = i
	e.mu.Unlock()
}

// Ifaces returns the pushed list, or enumerates via the net package.
func (e *Env) Ifaces() []Iface {
	e.mu.Lock()
	pushed := e.ifaces
	e.mu.Unlock()
	if len(pushed) > 0 {
		return pushed
	}
	var out []Iface
	all, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, in := range all {
		if in.Flags&net.FlagUp == 0 || in.Flags&net.FlagLoopback != 0 || in.Flags&net.FlagMulticast == 0 {
			continue
		}
		addrs, _ := in.Addrs()
		f := Iface{Name: in.Name, Index: in.Index, MTU: in.MTU}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				f.IPs = append(f.IPs, ipn.IP.String())
			}
		}
		if len(f.IPs) > 0 {
			out = append(out, f)
		}
	}
	return out
}

// NetIfaces converts to net.Interface values usable by zeroconf.
func (e *Env) NetIfaces() []net.Interface {
	var out []net.Interface
	for _, f := range e.Ifaces() {
		out = append(out, net.Interface{Index: f.Index, MTU: f.MTU, Name: f.Name,
			Flags: net.FlagUp | net.FlagMulticast | net.FlagBroadcast})
	}
	return out
}

// IPs returns every IPv4 address of the usable interfaces.
func (e *Env) IPs() []net.IP {
	var out []net.IP
	for _, f := range e.Ifaces() {
		for _, s := range f.IPs {
			if ip := net.ParseIP(s); ip != nil {
				out = append(out, ip)
			}
		}
	}
	return out
}
