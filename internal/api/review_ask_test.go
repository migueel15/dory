package api

import (
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/github"
)

func TestReviewPrompt(t *testing.T) {
	got := reviewPrompt(7, "a/b.go", 3, 5, "+x\n-y\n", "¿Por qué?")
	want := "Consulta de revisión sobre PR #7, fichero a/b.go líneas 3–5:\n```diff\n+x\n-y\n```\n¿Por qué?\n\nResponde sin modificar código salvo que se pida."
	if got != want {
		t.Fatalf("got %q", got)
	}
	if got := reviewPrompt(7, "a", 4, 4, "", "q"); !strings.Contains(got, "fichero a líneas 4:") {
		t.Fatalf("single line: %q", got)
	}
	if got := reviewPrompt(7, "a.go", 0, 0, "", "q"); !strings.Contains(got, "fichero a.go completo:\nq") {
		t.Fatalf("whole file: %q", got)
	}
}

func TestReviewIntroEndsWithQuestion(t *testing.T) {
	pr := &github.Pull{Number: 7, Repo: "o/r", Title: "T", URL: "https://github.com/o/r/pull/7", HeadRef: "feat/x", BaseRef: "main"}
	got := reviewIntro(pr, "PREGUNTA")
	if !strings.HasSuffix(got, "\n\nPREGUNTA") || !strings.Contains(got, "Rama: feat/x → main") || strings.Contains(got, "%!") {
		t.Fatalf("intro: %q", got)
	}
}
