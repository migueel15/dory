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
	"sort"
	"sync"
	"time"
)

type Action string

const (
	ActionAdd    Action = "add"
	ActionRevoke Action = "revoke"
)

// Entry is one signed membership change. Prevs are the ids of the log heads
// the signer knew when signing; the signer must be a valid member in the
// state defined by those entries and their ancestors. Concurrent entries
// therefore do not invalidate each other (mutual revokes revoke both).
type Entry struct {
	ID        string   `json:"id"`
	GroupID   string   `json:"group_id"`
	Action    Action   `json:"action"`
	Subject   string   `json:"subject"` // hex public key
	Name      string   `json:"name"`
	Signer    string   `json:"signer"` // hex public key
	Prevs     []string `json:"prevs"`
	Timestamp int64    `json:"timestamp"` // unix nanoseconds, informational
	Signature string   `json:"signature"`
}

type signedPayload struct {
	GroupID   string   `json:"group_id"`
	Action    Action   `json:"action"`
	Subject   string   `json:"subject"`
	Name      string   `json:"name"`
	Signer    string   `json:"signer"`
	Prevs     []string `json:"prevs"`
	Timestamp int64    `json:"timestamp"`
}

func (e Entry) payload() []byte {
	b, _ := json.Marshal(signedPayload{e.GroupID, e.Action, e.Subject, e.Name, e.Signer, e.Prevs, e.Timestamp})
	return b
}

func (e Entry) computeID() string {
	h := sha256.New()
	h.Write(e.payload())
	h.Write([]byte(e.Signature))
	return hex.EncodeToString(h.Sum(nil))
}

// Log is a signed, replicable membership log (a DAG of entries).
type Log struct {
	GroupID string  `json:"group_id"`
	Entries []Entry `json:"entries"`

	mu sync.RWMutex // guards Entries once the log is shared with a server
}

// Clone returns a consistent copy of the log.
func (l *Log) Clone() *Log {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return &Log{GroupID: l.GroupID, Entries: append([]Entry{}, l.Entries...)}
}

// NewGroup creates a log whose first entry is self-signed by the creator.
func NewGroup(creator *Identity) (*Log, error) {
	gid := make([]byte, 16)
	if _, err := rand.Read(gid); err != nil {
		return nil, err
	}
	l := &Log{GroupID: hex.EncodeToString(gid)}
	e := l.build(creator, ActionAdd, creator.PublicKeyHex(), creator.Name, nil)
	l.Entries = append(l.Entries, e)
	return l, nil
}

func (l *Log) build(signer *Identity, a Action, subject, name string, prevs []string) Entry {
	e := Entry{GroupID: l.GroupID, Action: a, Subject: subject, Name: name,
		Signer: signer.PublicKeyHex(), Prevs: prevs, Timestamp: time.Now().UnixNano()}
	e.Signature = hex.EncodeToString(signer.Sign(e.payload()))
	e.ID = e.computeID()
	return e
}

func (l *Log) heads() []string {
	ref := map[string]bool{}
	for _, e := range l.Entries {
		for _, p := range e.Prevs {
			ref[p] = true
		}
	}
	var out []string
	for _, e := range l.Entries {
		if !ref[e.ID] {
			out = append(out, e.ID)
		}
	}
	sort.Strings(out)
	return out
}

// Append signs a new entry with signer and adds it, verifying the result.
func (l *Log) Append(signer *Identity, a Action, subjectPub ed25519.PublicKey, name string) (Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.build(signer, a, hex.EncodeToString(subjectPub), name, l.heads())
	if err := l.addLocked(e); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// Add inserts a (possibly replicated) entry if the resulting log verifies.
func (l *Log) Add(e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.addLocked(e)
}

func (l *Log) addLocked(e Entry) error {
	for _, x := range l.Entries {
		if x.ID == e.ID {
			return nil
		}
	}
	cand := &Log{GroupID: l.GroupID, Entries: append(append([]Entry{}, l.Entries...), e)}
	if err := cand.Verify(); err != nil {
		return err
	}
	l.Entries = cand.Entries
	return nil
}

// Verify checks the whole chain: ids, signatures, causality and signer validity.
func (l *Log) Verify() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	byID := map[string]Entry{}
	for _, e := range l.Entries {
		if _, dup := byID[e.ID]; dup {
			return fmt.Errorf("peer: duplicate entry %s", e.ID)
		}
		byID[e.ID] = e
	}
	roots := 0
	done := map[string]bool{}
	var visit func(id string, stack map[string]bool) error
	visit = func(id string, stack map[string]bool) error {
		if done[id] {
			return nil
		}
		e := byID[id]
		if stack[id] {
			return errors.New("peer: cycle in log")
		}
		stack[id] = true
		for _, p := range e.Prevs {
			if _, ok := byID[p]; !ok {
				return fmt.Errorf("peer: entry %s references unknown %s", id, p)
			}
			if err := visit(p, stack); err != nil {
				return err
			}
		}
		delete(stack, id)
		if err := l.verifyEntry(e, byID); err != nil {
			return err
		}
		if len(e.Prevs) == 0 {
			roots++
		}
		done[id] = true
		return nil
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id, map[string]bool{}); err != nil {
			return err
		}
	}
	if roots != 1 {
		return fmt.Errorf("peer: expected exactly one root entry, got %d", roots)
	}
	return nil
}

