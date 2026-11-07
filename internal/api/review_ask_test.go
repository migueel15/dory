package api

import (
	"strings"
	"testing"
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
}
