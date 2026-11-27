package runtime

import "testing"

func TestRememberCatalogDriverFallback(t *testing.T) {
	m := &Manager{}

	liveClaude := []Model{
		{Slug: "sonnet", Name: "Sonnet 5.5", ResolvedModel: "claude-sonnet-5-5"},
		{Slug: "opus", Name: "Opus 5.5", ResolvedModel: "claude-opus-5-5"},
	}
	fallbackClaude := []Model{{Slug: "sonnet", Name: "Sonnet"}}

	// Primary account reads live catalog.
	got, ok := m.rememberCatalog("claude", Status{Driver: "claude", Catalog: true, Models: liveClaude})
	if !ok || len(got) != len(liveClaude) {
		t.Fatalf("primary live catalog = (%v, %v), want (%v, true)", got, ok, liveClaude)
	}

	// Secondary account without its own catalog inherits the driver's catalog.
	got, ok = m.rememberCatalog("claude:work", Status{Driver: "claude", Catalog: false, Models: fallbackClaude})
	if !ok || len(got) != len(liveClaude) || got[0].ResolvedModel != "claude-sonnet-5-5" {
		t.Fatalf("secondary fallback = (%v, %v), want driver catalog", got, ok)
	}

	// Secondary account reads live catalog first; primary inherits it when its own read fails.
	liveCodex := []Model{{Slug: "gpt-6.2-sol", Name: "GPT-6.2-Sol"}}
	fallbackCodex := []Model{{Slug: "gpt-5.5", Name: "GPT-5.5"}}
	_, _ = m.rememberCatalog("codex:work", Status{Driver: "codex", Catalog: true, Models: liveCodex})
	got, ok = m.rememberCatalog("codex", Status{Driver: "codex", Catalog: false, Models: fallbackCodex})
	if !ok || len(got) != 1 || got[0].Slug != "gpt-6.2-sol" {
		t.Fatalf("primary codex fallback = (%v, %v), want driver catalog", got, ok)
	}

	// Driver with no saved catalog keeps hardcoded fallback and reports Catalog=false.
	fallbackOther := []Model{{Slug: "default", Name: "Default"}}
	got, ok = m.rememberCatalog("other", Status{Driver: "other", Catalog: false, Models: fallbackOther})
	if ok || len(got) != 1 || got[0].Slug != "default" {
		t.Fatalf("no-catalog fallback = (%v, %v), want (%v, false)", got, ok, fallbackOther)
	}
}
