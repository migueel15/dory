package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/migueel15/dory/internal/apperr"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Secrets asked through request_secret never reach the model, the database
// or the logs: the values go to owner-only files and the agent gets their
// paths. Only the prompt (label, reason, fields) is stored.

// SecretTTL is how long a secret file lives on disk.
const SecretTTL = 24 * time.Hour

// SecretField is one value of a secret form.
type SecretField struct {
	Name        string `json:"name" jsonschema:"Key of the value in the result, e.g. token; letters, digits and _"`
	Label       string `json:"label" jsonschema:"What the user sees above the box"`
	Placeholder string `json:"placeholder,omitempty"`
	// Secret hides what the user types; nil means true.
	Secret *bool `json:"secret,omitempty" jsonschema:"false shows what is typed (an email, an id); default true"`
}

// SecretPrompt is what the user sees: never the values.
type SecretPrompt struct {
	Label       string        `json:"label"`
	Reason      string        `json:"reason"`
	URL         string        `json:"url,omitempty"`
	Placeholder string        `json:"placeholder,omitempty"`
	Fields      []SecretField `json:"fields,omitempty"`
}

// SecretResult is what the agent gets back.
type SecretResult struct {
	Status string            `json:"status"`
	File   string            `json:"file,omitempty"`
	Files  map[string]string `json:"files,omitempty"`
	Note   string            `json:"note,omitempty"`
}

// secretReply is the user's answer to a secret request.
type secretReply struct {
	values  map[string]string
	decline bool
}

var (
	fieldName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)
	// ErrSecretFields is a reply whose fields do not match the request.
	ErrSecretFields = apperr.New(apperr.Invalid, "the values do not match the requested fields")
)

// Validate checks the prompt before asking.
func (p SecretPrompt) Validate() error {
	if strings.TrimSpace(p.Label) == "" || strings.TrimSpace(p.Reason) == "" {
		return errors.New("request_secret needs a label and a reason")
	}
	if len(p.Fields) > 8 {
		return errors.New("request_secret takes at most 8 fields")
	}
	seen := map[string]bool{}
	for _, f := range p.Fields {
		if !fieldName.MatchString(f.Name) {
			return fmt.Errorf("invalid field name %q", f.Name)
		}
		if seen[f.Name] {
			return fmt.Errorf("repeated field name %q", f.Name)
		}
		seen[f.Name] = true
	}
	return nil
}

// names are the keys the reply must carry: the fields, or "value" alone.
func (p SecretPrompt) names() []string {
	if len(p.Fields) == 0 {
		return []string{"value"}
	}
	out := make([]string, len(p.Fields))
	for i, f := range p.Fields {
		out[i] = f.Name
	}
	return out
}

func (m *Manager) secretDir() string { return filepath.Join(m.dataDir, "secrets", "files") }

