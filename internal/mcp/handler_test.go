package mcp

import "testing"

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
