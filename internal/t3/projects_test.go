package t3

import "testing"

func TestRepoKey(t *testing.T) {
	want := "github.com/migueel15/dory"
	for _, in := range []string{
		"https://github.com/migueel15/dory",
		"https://github.com/migueel15/dory.git",
		"git@github.com:migueel15/dory.git",
		"ssh://git@github.com/migueel15/dory",
		"https://GitHub.com/Migueel15/Dory/",
	} {
		if got := repoKey(in); got != want {
			t.Errorf("repoKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugify(t *testing.T) {
	if got := slugify("Mi Proyecto_2"); got != "mi-proyecto-2" {
		t.Errorf("slugify = %q", got)
	}
}
