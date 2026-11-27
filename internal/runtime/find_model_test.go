package runtime

import "testing"

func TestFindModelByVersionedName(t *testing.T) {
	models := []Model{{Slug: "sonnet", Name: "Sonnet 5.5", ResolvedModel: "claude-sonnet-5-5"}, {Slug: "claude-sonnet-5", Name: "Sonnet 5", ResolvedModel: "claude-sonnet-5"}}
	for _, in := range []string{"sonnet", "Sonnet 5.5", "claude-sonnet-5-5", "sonnet-5.5"} {
		if m, ok := findModel(models, in); !ok || m.Slug != "sonnet" {
			t.Errorf("findModel(%q) = %q, %v", in, m.Slug, ok)
		}
	}
	if m, ok := findModel(models, "claude-sonnet-5"); !ok || m.Slug != "claude-sonnet-5" {
		t.Errorf("claude-sonnet-5 = %q, %v", m.Slug, ok)
	}
}

func TestResolveModelWithoutCatalog(t *testing.T) {
	claudeFallback := Status{
		Driver:  "claude",
		Catalog: false,
		Models:  []Model{{Slug: "opus", Name: "Opus"}, {Slug: "sonnet", Name: "Sonnet"}},
	}
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"Sonnet 5.5", "claude-sonnet-5-5"},
		{"claude-sonnet-5-5", "claude-sonnet-5-5"},
		{"Opus 5.6", "claude-opus-5-6"},
		{"sonnet", "sonnet"},
	} {
		m, ok := resolveModel(claudeFallback, tc.in)
		if !ok || m.Slug != tc.want {
			t.Errorf("claude resolveModel(%q) = (%q, %v), want (%q, true)", tc.in, m.Slug, ok, tc.want)
		}
	}
	if _, ok := resolveModel(claudeFallback, "   "); ok {
		t.Errorf("claude resolveModel(empty) = true, want false")
	}

	codexFallback := Status{
		Driver:  "codex",
		Catalog: false,
		Models:  []Model{{Slug: "gpt-5.5", Name: "GPT-5.5"}},
	}
	if m, ok := resolveModel(codexFallback, "GPT-6.2-Sol"); !ok || m.Slug != "gpt-6.2-sol" {
		t.Errorf("codex resolveModel(%q) = (%q, %v), want (%q, true)", "GPT-6.2-Sol", m.Slug, ok, "gpt-6.2-sol")
	}

	withCatalog := Status{
		Driver:  "claude",
		Catalog: true,
		Models:  []Model{{Slug: "sonnet", Name: "Sonnet 5.5", ResolvedModel: "claude-sonnet-5-5"}},
	}
	if _, ok := resolveModel(withCatalog, "Sonnet 9.9"); ok {
		t.Errorf("withCatalog resolveModel(Sonnet 9.9) = true, want false")
	}
}
