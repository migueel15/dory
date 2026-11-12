// Package peer implements device identity and the signed membership log.
package peer

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	keyFile  = "identity.key"
	metaFile = "identity.json"
)

// Identity is the local device identity: an Ed25519 key pair plus a name.
type Identity struct {
	Name string
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
	dir  string
}

// DeviceID derives the device id from a public key.
func DeviceID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:16])
}

// LoadOrCreate loads the identity from dir, generating the key on first start.
func LoadOrCreate(dir, defaultName string) (*Identity, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	id := &Identity{dir: dir}
	kp := filepath.Join(dir, keyFile)
	seed, err := os.ReadFile(kp)
	switch {
	case err == nil:
		raw, derr := hex.DecodeString(string(seed))
		if derr != nil || len(raw) != ed25519.SeedSize {
			return nil, errors.New("peer: corrupt identity key")
		}
		id.priv = ed25519.NewKeyFromSeed(raw)
	case errors.Is(err, os.ErrNotExist):
		_, priv, gerr := ed25519.GenerateKey(rand.Reader)
		if gerr != nil {
			return nil, gerr
		}
		id.priv = priv
		if werr := writeFile0600(kp, []byte(hex.EncodeToString(priv.Seed()))); werr != nil {
			return nil, werr
		}
	default:
		return nil, err
	}
	id.pub = id.priv.Public().(ed25519.PublicKey)

	id.Name = defaultName
	if b, err := os.ReadFile(filepath.Join(dir, metaFile)); err == nil {
		var m struct{ Name string }
		if json.Unmarshal(b, &m) == nil && m.Name != "" {
			id.Name = m.Name
		}
	}
	if id.Name == "" {
		h, _ := os.Hostname()
		id.Name = h
	}
	return id, nil
}

// SetName renames the device and persists the change.
func (i *Identity) SetName(name string) error {
	if name == "" {
		return errors.New("peer: empty device name")
	}
	b, _ := json.Marshal(struct{ Name string }{name})
	if err := writeFile0600(filepath.Join(i.dir, metaFile), b); err != nil {
		return err
	}
	i.Name = name
	return nil
}

func (i *Identity) PublicKey() ed25519.PublicKey { return i.pub }
func (i *Identity) PublicKeyHex() string         { return hex.EncodeToString(i.pub) }
func (i *Identity) DeviceID() string             { return DeviceID(i.pub) }
func (i *Identity) Sign(msg []byte) []byte       { return ed25519.Sign(i.priv, msg) }

func writeFile0600(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("peer: %w", err)
	}
	return nil
}
