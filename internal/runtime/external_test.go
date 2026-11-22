package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExternalMessagesKeepDoryContext(t *testing.T) {
	prompt := "<dory-context kind=\"plan\" task=\"t1\">\nContexto para el agente\n</dory-context>\n\nPetición completa del usuario"
	for _, provider := range []string{"codex", "claude", "claude-block"} {
		t.Run(provider, func(t *testing.T) {
			var record any
			switch provider {
			case "codex":
				record = map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "id": "u1", "role": "user", "content": []any{map[string]any{"text": prompt}}}}
			case "claude":
				record = map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": prompt}}
			default:
				record = map[string]any{"type": "user", "uuid": "u1", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": prompt}}}}
			}
			line, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "session.jsonl")
			if err := os.WriteFile(path, line, 0o600); err != nil {
				t.Fatal(err)
			}
			kind := provider
			if kind == "claude-block" {
				kind = "claude"
			}
			messages, err := ExternalMessages(External{Provider: kind, path: path})
			if err != nil || len(messages) != 1 || messages[0].Text != prompt {
				t.Fatalf("messages = %+v, error = %v", messages, err)
			}
		})
	}
}

func TestExternalSessions(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, ".claude", "projects", "-repo")
	codex := filepath.Join(home, ".codex", "sessions", "2026", "11", "16")
	os.MkdirAll(claude, 0o755)
	os.MkdirAll(codex, 0o755)
	os.WriteFile(filepath.Join(claude, "s1.jsonl"), []byte(`{"type":"user","uuid":"u1","sessionId":"s1","cwd":"/repo","gitBranch":"main","timestamp":"2026-11-16T00:00:00Z","message":{"content":"Arregla el login"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-11-16T00:00:01Z","message":{"content":[{"type":"thinking","thinking":"Miro"},{"type":"text","text":"Voy"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}
{"type":"user","uuid":"u2","sessionId":"s1","timestamp":"2026-11-16T00:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"a.go"}]}}
{"type":"custom-title","customTitle":"Login roto","sessionId":"s1"}
`), 0o644)
	os.WriteFile(filepath.Join(codex, "rollout-x.jsonl"), []byte(`{"timestamp":"2026-11-16T01:00:00Z","type":"session_meta","payload":{"session_id":"c1","cwd":"/repo2"}}
{"timestamp":"2026-11-16T01:00:01Z","type":"response_item","payload":{"type":"message","id":"m0","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions"}]}}
{"timestamp":"2026-11-16T01:00:02Z","type":"response_item","payload":{"type":"message","id":"m1","role":"user","content":[{"type":"input_text","text":"Hola codex"}]}}
{"timestamp":"2026-11-16T01:00:03Z","type":"response_item","payload":{"type":"function_call","name":"exec","arguments":"{\"command\":\"ls\"}","call_id":"k1"}}
{"timestamp":"2026-11-16T01:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"k1","output":"b.go"}}
{"timestamp":"2026-11-16T01:00:05Z","type":"response_item","payload":{"type":"message","id":"m2","role":"assistant","content":[{"type":"output_text","text":"hola\n\n<oai-mem-citation>\n<citation_entries>referencia</citation_entries>\n</oai-mem-citation>"}]}}
`), 0o644)
	list := ExternalSessions(Paths{Home: home})
	if len(list) != 2 {
		t.Fatalf("sessions %+v", list)
	}
	byProvider := map[string]External{}
	for _, e := range list {
		byProvider[e.Provider] = e
	}
	if c := byProvider["claude"]; c.ID != "s1" || c.Title != "Login roto" || c.Dir != "/repo" || c.Branch != "main" {
		t.Fatalf("claude %+v", c)
	}
	if c := byProvider["codex"]; c.ID != "c1" || c.Title != "Hola codex" || c.Dir != "/repo2" {
		t.Fatalf("codex %+v", c)
	}
	msgs, _ := ExternalMessages(byProvider["claude"])
	if len(msgs) != 4 || msgs[3].Role != "tool" || msgs[3].Text != "a.go" || msgs[3].Detail != "ls" {
		t.Fatalf("claude messages %+v", msgs)
	}
	msgs, _ = ExternalMessages(byProvider["codex"])
	if len(msgs) != 3 || msgs[0].Text != "Hola codex" || msgs[1].Text != "b.go" || msgs[2].Text != "hola\n\n" {
		t.Fatalf("codex messages %+v", msgs)
	}
}

func TestFollowPicksUpTerminalTurns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	m, _ := manager(t, 0)
	m.providers["claude"] = Claude{}
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "-repo")
	os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "s9.jsonl")
	os.WriteFile(file, []byte(`{"type":"user","uuid":"u1","sessionId":"s9","cwd":"/repo","timestamp":"2026-11-16T00:00:00Z","message":{"content":"Primera"}}
`+"\n"), 0o644)
	e, err := ExternalSession(Paths{Home: home}, "claude", "s9")
	if err != nil {
		t.Fatal(err)
	}
	th, err := m.Adopt(ctx, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	updates, stop := m.Subscribe(th.ID)
	defer stop()
	go m.Follow(ctx, Paths{Home: home})
	time.Sleep(2500 * time.Millisecond) // first pass records the file
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"user","uuid":"u2","sessionId":"s9","timestamp":"2026-11-16T00:01:00Z","message":{"content":"Desde la terminal"}}` + "\n")
	f.Close()
	for {
		select {
		case <-ctx.Done():
			t.Fatal("terminal turn never reached Dory")
		case u := <-updates:
			if !u.Reload {
				continue
			}
			msgs, _ := m.Messages(ctx, th.ID)
			if len(msgs) == 2 && msgs[1].Text == "Desde la terminal" {
				return
			}
		}
	}
}
