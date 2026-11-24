package runtime

import (
	"sort"
	"time"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

type usageEvent = harness.UsageEvent
type UsageTokens = harness.UsageTokens

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

// UsageHistory aggregates local adapter histories.
func (m *Manager) UsageHistory(p Paths, from time.Time, hourly bool, loc *time.Location) UsageReport {
	var events []usageEvent
	for _, d := range m.registry.All() {
		info := d.Info()
		if d.History != nil && info.ID == info.Driver {
			events = append(events, d.History.UsageHistory(p, from)...)
		}
	}
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
		kit.AddUsageTokens(&r.Tokens, e.Tokens)
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
