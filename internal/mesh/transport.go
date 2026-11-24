package mesh

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

type peerTransport struct {
	address, key string
	transport    *http.Transport
}

func (m *Mesh) peer(ctx context.Context, node string) (peerTransport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var p peerTransport
	err := m.db.QueryRowContext(ctx, `SELECT address, public_key FROM peers WHERE node_id = ? AND revoked_at IS NULL AND disabled_at IS NULL`, node).Scan(&p.address, &p.key)
	if err != nil || p.address == "" {
		return p, err
	}
	if cached, ok := m.transports[node]; ok && cached.address == p.address && cached.key == p.key {
		return cached, nil
	}
	m.invalidateTransport(node)
	p.transport = m.newTransport(p.key, nil)
	m.transports[node] = p
	return p, nil
}

// invalidateTransport closes idle connections and removes the cached transport.
// The caller holds m.mu.
func (m *Mesh) invalidateTransport(node string) {
	if p, ok := m.transports[node]; ok {
		p.transport.CloseIdleConnections()
		delete(m.transports, node)
	}
}

func (m *Mesh) newTransport(key string, got *string) *http.Transport {
	// Offer the peer ALPN even for WebSocket requests, so the mesh port
	// presents its node certificate and negotiates HTTP/1.1.
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 15 * time.Second}, Config: m.id.clientTLS(key, got)}
	return &http.Transport{
		DialTLSContext:        dialer.DialContext,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: 15 * time.Second,
	}
}

func (p peerTransport) client() *http.Client {
	return client(p.transport)
}

// The pairing handshake and discovery probe addresses before we know their node ID.
func (m *Mesh) probeClient(key string, got *string) *http.Client {
	return client(m.newTransport(key, got))
}

func client(transport *http.Transport) *http.Client {
	return &http.Client{
		Transport:     transport,
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
