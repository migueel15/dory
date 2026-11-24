package kit

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/harness"
)

// ExternalLimit bounds how many recent sessions of each harness are listed.
const ExternalLimit = 150

type SummaryCache struct {
	mu   sync.Mutex
	seen map[summaryKey]cachedSummary
}

type summaryKey struct{ provider, path string }

type cachedSummary struct {
	mod time.Time
	ext harness.External
}

type sessionParser func([]byte, *harness.External)
type recordParser func([]byte, func(harness.Message), []harness.Message, map[string]int)

func (c *SummaryCache) List(provider, pattern string, parse sessionParser) []harness.External {
	if c == nil {
		c = &SummaryCache{seen: map[summaryKey]cachedSummary{}}
	}
	var out []harness.External
	files, _ := filepath.Glob(pattern)
	type file struct {
		path string
		mod  time.Time
	}
	var list []file
	for _, f := range files {
		if st, err := os.Stat(f); err == nil {
			list = append(list, file{f, st.ModTime()})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod.After(list[j].mod) })
	if len(list) > ExternalLimit {
		list = list[:ExternalLimit]
	}
	for _, f := range list {
		if e, ok := c.summarize(provider, f.path, f.mod, parse); ok {
			out = append(out, e)
		}
	}
	return out
}

// summarize reads the title, folder and branch of a session file, cached by
// modification time because files can be large.
func (cache *SummaryCache) summarize(provider, path string, mod time.Time, parse sessionParser) (harness.External, bool) {
	cache.mu.Lock()
	c, hit := cache.seen[summaryKey{provider, path}]
	cache.mu.Unlock()
	if hit && c.mod.Equal(mod) {
		return c.ext, c.ext.ID != ""
	}
	e := harness.External{Provider: provider, Path: path, UpdatedAt: mod.UTC().Format(time.RFC3339)}
	f, err := os.Open(path)
	if err != nil {
		return e, false
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64<<10), 64<<20)
	for scan.Scan() {
		parse(scan.Bytes(), &e)
	}
	if e.Title == "" {
		e.ID = ""
	}
	cache.mu.Lock()
	if cache.seen == nil {
		cache.seen = make(map[summaryKey]cachedSummary)
	}
	cache.seen[summaryKey{provider, path}] = cachedSummary{mod, e}
	cache.mu.Unlock()
	return e, e.ID != ""
}

func ReadExternal(e harness.External, parse recordParser) ([]harness.Message, error) {
	f, err := os.Open(e.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64<<10), 64<<20)
	var out []harness.Message
	index := map[string]int{}
	emit := func(m harness.Message) {
		if i, ok := index[m.ID]; ok {
			out[i] = m
			return
		}
		index[m.ID] = len(out)
		out = append(out, m)
	}
	for scan.Scan() {
		parse(scan.Bytes(), emit, out, index)
	}
	return out, scan.Err()
}

func FirstMatch(pattern string) string {
	files, _ := filepath.Glob(pattern)
	if len(files) == 0 {
		return ""
	}
	return files[0]
}
func DoryMessage(text string) bool {
	return strings.HasPrefix(text, "<dory-context ") || strings.HasPrefix(text, "<dory-plan ")
}
