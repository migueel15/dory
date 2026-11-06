package migrations

import (
	"io/fs"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestUniqueVersions(t *testing.T) {
	files, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]string{}
	for _, f := range files {
		v, err := goose.NumericComponent(f)
		if err != nil {
			t.Errorf("%s: no se puede obtener la versión: %v", f, err)
			continue
		}
		if prev, ok := seen[v]; ok {
			t.Errorf("versión %d duplicada: %s y %s", v, prev, f)
			continue
		}
		seen[v] = f
	}
}
