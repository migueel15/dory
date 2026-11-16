package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

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
{"timestamp":"2026-11-16T01:00:05Z","type":"response_item","payload":{"type":"message","id":"m2","role":"assistant","content":[{"type":"output_text","text":"hola"}]}}
`), 0o644)
	list := ExternalSessions(home)
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
	if len(msgs) != 3 || msgs[0].Text != "Hola codex" || msgs[1].Text != "b.go" || msgs[2].Text != "hola" {
		t.Fatalf("codex messages %+v", msgs)
	}
}
