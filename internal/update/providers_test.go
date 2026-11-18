package update

import "testing"

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"2.1.327", "2.1.326 (Claude Code)", true},
		{"0.169.0", "codex-cli 0.169.0", false},
		{"2.3.0", "opencode v2.10.0", false},
		{"2.10.0", "v2.3.0", true},
		{"", "1.0.0", false},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
