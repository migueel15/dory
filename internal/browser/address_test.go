package browser

import "testing"

func TestAddress(t *testing.T) {
	m := &Manager{SearchEngine: func() string { return "duckduckgo" }}
	for in, want := range map[string]string{
		"github.com":            "https://github.com",
		"localhost:3000/a":      "http://localhost:3000/a",
		"192.168.1.2:8080":      "https://192.168.1.2:8080",
		"https://example.com/x": "https://example.com/x",
		"about:blank":           "about:blank",
		"go generics":           "https://duckduckgo.com/?q=go+generics",
		"dory":                  "https://duckduckgo.com/?q=dory",
	} {
		if got := m.address(in); got != want {
			t.Errorf("address(%q) = %q, want %q", in, got, want)
		}
	}
	if got := (&Manager{}).address("x y"); got != "https://www.google.com/search?q=x+y" {
		t.Errorf("default engine: %q", got)
	}
}
