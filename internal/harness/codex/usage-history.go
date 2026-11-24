package codex

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

func (c Codex) UsageHistory(p harness.Paths, _ time.Time) []harness.UsageEvent {
	return c.history.Events(filepath.Join(kit.AccountDir(p, c), "sessions"), readCodexUsage)
}

// readCodexUsage reads the token_count events of a Codex session. Each one
// carries the usage of the last response; Codex repeats an event when nothing
// changed, which shows as the same running total.
func readCodexUsage(path string) []harness.UsageEvent {
	var out []harness.UsageEvent
	session := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	model := ""
	var last json.RawMessage
	kit.EachLine(path, func(line []byte) {
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
				m = (Codex{}).ID()
			}
			out = append(out, harness.UsageEvent{Time: l.Timestamp, Provider: (Codex{}).ID(), Session: session, Model: m,
				Tokens: harness.UsageTokens{Input: max(u.Input-u.Cached-u.CacheWrite, 0), Output: u.Output, CacheWrite: u.CacheWrite, CacheRead: u.Cached}})
		}
	})
	return out
}
