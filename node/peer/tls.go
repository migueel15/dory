package peer

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// Membership tells whether a public key belongs to the group. *Log satisfies it.
type Membership interface {
	Allowed(pub ed25519.PublicKey) bool
}

// ErrNotMember is returned by the TLS verification for keys outside the group.
var ErrNotMember = errors.New("peer: certificate key is not a group member")

// Certificate builds a self-signed certificate over the device Ed25519 key.
// There is no CA: trust comes only from the membership log.
func (i *Identity) Certificate() (tls.Certificate, error) {
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: i.DeviceID()},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(20, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:              []string{"dory"},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, i.pub, i.priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: i.priv}, nil
}

// verifier returns a VerifyPeerCertificate that accepts only Ed25519 keys that
// are group members and, if want is not empty, whose device id is want.
func verifier(m Membership, want string) func([][]byte, [][]*x509.Certificate) error {
	return func(raw [][]byte, _ [][]*x509.Certificate) error {
		if len(raw) == 0 {
			return errors.New("peer: no certificate")
		}
		cert, err := x509.ParseCertificate(raw[0])
		if err != nil {
			return err
		}
		pub, ok := cert.PublicKey.(ed25519.PublicKey)
		if !ok {
			return errors.New("peer: certificate key is not Ed25519")
		}
		if err = cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); err != nil {
			return fmt.Errorf("peer: bad self-signature: %w", err)
		}
		if !m.Allowed(pub) {
			return ErrNotMember
		}
		if want != "" && DeviceID(pub) != want {
			return fmt.Errorf("peer: connected to device %s, expected %s", DeviceID(pub), want)
		}
		return nil
	}
}

// ServerTLS is the TLS 1.3 config of the listener: it demands a client
// certificate and accepts only members.
func (i *Identity) ServerTLS(m Membership) (*tls.Config, error) {
	c, err := i.Certificate()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:            tls.VersionTLS13,
		Certificates:          []tls.Certificate{c},
		ClientAuth:            tls.RequireAnyClientCert,
		VerifyPeerCertificate: verifier(m, ""),
	}, nil
}

// ClientTLS is the TLS 1.3 config to dial a member. If want is not empty the
// server must be that device. Name checks are replaced by key checks.
func (i *Identity) ClientTLS(m Membership, want string) (*tls.Config, error) {
	c, err := i.Certificate()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{c},
		// The chain is self-signed and nameless; VerifyPeerCertificate
		// replaces the standard verification with the membership check.
		InsecureSkipVerify:    true,
		VerifyPeerCertificate: verifier(m, want),
	}, nil
}

// peerKey extracts the device id of the verified client of a TLS connection.
func peerDevice(cs *tls.ConnectionState) string {
	if cs == nil || len(cs.PeerCertificates) == 0 {
		return ""
	}
	if pub, ok := cs.PeerCertificates[0].PublicKey.(ed25519.PublicKey); ok {
		return DeviceID(pub)
	}
	return ""
}
