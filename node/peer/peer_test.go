package peer

import (
	"path/filepath"
	"testing"
)

func ident(t *testing.T, name string) *Identity {
	t.Helper()
	i, err := LoadOrCreate(t.TempDir(), name)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func TestValidChain(t *testing.T) {
	a, b, c := ident(t, "a"), ident(t, "b"), ident(t, "c")
	l, _ := NewGroup(a)
	if _, err := l.Append(a, ActionAdd, b.PublicKey(), "b"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(b, ActionAdd, c.PublicKey(), "c"); err != nil {
		t.Fatal(err)
	}
	if err := l.Verify(); err != nil || len(l.Members()) != 3 || !l.Allowed(c.PublicKey()) {
		t.Fatalf("err=%v members=%v", err, l.Members())
	}
	if _, err := l.Append(a, ActionRevoke, c.PublicKey(), ""); err != nil {
		t.Fatal(err)
	}
	if l.Allowed(c.PublicKey()) || len(l.Members()) != 2 {
		t.Fatal("c should be revoked")
	}
}

func TestTamperedRejected(t *testing.T) {
	a, b := ident(t, "a"), ident(t, "b")
	l, _ := NewGroup(a)
	l.Append(a, ActionAdd, b.PublicKey(), "b")
	l.Entries[1].Name = "evil"
	if l.Verify() == nil {
		t.Fatal("tampered name accepted")
	}
	l.Entries[1].Name = "b"
	l.Entries[1].Signature = l.Entries[0].Signature
	if l.Verify() == nil {
		t.Fatal("bad signature accepted")
	}
}

func TestRevokedSignerRejected(t *testing.T) {
	a, b, c := ident(t, "a"), ident(t, "b"), ident(t, "c")
	l, _ := NewGroup(a)
	l.Append(a, ActionAdd, b.PublicKey(), "b")
	l.Append(a, ActionRevoke, b.PublicKey(), "")
	if _, err := l.Append(b, ActionAdd, c.PublicKey(), "c"); err == nil {
		t.Fatal("entry from revoked signer accepted")
	}
	if _, err := l.Append(c, ActionAdd, c.PublicKey(), "c"); err == nil {
		t.Fatal("non-member signer accepted")
	}
}

func TestCrossRevocations(t *testing.T) {
	a, b := ident(t, "a"), ident(t, "b")
	la, _ := NewGroup(a)
	la.Append(a, ActionAdd, b.PublicKey(), "b")
	lb := &Log{GroupID: la.GroupID, Entries: append([]Entry{}, la.Entries...)}
	ea, err := la.Append(a, ActionRevoke, b.PublicKey(), "")
	if err != nil {
		t.Fatal(err)
	}
	eb, err := lb.Append(b, ActionRevoke, a.PublicKey(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := la.Add(eb); err != nil {
		t.Fatal(err)
	}
	if err := lb.Add(ea); err != nil {
		t.Fatal(err)
	}
	for _, l := range []*Log{la, lb} {
		if len(l.Members()) != 0 || l.Allowed(a.PublicKey()) || l.Allowed(b.PublicKey()) {
			t.Fatal("both should be revoked")
		}
	}
}

func TestKeyPersists(t *testing.T) {
	dir := t.TempDir()
	i1, _ := LoadOrCreate(dir, "x")
	i2, _ := LoadOrCreate(dir, "y")
	if i1.DeviceID() != i2.DeviceID() || i1.PublicKeyHex() != i2.PublicKeyHex() {
		t.Fatal("key changed")
	}
	if err := i2.SetName("laptop"); err != nil {
		t.Fatal(err)
	}
	i3, _ := LoadOrCreate(dir, "z")
	if i3.Name != "laptop" {
		t.Fatalf("name %q", i3.Name)
	}
}

func TestLogFileRoundTrip(t *testing.T) {
	a, b := ident(t, "a"), ident(t, "b")
	l, _ := NewGroup(a)
	l.Append(a, ActionAdd, b.PublicKey(), "b")
	p := filepath.Join(t.TempDir(), "log.json")
	if err := l.SaveFile(p); err != nil {
		t.Fatal(err)
	}
	l2, err := LoadLog(p)
	if err != nil || len(l2.Members()) != 2 {
		t.Fatalf("%v", err)
	}
}
