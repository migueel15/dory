package t3

import "testing"

func TestAccountKey(t *testing.T) {
	a := accountKey("codex", "a@x.com", "w1", "")
	if a == "" || a != accountKey("codex", "a@x.com", "w1", "fp") {
		t.Fatalf("same email and workspace must give the same key: %q", a)
	}
	if a == accountKey("codex", "a@x.com", "w2", "") || a == accountKey("claudeAgent", "a@x.com", "w1", "") {
		t.Fatal("another workspace or provider must give another key")
	}
	if accountKey("gemini", "", "", "fp") != accountKey("gemini", "", "", "fp") || accountKey("gemini", "", "", "") != "" {
		t.Fatal("fingerprint gives a stable key; nothing gives none")
	}
}
