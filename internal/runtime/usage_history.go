package runtime

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// usageEvent is the token usage of one model response read from the local
// logs of a harness.
type usageEvent struct {
	Time     time.Time
	Provider string
	Session  string
	Model    string
	Key      string // dedup key; empty when the format needs none
	Tokens   UsageTokens
}

// UsageTokens splits the tokens of a response by how they are billed. Input
// excludes the cached part.
type UsageTokens struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheWrite int64 `json:"cache_write"`
	CacheRead  int64 `json:"cache_read"`
	// CacheWrite1h is the part of CacheWrite kept for an hour, which costs
	// more than the 5 minute writes (Claude Code writes its cache this way).
	CacheWrite1h int64 `json:"cache_write_1h"`
}

func (t *UsageTokens) add(o UsageTokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheWrite += o.CacheWrite
	t.CacheRead += o.CacheRead
	t.CacheWrite1h += o.CacheWrite1h
}

// UsageCost is the API list price of some tokens, by type, in dollars.
type UsageCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheWrite float64 `json:"cache_write"`
	CacheRead  float64 `json:"cache_read"`
	// CacheSaving is what the cached input would cost at the input price
	// minus what it costs as cache reads.
	CacheSaving float64 `json:"cache_saving"`
}

// UsageRow sums the usage of one model of one provider in one bucket.
type UsageRow struct {
	Bucket   time.Time   `json:"bucket"`
	Provider string      `json:"provider"`
	Model    string      `json:"model"`
	Sessions int         `json:"sessions"`
	Tokens   UsageTokens `json:"tokens"`
	Cost     UsageCost   `json:"cost"`
	Priced   bool        `json:"priced"`
}

// UsageReport is the usage history of this machine since From.
type UsageReport struct {
	From   time.Time  `json:"from"`
	Bucket string     `json:"bucket"` // hour or day
	Rows   []UsageRow `json:"rows"`
	// Sessions counts the distinct sessions of each provider in the range;
	// adding the rows would count a session once per bucket and model.
	Sessions map[string]int `json:"sessions"`
}

// UsageHistory reads the Claude Code and Codex logs under home and sums the
// usage since from, by hour or by day in loc.
func UsageHistory(home string, from time.Time, hourly bool, loc *time.Location) UsageReport {
	events := usageEvents(filepath.Join(home, ".claude", "projects"), "claude", readClaudeUsage)
	events = append(events, usageEvents(filepath.Join(home, ".codex", "sessions"), "codex", readCodexUsage)...)
	return aggregateUsage(events, from, hourly, loc)
}

func aggregateUsage(events []usageEvent, from time.Time, hourly bool, loc *time.Location) UsageReport {
	type key struct {
		bucket          time.Time
		provider, model string
	}
	rows := map[key]*UsageRow{}
	rowSessions := map[key]map[string]bool{}
	sessions := map[string]map[string]bool{}
	seen := map[string]bool{}
	for _, e := range events {
		if e.Time.Before(from) {
			continue
		}
		if e.Key != "" {
			if seen[e.Key] {
				continue
			}
			seen[e.Key] = true
		}
		t := e.Time.In(loc)
		b := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		if hourly {
			b = t.Truncate(time.Hour)
		}
		k := key{b, e.Provider, e.Model}
		r := rows[k]
		if r == nil {
			_, ok := priceFor(e.Model)
			r = &UsageRow{Bucket: b, Provider: e.Provider, Model: e.Model, Priced: ok}
			rows[k], rowSessions[k] = r, map[string]bool{}
		}
		r.Tokens.add(e.Tokens)
		rowSessions[k][e.Session] = true
		if sessions[e.Provider] == nil {
			sessions[e.Provider] = map[string]bool{}
		}
		sessions[e.Provider][e.Session] = true
	}
	out := UsageReport{From: from, Bucket: "day", Rows: []UsageRow{}, Sessions: map[string]int{}}
	if hourly {
		out.Bucket = "hour"
	}
	for k, r := range rows {
		r.Sessions = len(rowSessions[k])
		if p, ok := priceFor(r.Model); ok {
			r.Cost = UsageCost{
				Input:       float64(r.Tokens.Input) * p.Input / 1e6,
				Output:      float64(r.Tokens.Output) * p.Output / 1e6,
				CacheWrite:  (float64(r.Tokens.CacheWrite-r.Tokens.CacheWrite1h)*p.CacheWrite + float64(r.Tokens.CacheWrite1h)*p.cacheWrite1h()) / 1e6,
				CacheRead:   float64(r.Tokens.CacheRead) * p.CacheRead / 1e6,
				CacheSaving: float64(r.Tokens.CacheRead) * (p.Input - p.CacheRead) / 1e6,
			}
		}
		out.Rows = append(out.Rows, *r)
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		a, b := out.Rows[i], out.Rows[j]
		if !a.Bucket.Equal(b.Bucket) {
			return a.Bucket.Before(b.Bucket)
		}
		return a.Provider+a.Model < b.Provider+b.Model
	})
	for p, s := range sessions {
		out.Sessions[p] = len(s)
	}
	return out
}

