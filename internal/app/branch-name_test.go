package app

import (
	"testing"
)

func TestCleanBranch(t *testing.T) {
	for in, want := range map[string]string{
		"Feat/Add Login Page\nextra": "feat/add-login-page",
		"`fix/bug..x`":               "fix/bug.x",
		"":                           "",
		"///":                        "",
	} {
		if got := cleanBranch(in); got != want {
			t.Errorf("cleanBranch(%q) = %q, want %q", in, got, want)
		}
	}
}
