package antigravity

import (
	"database/sql"
	"encoding/binary"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/harness"
	_ "modernc.org/sqlite"
)

func dataRoots(goos, home, appData, localAppData, xdgConfig string) []string {
	if home == "" {
		return nil
	}
	roots := []string{
		filepath.Join(home, ".gemini", "antigravity-cli"),
		filepath.Join(home, ".gemini", "antigravity-acp"),
		filepath.Join(home, ".gemini", "antigravity"),
		filepath.Join(home, ".gemini", "antigravity-ide"),
	}
	switch goos {
	case "windows":
		for _, dir := range []string{appData, localAppData} {
			if dir != "" {
				roots = append(roots, filepath.Join(dir, "antigravity"))
			}
		}
	case "darwin":
		roots = append(roots, filepath.Join(home, "Library", "Application Support", "antigravity"))
	default:
		if filepath.IsAbs(xdgConfig) {
			roots = append(roots, filepath.Join(xdgConfig, "antigravity"))
		}
	}
	return append(roots, filepath.Join(home, ".config", "antigravity"))
}

func usageEvents(home string, from time.Time) []harness.UsageEvent {
	roots := dataRoots(runtime.GOOS, home, os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_CONFIG_HOME"))
	var events []harness.UsageEvent
	seen := map[string]bool{}
	for _, root := range roots {
		dir := filepath.Join(root, "conversations")
		if canonical, err := filepath.EvalSymlinks(dir); err == nil {
			dir = canonical
		}
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() || filepath.Ext(path) != ".db" || seen[path] {
				return nil
			}
			seen[path] = true
			st, err := d.Info()
			if err != nil {
				return nil
			}
			mod := st.ModTime()
			if wal, err := os.Stat(path + "-wal"); err == nil && wal.ModTime().After(mod) {
				mod = wal.ModTime()
			}
			if !mod.Before(from) {
				events = append(events, readUsage(path, mod)...)
			}
			return nil
		})
	}
	return events
}

