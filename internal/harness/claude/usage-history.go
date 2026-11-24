package claude

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

func (c Claude) UsageHistory(p harness.Paths, _ time.Time) []harness.UsageEvent {
	return c.history.Events(filepath.Join(kit.AccountDir(p, c), "projects"), readClaudeUsage)
}

// readClaudeUsage reads the assistant messages of a Claude Code session. One
// response is written once per content block with the same usage, and resumed
// sessions copy earlier ones, so each carries a message.id:requestId key.
func readClaudeUsage(path string) []harness.UsageEvent {
	var out []harness.UsageEvent
	session := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	kit.EachLine(path, func(line []byte) {
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
		out = append(out, harness.UsageEvent{Time: l.Timestamp, Provider: (Claude{}).ID(), Session: s, Model: l.Message.Model, Key: key,
			Tokens: harness.UsageTokens{Input: u.Input, Output: u.Output, CacheWrite: u.CacheWrite, CacheRead: u.CacheRead, CacheWrite1h: min(u.Creation.Hour, u.CacheWrite)}})
	})
	return out
}
