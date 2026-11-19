package dictation

import (
	"path/filepath"
	"testing"
)

func TestEntryPathStaysInsideDestination(t *testing.T) {
	dest := t.TempDir()
	for name, want := range map[string]string{
		"nemo/bin/nemo-speech":     filepath.Join(dest, "bin", "nemo-speech"),
		`nemo\bin\nemo-speech.exe`: filepath.Join(dest, "bin", "nemo-speech.exe"),
		"nemo/../../etc/passwd":    "",
		"nemo/":                    "",
		"/etc/passwd":              filepath.Join(dest, "etc", "passwd"),
	} {
		got, ok := entryPath(dest, name)
		if want == "" && ok || want != "" && got != want {
			t.Errorf("entryPath(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
}
