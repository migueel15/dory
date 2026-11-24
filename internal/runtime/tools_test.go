package runtime

import "testing"

func TestLegacyTool(t *testing.T) {
	if c := legacyTool("Bash", "go test ./..."); c.Kind != "command" || c.Command != "go test ./..." {
		t.Fatalf("bash: %+v", c)
	}
	if c := legacyTool("Editar ficheros", "update a.go\nadd b.go"); len(c.Files) != 2 || c.Files[1].Change != "add" {
		t.Fatalf("codex files: %+v", c)
	}
	if c := legacyTool("Read", "/a/b.go"); c.Kind != "read" || c.Path != "/a/b.go" {
		t.Fatalf("read: %+v", c)
	}
}
