package localdb

import "testing"

func TestDecodeOldTool(t *testing.T) {
	if c := decodeOldTool("Bash", "go test ./..."); c.Kind != "command" || c.Command != "go test ./..." {
		t.Fatalf("bash: %+v", c)
	}
	if c := decodeOldTool("Editar ficheros", "update a.go\nadd b.go"); len(c.Files) != 2 || c.Files[1].Change != "add" {
		t.Fatalf("codex files: %+v", c)
	}
	if c := decodeOldTool("Read", "/a/b.go"); c.Kind != "read" || c.Path != "/a/b.go" {
		t.Fatalf("read: %+v", c)
	}
}
