package checks

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultMTLSPort is the TCP port of the mutual-TLS listener.
const DefaultMTLSPort = 47890

// PKIDir returns the directory where the spike PKI is written.
func PKIDir(env *Env) string { return filepath.Join(env.DataDir, "pki") }

// MTLS is the mutual-TLS listener. Only clients presenting a certificate signed
// by the spike CA are accepted.
type MTLS struct {
	env  *Env
	dir  string
	port int

	mu       sync.Mutex
	srv      *http.Server
	ln       net.Listener
	caPool   *x509.CertPool
	clientTC tls.Certificate

	Conns atomic.Int64 // TCP connections accepted
	Reqs  atomic.Int64 // authenticated requests served
}

func NewMTLS(env *Env, port int) *MTLS {
	if port == 0 {
		port = DefaultMTLSPort
	}
	return &MTLS{env: env, dir: PKIDir(env), port: port}
}

func (m *MTLS) Port() int { return m.port }

func writePEM(path, typ string, der []byte, mode os.FileMode) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), mode)
}

func writeKey(path string, k *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		return err
	}
	return writePEM(path, "EC PRIVATE KEY", der, 0o600)
}

func serial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	return n
}

// ensurePKI creates (once) the CA and client certificate and (every start) a
// server certificate whose SANs cover the current device IPs.
func (m *MTLS) ensurePKI() error {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return err
	}
	caKeyPath, caPath := filepath.Join(m.dir, "ca.key"), filepath.Join(m.dir, "ca.pem")
	var caCert *x509.Certificate
	var caKey *ecdsa.PrivateKey
	if b, err := os.ReadFile(caPath); err == nil {
		kb, kerr := os.ReadFile(caKeyPath)
		blk, _ := pem.Decode(b)
		kblk, _ := pem.Decode(kb)
		if kerr == nil && blk != nil && kblk != nil {
			caCert, _ = x509.ParseCertificate(blk.Bytes)
			caKey, _ = x509.ParseECPrivateKey(kblk.Bytes)
		}
	}
	if caCert == nil || caKey == nil {
		var err error
		caKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		tpl := &x509.Certificate{
			SerialNumber: serial(), Subject: pkix.Name{CommonName: "dory-spike-ca"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0),
			IsCA: true, BasicConstraintsValid: true,
			KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		}
		der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &caKey.PublicKey, caKey)
		if err != nil {
			return err
		}
		if caCert, err = x509.ParseCertificate(der); err != nil {
			return err
		}
		if err = writePEM(caPath, "CERTIFICATE", der, 0o644); err != nil {
			return err
		}
		if err = writeKey(caKeyPath, caKey); err != nil {
			return err
		}
		_ = os.Remove(filepath.Join(m.dir, "client.pem")) // force a client cert from the new CA
	}
	m.caPool = x509.NewCertPool()
	m.caPool.AddCert(caCert)

	issue := func(name string, eku x509.ExtKeyUsage, cn string, ips []net.IP, dns []string) error {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		tpl := &x509.Certificate{
			SerialNumber: serial(), Subject: pkix.Name{CommonName: cn},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(5, 0, 0),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{eku},
			IPAddresses: ips, DNSNames: dns,
		}
		der, err := x509.CreateCertificate(rand.Reader, tpl, caCert, &k.PublicKey, caKey)
		if err != nil {
			return err
		}
		if err = writePEM(filepath.Join(m.dir, name+".pem"), "CERTIFICATE", der, 0o644); err != nil {
			return err
		}
		return writeKey(filepath.Join(m.dir, name+".key"), k)
	}
	ips := append([]net.IP{net.ParseIP("127.0.0.1")}, m.env.IPs()...)
	if err := issue("server", x509.ExtKeyUsageServerAuth, "dory-spike-server", ips,
		[]string{"dory-spike.local", "localhost"}); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(m.dir, "client.pem")); err != nil {
		if err := issue("client", x509.ExtKeyUsageClientAuth, "dory-spike-client", nil, nil); err != nil {
			return err
		}
	}
	var err error
	m.clientTC, err = tls.LoadX509KeyPair(filepath.Join(m.dir, "client.pem"), filepath.Join(m.dir, "client.key"))
	return err
}

// Start (re)starts the listener on 0.0.0.0:port.
func (m *MTLS) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.srv != nil {
		_ = m.srv.Close()
		m.srv = nil
	}
	if err := m.ensurePKI(); err != nil {
		return fmt.Errorf("pki: %w", err)
	}
	srvCert, err := tls.LoadX509KeyPair(filepath.Join(m.dir, "server.pem"), filepath.Join(m.dir, "server.key"))
	if err != nil {
		return err
	}
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{srvCert},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: m.caPool,
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", m.port))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		m.Reqs.Add(1)
		cn := ""
		if len(r.TLS.PeerCertificates) > 0 {
			cn = r.TLS.PeerCertificates[0].Subject.CommonName
		}
		fmt.Fprintf(w, "dory-spike mtls ok client=%s\n", cn)
	})
	m.srv = &http.Server{
		Handler:   mux,
		TLSConfig: cfg,
		ConnState: func(_ net.Conn, s http.ConnState) {
			if s == http.StateNew {
				m.Conns.Add(1)
			}
		},
	}
	m.ln = ln
	go func(s *http.Server) { _ = s.ServeTLS(ln, "", "") }(m.srv)
	return nil
}

func (m *MTLS) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.srv != nil {
		_ = m.srv.Close()
		m.srv = nil
	}
}

// client builds an HTTP client against 127.0.0.1 trusting the spike CA.
func (m *MTLS) client(withCert bool) *http.Client {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: m.caPool, ServerName: "localhost"}
	if withCert {
		cfg.Certificates = []tls.Certificate{m.clientTC}
	}
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: cfg, DisableKeepAlives: true}}
}

func (m *MTLS) get(c *http.Client) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d/ping", m.port), nil)
	// ServerName "localhost" is a SAN of the server cert.
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	return string(b), nil
}

// SelfTest checks the listener from inside the process: a client with a valid
// certificate is accepted, a client without one is rejected.
func (m *MTLS) SelfTest() (string, error) {
	m.mu.Lock()
	running := m.srv != nil
	m.mu.Unlock()
	if !running {
		if err := m.Start(); err != nil {
			return "", fmt.Errorf("start: %w", err)
		}
	}
	body, err := m.get(m.client(true))
	if err != nil {
		return "", fmt.Errorf("with client cert: %w", err)
	}
	if _, err = m.get(m.client(false)); err == nil {
		return "", errors.New("server accepted a client WITHOUT certificate")
	}
	ips := m.env.IPs()
	return fmt.Sprintf("listening on :%d (SAN ips %v); with cert: %q; without cert rejected (%v). Desktop: scripts/mtls-check.sh",
		m.port, ips, trim(body), err), nil
}

func trim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
