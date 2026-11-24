package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
)

// Nodes find each other in three ways (requirement RF-04): mDNS, a UDP beacon on the
// local network and the peer list of Tailscale. Both only update addresses
// of paired nodes or list new nodes to pair; trust always comes from pairing.
var beaconAddr = &net.UDPAddr{IP: net.IPv4(239, 255, 77, 10), Port: 7711}

type beacon struct {
	App  string `json:"app"`
	Node string `json:"node"`
	Name string `json:"name"`
	Port int    `json:"port"`
}

func (m *Mesh) discover(ctx context.Context) {
	go m.announce(ctx)
	go m.hear(ctx)
	go m.mdns(ctx)
	for {
		m.tailscale(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}

func (m *Mesh) announce(ctx context.Context) {
	conn, err := net.DialUDP("udp4", nil, beaconAddr)
	if err != nil {
		log.Printf("mesh: beacon: %v", err)
		return
	}
	defer conn.Close()
	msg, _ := json.Marshal(beacon{App: "dory", Node: m.log.Node(), Name: m.name, Port: m.port})
	for {
		conn.Write(msg)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (m *Mesh) hear(ctx context.Context) {
	conn, err := net.ListenMulticastUDP("udp4", nil, beaconAddr)
	if err != nil {
		log.Printf("mesh: listen for beacons: %v", err)
		return
	}
	go func() { <-ctx.Done(); conn.Close() }()
	buf := make([]byte, 1024)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		var b beacon
		if json.Unmarshal(buf[:n], &b) != nil || b.App != "dory" || b.Node == m.log.Node() {
			continue
		}
		m.spotted(ctx, b.Node, b.Name, net.JoinHostPort(from.IP.String(), fmt.Sprint(b.Port)))
	}
}

// tailscale asks each online Tailscale peer whether it runs a Dory node.
func (m *Mesh) tailscale(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := cmdexec.CommandContext(ctx, "tailscale", "status", "--json").Output()
	if err != nil {
		return
	}
	var status struct {
		Peer map[string]struct {
			HostName     string   `json:"HostName"`
			TailscaleIPs []string `json:"TailscaleIPs"`
			Online       bool     `json:"Online"`
		} `json:"Peer"`
	}
	if json.Unmarshal(out, &status) != nil {
		return
	}
	c := m.probeClient("", nil)
	c.Timeout = 3 * time.Second
	defer c.CloseIdleConnections()
	var wg sync.WaitGroup
	for _, p := range status.Peer {
		if !p.Online || len(p.TailscaleIPs) == 0 {
			continue
		}
		address := net.JoinHostPort(p.TailscaleIPs[0], fmt.Sprint(DefaultPort))
		wg.Add(1)
		go func() {
			defer wg.Done()
			var hello struct {
				NodeID string `json:"node_id"`
				Name   string `json:"name"`
			}
			if getJSON(ctx, c, "https://"+address+peerPrefix+"/hello", &hello) == nil && hello.NodeID != "" {
				m.spotted(ctx, hello.NodeID, hello.Name, address)
			}
		}()
	}
	wg.Wait()
}

// spotted records where a node was seen: the address of a paired node is
// updated, an unknown one is offered for pairing.
func (m *Mesh) spotted(ctx context.Context, node, name, address string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var current string
	err := m.db.QueryRowContext(ctx, `SELECT address FROM peers WHERE node_id = ? AND revoked_at IS NULL`, node).Scan(&current)
	if err == nil {
		if current != address {
			if _, err := m.db.ExecContext(ctx, `UPDATE peers SET address = ? WHERE node_id = ?`, address, node); err != nil {
				log.Printf("mesh: address of %s: %v", node, err)
				return
			}
			m.invalidateTransport(node)
			if ch, ok := m.wake[node]; ok {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
		return
	}
	m.found[node] = Found{ID: node, Name: name, Address: address, Seen: time.Now()}
}

// Discovered lists unpaired nodes seen in the last few minutes.
func (m *Mesh) Discovered() []Found {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Found{}
	for id, f := range m.found {
		if time.Since(f.Seen) > 5*time.Minute {
			delete(m.found, id)
			continue
		}
		if _, paired := m.wake[id]; !paired {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}
