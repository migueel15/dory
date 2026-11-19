package runtime

import (
	"math"
	"testing"
	"time"
)

func TestUsageHistory(t *testing.T) {
	events := append(readClaudeUsage("testdata/usage/claude.jsonl"), readCodexUsage("testdata/usage/codex.jsonl")...)
	r := aggregateUsage(events, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), false, time.UTC)
	if len(r.Rows) != 3 {
		t.Fatalf("rows = %+v", r.Rows)
	}
	opus, codex, mystery := r.Rows[0], r.Rows[1], r.Rows[2]
	if opus.Model != "claude-opus-5-5" || opus.Tokens != (UsageTokens{Input: 1000, Output: 100, CacheWrite: 2000, CacheRead: 10000}) || opus.Sessions != 1 {
		t.Fatalf("opus deduplicated = %+v", opus)
	}
	// 1000×4 + 100×20 + 2000×5 + 10000×0.2 per million
	if got := opus.Cost.Input + opus.Cost.Output + opus.Cost.CacheWrite + opus.Cost.CacheRead; math.Abs(got-0.018) > 1e-9 {
		t.Fatalf("opus cost = %v", got)
	}
	if math.Abs(opus.Cost.CacheSaving-0.038) > 1e-9 {
		t.Fatalf("cache saving = %v", opus.Cost.CacheSaving)
	}
	if codex.Model != "gpt-x" || codex.Tokens != (UsageTokens{Input: 100, Output: 30, CacheRead: 200}) || codex.Priced {
		t.Fatalf("codex = %+v", codex)
	}
	if mystery.Priced || mystery.Cost != (UsageCost{}) || !mystery.Bucket.Equal(time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("mystery = %+v", mystery)
	}
	if r.Sessions["claude"] != 2 || r.Sessions["codex"] != 1 {
		t.Fatalf("sessions = %v", r.Sessions)
	}
	hourly := aggregateUsage(events, time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), true, time.UTC)
	if len(hourly.Rows) != 1 || hourly.Rows[0].Bucket.Hour() != 9 || hourly.Bucket != "hour" {
		t.Fatalf("hourly = %+v", hourly)
	}
}

func TestHourCacheWritesCostMore(t *testing.T) {
	at := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	events := []usageEvent{{Time: at, Provider: "claude", Session: "s", Model: "claude-opus-5-5",
		Tokens: UsageTokens{CacheWrite: 3_000_000, CacheWrite1h: 2_000_000}}}
	r := aggregateUsage(events, at.Add(-time.Hour), false, time.UTC)
	// 1M at the 5 minute rate (5) and 2M at the hour rate (2×4).
	if got := r.Rows[0].Cost.CacheWrite; math.Abs(got-21) > 1e-9 {
		t.Fatalf("cache write cost = %v", got)
	}
}