// writeSecret stores one value in a new owner-only file and returns its path.
func (m *Manager) writeSecret(value string) (string, error) {
	dir := m.secretDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	path := filepath.Join(dir, hex.EncodeToString(b))
	tmp := path + ".partial"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(value)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// SaveSecret writes a value the user already gave (pasted in the chat).
func (m *Manager) SaveSecret(value string) (SecretResult, error) {
	path, err := m.writeSecret(value)
	if err != nil {
		return SecretResult{}, err
	}
	return SecretResult{Status: "saved", File: path}, nil
}

// RequestSecret shows a secret form to the user and waits for it, at most
// timeout. The values go to files; the result holds their paths.
func (m *Manager) RequestSecret(ctx context.Context, thread string, p SecretPrompt, timeout time.Duration) (SecretResult, error) {
	if _, err := m.local(ctx, thread); err != nil {
		return SecretResult{}, err
	}
	req := newRunID()
	reply := make(chan secretReply, 1)
	m.mu.Lock()
	m.secretWaiters[req] = secretWaiter{prompt: p, reply: reply}
	m.mu.Unlock()
	raw, _ := json.Marshal(p)
	if _, err := m.db.ExecContext(ctx, `INSERT INTO thread_requests(id, thread_id, kind, message_id, questions, created_at) VALUES (?, ?, 'secret', '', ?, ?)`, req, thread, string(raw), now()); err != nil {
		m.forgetSecret(req)
		return SecretResult{}, err
	}
	m.setStatus(ctx, thread, "waiting", "")
	m.publish(Update{ThreadID: thread, Reload: true})
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	bg := context.WithoutCancel(ctx)
	select {
	case r := <-reply:
		if r.decline {
			return SecretResult{Status: "declined", Note: "The user declined. Do not ask again unless they say so; go on without it or ask how to proceed."}, nil
		}
		return m.saveReply(p, r.values)
	case <-timer.C:
		m.forgetSecret(req)
		m.resolved(bg, thread, req)
		return SecretResult{Status: "timed_out", Note: "Nobody answered in time. Tell the user and ask again only when they are back."}, nil
	case <-ctx.Done():
		m.forgetSecret(req)
		m.db.ExecContext(bg, `DELETE FROM thread_requests WHERE id = ?`, req)
		m.publish(Update{ThreadID: thread, Reload: true})
		return SecretResult{Status: "cancelled"}, ctx.Err()
	}
}

func (m *Manager) forgetSecret(req string) {
	m.mu.Lock()
	delete(m.secretWaiters, req)
	m.mu.Unlock()
}

func (m *Manager) saveReply(p SecretPrompt, values map[string]string) (SecretResult, error) {
	if len(p.Fields) == 0 {
		return m.SaveSecret(values["value"])
	}
	out := SecretResult{Status: "saved", Files: map[string]string{}}
	for _, n := range p.names() {
		path, err := m.writeSecret(values[n])
		if err != nil {
			for _, f := range out.Files {
				os.Remove(f)
			}
			return SecretResult{}, err
		}
		out.Files[n] = path
	}
	return out, nil
}

// AnswerSecret hands the user's values (or a decline) to the waiting call.
// The values must match the requested fields exactly.
func (m *Manager) AnswerSecret(ctx context.Context, thread, id string, values map[string]string, decline bool) error {
	if _, err := m.local(ctx, thread); err != nil {
		return err
	}
	var owner string
	if err := m.db.QueryRowContext(ctx, `SELECT thread_id FROM thread_requests WHERE id = ? AND kind = 'secret'`, id).Scan(&owner); err != nil || owner != thread {
		return ErrNoRequest
	}
	m.mu.Lock()
	w, ok := m.secretWaiters[id]
	m.mu.Unlock()
	if !ok {
		return ErrNoRequest
	}
	if !decline {
		names := w.prompt.names()
		if len(values) != len(names) {
			return ErrSecretFields
		}
		for _, n := range names {
			if _, ok := values[n]; !ok {
				return ErrSecretFields
			}
		}
	}
	m.forgetSecret(id)
	w.reply <- secretReply{values: values, decline: decline}
	return m.resolved(ctx, thread, id)
}

type secretWaiter struct {
	prompt SecretPrompt
	reply  chan secretReply
}

// sweepSecrets removes the secret files older than SecretTTL, and leftover
// partial writes.
func (m *Manager) sweepSecrets() {
	if m.dataDir == "" {
		return
	}
	entries, err := os.ReadDir(m.secretDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > SecretTTL || (strings.HasSuffix(e.Name(), ".partial") && time.Since(info.ModTime()) > time.Minute) {
			if err := os.Remove(filepath.Join(m.secretDir(), e.Name())); err != nil {
				log.Printf("runtime: sweep secret: %v", err)
			}
		}
	}
}

// SweepSecrets clears expired secrets now and then every hour until ctx ends.
func (m *Manager) SweepSecrets(ctx context.Context) {
	m.sweepSecrets()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.sweepSecrets()
		}
	}
}

// redactSecret drops the value a request_secret call carries in its
// arguments, so a pasted secret is not kept in the chat.
func redactSecret(msg *Message) {
	t := msg.Tool
	if t == nil || !(strings.HasSuffix(t.Name, "request_secret") || t.Tool == "request_secret") || t.Input == "" {
		return
	}
	var in map[string]any
	if json.Unmarshal([]byte(t.Input), &in) != nil {
		t.Input = ""
		return
	}
	if v, ok := in["value"].(string); ok && v != "" {
		in["value"] = "[redacted]"
		msg.Text = strings.ReplaceAll(msg.Text, v, "[redacted]")
		msg.Detail = strings.ReplaceAll(msg.Detail, v, "[redacted]")
		raw, _ := json.Marshal(in)
		t.Input = string(raw)
	}
}