// usageFile caches the events of one log while its size and mtime hold.
type usageFile struct {
	mod    time.Time
	size   int64
	events []usageEvent
}

var usageCache = struct {
	sync.Mutex
	files map[string]usageFile
}{files: map[string]usageFile{}}

// usageEvents reads every JSONL under root, reusing the events of the files
// that did not change since the last read.
func usageEvents(root, provider string, read func(path string) []usageEvent) []usageEvent {
	var out []usageEvent
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		usageCache.Lock()
		f, ok := usageCache.files[path]
		usageCache.Unlock()
		if !ok || !f.mod.Equal(info.ModTime()) || f.size != info.Size() {
			f = usageFile{mod: info.ModTime(), size: info.Size(), events: read(path)}
			usageCache.Lock()
			usageCache.files[path] = f
			usageCache.Unlock()
		}
		out = append(out, f.events...)
		return nil
	})
	return out
}

// eachLine calls fn with every line of a file; long lines are kept whole.
func eachLine(path string, fn func(line []byte)) {
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

// readClaudeUsage reads the assistant messages of a Claude Code session. One
// response is written once per content block with the same usage, and resumed
// sessions copy earlier ones, so each carries a message.id:requestId key.
func readClaudeUsage(path string) []usageEvent {
	var out []usageEvent
	session := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	eachLine(path, func(line []byte) {
		var l struct {
			Type      string    `json:"type"`
			Timestamp time.Time `json:"timestamp"`
			SessionID string    `json:"sessionId"`
			RequestID string    `json:"requestId"`
			Message   struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Usage *struct {
					Input      int64 `json:"input_tokens"`
					Output     int64 `json:"output_tokens"`
					CacheWrite int64 `json:"cache_creation_input_tokens"`
					CacheRead  int64 `json:"cache_read_input_tokens"`
					Creation   struct {
						Hour int64 `json:"ephemeral_1h_input_tokens"`
					} `json:"cache_creation"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &l) != nil || l.Type != "assistant" || l.Message.Usage == nil || l.Message.Model == "" || l.Message.Model == "<synthetic>" {
			return
		}
		s := l.SessionID
		if s == "" {
			s = session
		}
		u := l.Message.Usage
		key := ""
		if l.Message.ID != "" {
			key = "claude:" + l.Message.ID + ":" + l.RequestID
		}
		out = append(out, usageEvent{Time: l.Timestamp, Provider: "claude", Session: s, Model: l.Message.Model, Key: key,
			Tokens: UsageTokens{Input: u.Input, Output: u.Output, CacheWrite: u.CacheWrite, CacheRead: u.CacheRead, CacheWrite1h: min(u.Creation.Hour, u.CacheWrite)}})
	})
	return out
}

// readCodexUsage reads the token_count events of a Codex session. Each one
// carries the usage of the last response; Codex repeats an event when nothing
// changed, which shows as the same running total.
func readCodexUsage(path string) []usageEvent {
	var out []usageEvent
	session := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	model := ""
	var last json.RawMessage
	eachLine(path, func(line []byte) {
		var l struct {
			Timestamp time.Time `json:"timestamp"`
			Type      string    `json:"type"`
			Payload   struct {
				Type  string `json:"type"`
				ID    string `json:"id"`
				Model string `json:"model"`
				Info  *struct {
					Total json.RawMessage `json:"total_token_usage"`
					Last  struct {
						Input      int64 `json:"input_tokens"`
						Cached     int64 `json:"cached_input_tokens"`
						CacheWrite int64 `json:"cache_write_input_tokens"`
						Output     int64 `json:"output_tokens"`
					} `json:"last_token_usage"`
				} `json:"info"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &l) != nil {
			return
		}
		switch {
		case l.Type == "session_meta" && l.Payload.ID != "":
			session = l.Payload.ID
		case l.Type == "turn_context" && l.Payload.Model != "":
			model = l.Payload.Model
		case l.Type == "event_msg" && l.Payload.Type == "token_count" && l.Payload.Info != nil:
			if string(l.Payload.Info.Total) == string(last) {
				return
			}
			last = l.Payload.Info.Total
			u := l.Payload.Info.Last
			m := model
			if m == "" {
				m = "codex"
			}
			out = append(out, usageEvent{Time: l.Timestamp, Provider: "codex", Session: session, Model: m,
				Tokens: UsageTokens{Input: max(u.Input-u.Cached, 0), Output: u.Output, CacheWrite: u.CacheWrite, CacheRead: u.Cached}})
		}
	})
	return out
}