// sessionUsage reads only the current conversation. The ACP stream
// supplies the content, while the CLI log supplies the model counters.
func sessionUsage(home, session string, start, end time.Time) []harness.StepUsage {
	if session == "" || filepath.Base(session) != session {
		return nil
	}
	roots := dataRoots(runtime.GOOS, home, os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_CONFIG_HOME"))
	for _, root := range roots {
		filename := filepath.Join(root, "conversations", session+".db")
		if _, err := os.Stat(filename); err != nil {
			continue
		}
		events := readUsage(filename, time.Time{})
		sort.SliceStable(events, func(i, j int) bool { return events[i].Time.Before(events[j].Time) })
		seen := map[string]bool{}
		var out []harness.StepUsage
		for _, e := range events {
			if e.Time.Before(start) || e.Time.After(end) || (e.Key != "" && seen[e.Key]) {
				continue
			}
			seen[e.Key] = true
			u := e.Tokens
			out = append(out, harness.StepUsage{Model: e.Model, Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Reasoning: e.Reasoning})
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func readUsage(path string, mod time.Time) []harness.UsageEvent {
	slash := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	uri := url.URL{Scheme: "file", Path: slash, RawQuery: "mode=ro&_pragma=busy_timeout(100)"}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil
	}
	defer db.Close()
	session := strings.TrimSuffix(filepath.Base(path), ".db")
	var events []harness.UsageEvent
	generations := map[int64]bool{}
	for _, source := range []struct {
		query string
		step  bool
	}{
		{"SELECT idx,data FROM gen_metadata ORDER BY idx", false},
		{"SELECT idx,metadata FROM steps WHERE metadata IS NOT NULL ORDER BY idx", true},
	} {
		rows, err := db.Query(source.query)
		if err != nil {
			continue
		}
		for rows.Next() {
			var idx int64
			var blob []byte
			if rows.Scan(&idx, &blob) != nil || (source.step && generations[idx]) {
				continue
			}
			e := agyUsageMetadata(blob, source.step, session, mod)
			if len(e) > 0 {
				generations[idx] = true
				events = append(events, e...)
			}
		}
		rows.Close()
	}
	return events
}

// The local SQLite store keeps generation metadata and token counts as
// protobuf blobs. Decode only the observed wire fields and skip unknown ones.
type agyProto map[uint64][]agyField
type agyField struct {
	n    uint64
	data []byte
}

func parseAgyProto(raw []byte) agyProto {
	fields := agyProto{}
	for len(raw) > 0 {
		tag, n := binary.Uvarint(raw)
		if n <= 0 || tag>>3 == 0 {
			return nil
		}
		raw = raw[n:]
		var f agyField
		switch tag & 7 {
		case 0, 2:
			value, n := binary.Uvarint(raw)
			if n <= 0 {
				return nil
			}
			raw = raw[n:]
			if tag&7 == 2 {
				if value > uint64(len(raw)) {
					return nil
				}
				f.data, raw = raw[:value], raw[value:]
			} else {
				f.n = value
			}
		case 1, 5:
			size := 8
			if tag&7 == 5 {
				size = 4
			}
			if len(raw) < size {
				return nil
			}
			raw = raw[size:]
			continue
		default:
			return nil
		}
		fields[tag>>3] = append(fields[tag>>3], f)
	}
	return fields
}

func (p agyProto) first(tag uint64) agyField {
	if values := p[tag]; len(values) > 0 {
		return values[0]
	}
	return agyField{}
}

func agyUsageMetadata(raw []byte, step bool, session string, fallback time.Time) []harness.UsageEvent {
	data := parseAgyProto(raw)
	if !step {
		data = parseAgyProto(data.first(1).data)
	}
	model, usageTag, retryTag := data, uint64(4), uint64(17)
	nameTag, idTag := uint64(19), uint64(3)
	stamp := parseAgyProto(parseAgyProto(data.first(9).data).first(4).data)
	if step {
		model = parseAgyProto(data.first(24).data)
		nameTag, idTag, usageTag, retryTag = 12, 1, 9, 28
		stamp = parseAgyProto(data.first(8).data)
		if stamp.first(1).n == 0 {
			stamp = parseAgyProto(data.first(1).data)
		}
	}
	at := fallback
	if sec := stamp.first(1).n; sec > 0 && sec < 253402300800 {
		at = time.Unix(int64(sec), int64(stamp.first(2).n))
	}
	name := strings.ToLower(strings.Join(strings.Fields(string(model.first(nameTag).data)), "-"))
	usages := [][]byte{data.first(usageTag).data}
	for _, field := range data[retryTag] {
		usages = append(usages, parseAgyProto(field.data).first(2).data)
	}
	var events []harness.UsageEvent
	for _, raw := range usages {
		p := parseAgyProto(raw)
		n := func(tag uint64) int64 {
			value := p.first(tag).n
			if value > 1<<40 {
				return 0
			}
			return int64(value)
		}
		tokens := harness.UsageTokens{Input: n(2), Output: max(n(3), n(9)+n(10)), CacheWrite: n(4), CacheRead: n(5)}
		if tokens == (harness.UsageTokens{}) {
			continue
		}
		modelName := name
		if id := n(1); modelName == "" || (id != 0 && uint64(id) != model.first(idTag).n) {
			modelName = "antigravity"
			if id != 0 {
				modelName += "-" + strconv.Itoa(int(id))
			}
		}
		var key string
		for _, tag := range []uint64{11, 12, 7} {
			if id := string(p.first(tag).data); id != "" {
				key = "antigravity:" + strconv.Itoa(int(tag)) + ":" + id
				break
			}
		}
		events = append(events, harness.UsageEvent{Time: at, Provider: ID, Session: session, Model: modelName, Tokens: tokens, Key: key, Reasoning: min(tokens.Output, n(10))})
	}
	return events
}
