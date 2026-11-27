package app

import (
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/runtime"
)

func TestProvisionalTitle(t *testing.T) {
	cases := map[string]string{
		"\n  Arreglar el login. Luego más cosas.\nsegunda": "Arreglar el login.",
		"sin puntuación v1.2 corta":                        "sin puntuación v1.2 corta",
		strings.Repeat("a", 100):                           strings.Repeat("a", 80),
	}
	for in, want := range cases {
		if got := provisionalTitle(in); got != want {
			t.Errorf("provisionalTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidTitle(t *testing.T) {
	if got, ok := validTitle(" «Añadir títulos automáticos» \n"); !ok || got != "Añadir títulos automáticos" {
		t.Errorf("got %q %v", got, ok)
	}
	for _, bad := range []string{"", "dos\nlíneas", "# Título", strings.Repeat("x", 81)} {
		if _, ok := validTitle(bad); ok {
			t.Errorf("validTitle(%q) accepted", bad)
		}
	}
}

func TestIdeaDescription(t *testing.T) {
	files := []runtime.Attachment{
		{ID: "run-1", Name: "captura.png", Mime: "image/png", Path: "/tmp/tasks/t1/run-1-captura.png"},
		{ID: "run-2", Name: "spec.pdf", Mime: "application/pdf", Path: "/tmp/tasks/t1/run-2-spec.pdf"},
	}
	got := ideaDescription("t1", "Mira esta pantalla", files)
	for _, want := range []string{
		"Mira esta pantalla",
		"/tmp/tasks/t1/run-1-captura.png (captura.png)",
		"![captura.png](/api/tasks/t1/attachments/run-1)",
		"[spec.pdf](/api/tasks/t1/attachments/run-2)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ideaDescription missing %q in:\n%s", want, got)
		}
	}
}
