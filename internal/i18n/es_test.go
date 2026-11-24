package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every literal key the server asks for must have a text: a missing one would
// show the bare key to the user.
func TestEveryUsedKeyHasText(t *testing.T) {
	use := regexp.MustCompile(`i18n\.Textf?\("([^"]+)"`)
	var missing []string
	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range use.FindAllStringSubmatch(string(src), -1) {
				if _, ok := es[m[1]]; !ok {
					missing = append(missing, path+": "+m[1])
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("keys without text:\n%s", strings.Join(missing, "\n"))
	}
}
