package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pendingSecret waits for the secret request of a thread.
func pendingSecret(t *testing.T, m *Manager, thread string) Request {
	t.Helper()
	for range 200 {
		if rs, _ := m.Requests(context.Background(), thread); len(rs) == 1 {
			return rs[0]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("request_secret left no pending request")
	return Request{}
}

func TestRequestSecretSavesFilesByField(t *testing.T) {
	m, _ := manager(t, 0)
	ctx := context.Background()
	th, err := m.Create(ctx, NewThread{Provider: "replay", Title: "Secreto"})
	if err != nil {
		t.Fatal(err)
	}
	hidden := false
	p := SecretPrompt{Label: "Cuenta", Reason: "Desplegar", Fields: []SecretField{{Name: "account", Label: "ID", Secret: &hidden}, {Name: "token", Label: "Token"}, {Name: "pin", Label: "PIN"}}}
	got := make(chan SecretResult, 1)
	go func() {
		r, err := m.RequestSecret(ctx, th.ID, p, time.Minute)
		if err != nil {
			t.Error(err)
		}
		got <- r
	}()
	req := pendingSecret(t, m, th.ID)
	if req.Kind != "secret" || req.Secret == nil || len(req.Secret.Fields) != 3 {
		t.Fatalf("request %+v", req)
	}
	if err := m.AnswerSecret(ctx, th.ID, req.ID, map[string]string{"account": "acc-1"}, false); err != ErrSecretFields {
		t.Fatalf("missing fields: %v", err)
	}
	if err := m.AnswerSecret(ctx, th.ID, req.ID, map[string]string{"account": "a", "token": "b", "pin": "c", "extra": "d"}, false); err != ErrSecretFields {
		t.Fatalf("extra field: %v", err)
	}
	values := map[string]string{"account": "acc-1", "token": "tok-SECRETO", "pin": "4321"}
	if err := m.AnswerSecret(ctx, th.ID, req.ID, values, false); err != nil {
		t.Fatal(err)
	}
	r := <-got
	if r.Status != "saved" || len(r.Files) != 3 {
		t.Fatalf("result %+v", r)
	}
	for name, path := range r.Files {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", name, info, err)
		}
		if b, _ := os.ReadFile(path); string(b) != values[name] {
			t.Fatalf("%s holds %q", name, b)
		}
	}
	if info, _ := os.Stat(filepath.Dir(r.Files["token"])); info.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v", info.Mode())
	}
	// The values are nowhere in the database.
	var hits int
	for _, q := range []string{`SELECT count(*) FROM thread_requests WHERE questions LIKE '%tok-SECRETO%'`, `SELECT count(*) FROM messages WHERE text LIKE '%tok-SECRETO%'`} {
		var n int
		if err := m.db.QueryRowContext(ctx, q).Scan(&n); err != nil && !strings.Contains(err.Error(), "no such table") {
			t.Fatal(err)
		}
		hits += n
	}
	if hits != 0 {
		t.Fatal("the secret reached the database")
	}
	if rs, _ := m.Requests(ctx, th.ID); len(rs) != 0 {
		t.Fatalf("requests left: %v", rs)
	}
}

func TestRequestSecretDeclineAndTimeoutWriteNothing(t *testing.T) {
	m, _ := manager(t, 0)
	ctx := context.Background()
	th, _ := m.Create(ctx, NewThread{Provider: "replay", Title: "Secreto"})
	p := SecretPrompt{Label: "sudo", Reason: "Instalar"}
	got := make(chan SecretResult, 1)
	go func() { r, _ := m.RequestSecret(ctx, th.ID, p, time.Minute); got <- r }()
	req := pendingSecret(t, m, th.ID)
	if err := m.AnswerSecret(ctx, th.ID, req.ID, nil, true); err != nil {
		t.Fatal(err)
	}
	if r := <-got; r.Status != "declined" {
		t.Fatalf("result %+v", r)
	}
	if r, _ := m.RequestSecret(ctx, th.ID, p, 20*time.Millisecond); r.Status != "timed_out" {
		t.Fatalf("result %+v", r)
	}
	if rs, _ := m.Requests(ctx, th.ID); len(rs) != 0 {
		t.Fatalf("requests left: %v", rs)
	}
	if entries, _ := os.ReadDir(m.secretDir()); len(entries) != 0 {
		t.Fatalf("files written: %v", entries)
	}
}

func TestSweepSecretsRemovesOldFiles(t *testing.T) {
	m, _ := manager(t, 0)
	r, err := m.SaveSecret("pegado")
	if err != nil || r.Status != "saved" {
		t.Fatal(r, err)
	}
	fresh, _ := m.SaveSecret("nuevo")
	old := time.Now().Add(-25 * time.Hour)
	os.Chtimes(r.File, old, old)
	m.sweepSecrets()
	if _, err := os.Stat(r.File); !os.IsNotExist(err) {
		t.Fatal("old secret kept")
	}
	if _, err := os.Stat(fresh.File); err != nil {
		t.Fatal("fresh secret removed")
	}
}

func TestSecretPromptValidate(t *testing.T) {
	for _, p := range []SecretPrompt{
		{Label: "x"},
		{Label: "x", Reason: "y", Fields: []SecretField{{Name: "1a"}}},
		{Label: "x", Reason: "y", Fields: []SecretField{{Name: "a"}, {Name: "a"}}},
	} {
		if p.Validate() == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
}

func TestRedactSecretValue(t *testing.T) {
	msg := Message{Text: "x hunter2", Tool: &ToolCall{Name: "mcp__dory__request_secret", Input: `{"label":"a","value":"hunter2"}`}}
	redactSecret(&msg)
	if strings.Contains(msg.Tool.Input+msg.Text, "hunter2") {
		t.Fatalf("leaked: %+v %s", msg, msg.Tool.Input)
	}
}
