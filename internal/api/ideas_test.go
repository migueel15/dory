package api

import (
	"strings"
	"testing"
)

func TestProvisionalTitle(t *testing.T) {
	cases := map[string]string{
		"\n  Arreglar el login. Luego más cosas.\nsegunda": "Arreglar el login.",
		"sin puntuación v1.2 corta":                     "sin puntuación v1.2 corta",
		strings.Repeat("a", 100):                        strings.Repeat("a", 80),
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
