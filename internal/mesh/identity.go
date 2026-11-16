// Package mesh connects the nodes of one user: it pairs them, keeps their
// data in sync over mutual TLS and forwards live calls (a thread of another
// machine) to the node that owns them.
package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// Identity is the key pair of a node. Peers recognise each other by the
// public key; the certificate only carries it into TLS.
type Identity struct {
	Key  ed25519.PrivateKey
	Cert tls.Certificate
}

// PublicKey encodes the public key as peers store it.
func (id Identity) PublicKey() string { return encodeKey(id.Key.Public().(ed25519.PublicKey)) }

func encodeKey(k ed25519.PublicKey) string { return base64.RawURLEncoding.EncodeToString(k) }

// Fingerprint is a short form of a public key, for people to compare.
func Fingerprint(publicKey string) string {
	sum := sha256.Sum256([]byte(publicKey))
	return fmt.Sprintf("%X", sum[:4])
}

// LoadIdentity reads the node key from dir, creating it on first use. The
// file is readable only by the owner.
func LoadIdentity(dir string) (Identity, error) {
	path := filepath.Join(dir, "node.key")
	raw, err := os.ReadFile(path)
	var key ed25519.PrivateKey
	switch {
	case errors.Is(err, os.ErrNotExist):
		_, key, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return Identity{}, err
		}
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return Identity{}, err
		}
		if err = os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			return Identity{}, err
		}
	case err != nil:
		return Identity{}, err
	default:
		block, _ := pem.Decode(raw)
		if block == nil {
			return Identity{}, errors.New("mesh: node.key is not PEM")
		}
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return Identity{}, err
		}
		var ok bool
		if key, ok = parsed.(ed25519.PrivateKey); !ok {
			return Identity{}, errors.New("mesh: node.key is not Ed25519")
		}
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "dory-node"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Key: key, Cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}}, nil
}

// peerKey extracts the public key a peer presented in TLS.
func peerKey(certs [][]byte) (string, error) {
	if len(certs) == 0 {
		return "", errors.New("mesh: peer sent no certificate")
	}
	cert, err := x509.ParseCertificate(certs[0])
	if err != nil {
		return "", err
	}
	k, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return "", errors.New("mesh: peer key is not Ed25519")
	}
	return encodeKey(k), nil
}

// clientTLS dials a peer and accepts it only if its key is want ("" accepts
// any key: used only while pairing, where the code authenticates both sides).
func (id Identity) clientTLS(want string, got *string) *tls.Config {
	return &tls.Config{
		Certificates:       []tls.Certificate{id.Cert},
		InsecureSkipVerify: true, // trust comes from the pinned key below, not from a CA
		MinVersion:         tls.VersionTLS13,
		VerifyPeerCertificate: func(certs [][]byte, _ [][]*x509.Certificate) error {
			k, err := peerKey(certs)
			if err != nil {
				return err
			}
			if got != nil {
				*got = k
			}
			if want != "" && k != want {
				return fmt.Errorf("mesh: peer key %s is not the paired one", Fingerprint(k))
			}
			return nil
		},
	}
}
