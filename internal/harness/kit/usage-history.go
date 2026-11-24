package kit

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/harness"
)

type usageFile struct {
	mod    time.Time
	size   int64
	events []harness.UsageEvent
}

type UsageCache struct {
	sync.Mutex
	files map[string]usageFile
}

// usageEvents reads every JSONL under root, reusing the events of the files
// that did not change since the last read.
func (cache *UsageCache) Events(root string, read func(path string) []harness.UsageEvent) []harness.UsageEvent {
	if cache == nil {
		cache = &UsageCache{files: map[string]usageFile{}}
	}
	var out []harness.UsageEvent
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		cache.Lock()
		f, ok := cache.files[path]
		cache.Unlock()
		if !ok || !f.mod.Equal(info.ModTime()) || f.size != info.Size() {
			f = usageFile{mod: info.ModTime(), size: info.Size(), events: read(path)}
			cache.Lock()
			if cache.files == nil {
				cache.files = make(map[string]usageFile)
			}
			cache.files[path] = f
			cache.Unlock()
		}
		out = append(out, f.events...)
		return nil
	})
	return out
}

// eachLine calls fn with every line of a file; long lines are kept whole.
func EachLine(path string, fn func(line []byte)) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		fn(sc.Bytes())
	}
}

func AddUsageTokens(t *harness.UsageTokens, o harness.UsageTokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheWrite += o.CacheWrite
	t.CacheRead += o.CacheRead
	t.CacheWrite1h += o.CacheWrite1h
}

// UsageCost is the API list price of some tokens, by type, in dollars.
