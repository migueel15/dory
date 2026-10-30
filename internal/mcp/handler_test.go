package mcp

import (
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/service"
)

func TestNormalizeRemote(t *testing.T) {
	want := "github.com/migueel15/dory"
	for _, url := range []string{
		"https://github.com/migueel15/dory.git",
		"https://GitHub.com/migueel15/dory/",
		"git@github.com:migueel15/dory.git",
		"ssh://git@github.com:22/migueel15/dory.git",
		"https://user@github.com/migueel15/dory",
	} {
		if got := normalizeRemote(url); got != want {
			t.Errorf("normalizeRemote(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestHasTags(t *testing.T) {
	if !hasTags([]string{"a", "b"}, []string{"b"}) || hasTags([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("hasTags must require every wanted tag")
	}
}

func TestSummaryView(t *testing.T) {
	if s, err := checkView(""); !s || err != nil {
		t.Fatal("empty view must default to summary")
	}
	if s, err := checkView("full"); s || err != nil {
		t.Fatal("full view must not summarize")
	}
	if _, err := checkView("other"); err == nil {
		t.Fatal("unknown view must fail")
	}
	m := summarizeMemory(service.Memory{Content: strings.Repeat("ñ", 250)})
	if len([]rune(m.Content)) != 200 {
		t.Fatalf("memory summary must keep 200 characters, got %d", len([]rune(m.Content)))
	}
}
