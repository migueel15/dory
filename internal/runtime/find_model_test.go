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