func (l *Log) verifyEntry(e Entry, byID map[string]Entry) error {
	if e.GroupID != l.GroupID {
		return fmt.Errorf("peer: entry %s from another group", e.ID)
	}
	if e.Action != ActionAdd && e.Action != ActionRevoke {
		return fmt.Errorf("peer: entry %s has unknown action", e.ID)
	}
	if e.computeID() != e.ID {
		return fmt.Errorf("peer: entry %s id mismatch (tampered)", e.ID)
	}
	signer, err := hex.DecodeString(e.Signer)
	if err != nil || len(signer) != ed25519.PublicKeySize {
		return fmt.Errorf("peer: entry %s bad signer key", e.ID)
	}
	sub, err := hex.DecodeString(e.Subject)
	if err != nil || len(sub) != ed25519.PublicKeySize {
		return fmt.Errorf("peer: entry %s bad subject key", e.ID)
	}
	sig, err := hex.DecodeString(e.Signature)
	if err != nil || !ed25519.Verify(signer, e.payload(), sig) {
		return fmt.Errorf("peer: entry %s invalid signature", e.ID)
	}
	if len(e.Prevs) == 0 {
		if e.Action != ActionAdd || e.Signer != e.Subject {
			return fmt.Errorf("peer: root entry %s must be self-signed add", e.ID)
		}
		return nil
	}
	added, revoked := ancestorState(e.Prevs, byID)
	if !added[e.Signer] || revoked[e.Signer] {
		return fmt.Errorf("peer: entry %s signer is not a valid member", e.ID)
	}
	switch e.Action {
	case ActionAdd:
		if added[e.Subject] {
			return fmt.Errorf("peer: entry %s subject already known", e.ID)
		}
	case ActionRevoke:
		if !added[e.Subject] || revoked[e.Subject] {
			return fmt.Errorf("peer: entry %s subject is not an active member", e.ID)
		}
	}
	return nil
}

// ancestorState computes added/revoked sets over the closure of prevs.
func ancestorState(prevs []string, byID map[string]Entry) (added, revoked map[string]bool) {
	added, revoked = map[string]bool{}, map[string]bool{}
	seen := map[string]bool{}
	stack := append([]string{}, prevs...)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		e := byID[id]
		if e.Action == ActionAdd {
			added[e.Subject] = true
		} else {
			revoked[e.Subject] = true
		}
		stack = append(stack, e.Prevs...)
	}
	return
}

// Member is an active group member.
type Member struct {
	PublicKey string `json:"public_key"`
	DeviceID  string `json:"device_id"`
	Name      string `json:"name"`
}

// Members returns the active (added, not revoked) members. Call Verify first
// for untrusted logs.
func (l *Log) Members() []Member {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.members()
}

func (l *Log) members() []Member {
	revoked := map[string]bool{}
	names := map[string]string{}
	var order []string
	for _, e := range l.Entries {
		if e.Action == ActionRevoke {
			revoked[e.Subject] = true
		} else if _, ok := names[e.Subject]; !ok {
			names[e.Subject] = e.Name
			order = append(order, e.Subject)
		}
	}
	var out []Member
	for _, k := range order {
		if revoked[k] {
			continue
		}
		raw, _ := hex.DecodeString(k)
		out = append(out, Member{k, DeviceID(raw), names[k]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublicKey < out[j].PublicKey })
	return out
}

// Allowed reports whether pub is an active member.
func (l *Log) Allowed(pub ed25519.PublicKey) bool {
	k := hex.EncodeToString(pub)
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, m := range l.members() {
		if m.PublicKey == k {
			return true
		}
	}
	return false
}

// SaveFile writes the log as JSON.
func (l *Log) SaveFile(path string) error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// LoadLog reads and verifies a log from a JSON file.
func LoadLog(path string) (*Log, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var l Log
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	if err := l.Verify(); err != nil {
		return nil, err
	}
	return &l, nil
}
