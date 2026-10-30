package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/migueel15/dory/internal/config"
)

func TestSameOriginBehindProxy(t *testing.T) {
	cfg := config.Config{PublicOrigin: "https://dory.dorlab.net"}
	cases := []struct {
		name   string
		origin string
		want   bool
	}{
		{"public origin over https", "https://dory.dorlab.net", true},
		{"other host", "https://evil.example", false},
		{"plain http of the same host", "http://dory.dorlab.net", false},
		{"no origin header", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/auth/login", nil)
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if got := sameOrigin(r, cfg); got != c.want {
				t.Fatalf("sameOrigin(%q) = %v, want %v", c.origin, got, c.want)
			}
		})
	}
}

func TestSameOriginWithoutPublicOrigin(t *testing.T) {
	r := httptest.NewRequest("POST", "http://localhost:8080/api/auth/login", nil)
	r.Header.Set("Origin", "http://localhost:8080")
	if !sameOrigin(r, config.Config{}) {
		t.Fatal("expected same-origin request to pass without public_origin")
	}
	r.Header.Set("Origin", "https://localhost:8080")
	if sameOrigin(r, config.Config{}) {
		t.Fatal("expected scheme mismatch to fail without public_origin")
	}
}
