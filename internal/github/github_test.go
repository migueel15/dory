package github

import "testing"

func TestParseRemote(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/migueel15/dory.git": "migueel15/dory",
		"git@github.com:migueel15/dory.git":     "migueel15/dory",
		"https://github.com/a/b.c":              "a/b.c",
		"https://gitlab.com/a/b":                "",
	} {
		got, ok := ParseRemote(in)
		if got != want || ok != (want != "") {
			t.Errorf("%s: got %q %v", in, got, ok)
		}
	}
}
