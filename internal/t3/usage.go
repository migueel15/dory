package t3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// usageInterval is slower than pollInterval: each read opens a WebSocket.
const (
	usageInterval = time.Minute
	usageTimeout  = 20 * time.Second
	usageDays     = 7
	// usageRefreshGap limits forced reads: each one makes T3 probe its providers.
	usageRefreshGap = 15 * time.Second
)

// Reasons why a part of the usage has no data. Details stay out of the API
// because transport errors may quote the WebSocket ticket.
const (
	UsageUnavailable  = "unavailable"
	UsageMissingScope = "missing_scope"
)

var errMissingScope = errors.New("t3: token lacks the required scope")

// UsageWindow is a plan limit exactly as T3 reports it.
type UsageWindow struct {
	Kind          string  `json:"kind"`
	Label         string  `json:"label"`
	UsedPercent   float64 `json:"used_percent"`
	ResetsAt      *string `json:"resets_at"`
	WindowMinutes *int    `json:"window_minutes"`
}

// ProviderLimits has no windows when the provider did not report any.
type ProviderLimits struct {
	Provider  string        `json:"provider"`
	Name      string        `json:"name"`
	CheckedAt *string       `json:"checked_at"`
	Windows   []UsageWindow `json:"windows"`
}

// ModelUsage adds up one week of a model. CostSource keeps what kind of number
// CostUSD is: providerReported, modelPriced (an estimate) or unpriced.
type ModelUsage struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	CachedTokens int64   `json:"cached_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	CostSource   string  `json:"cost_source"`
}

// InstanceUsage is the latest usage read of one instance. It is never stored.
type InstanceUsage struct {
	InstanceID    string           `json:"instance_id"`
	EnvironmentID string           `json:"environment_id"`
	FetchedAt     time.Time        `json:"fetched_at"`
	LimitsError   string           `json:"limits_error"`
	Providers     []ProviderLimits `json:"providers"`
	ModelsError   string           `json:"models_error"`
	Models        []ModelUsage     `json:"models"`
}

type wireConfig struct {
	Providers []struct {
		InstanceID  string `json:"instanceId"`
		DisplayName string `json:"displayName"`
		Enabled     bool   `json:"enabled"`
		UsageLimits *struct {
			CheckedAt string `json:"checkedAt"`
			Windows   []struct {
				Kind              string  `json:"kind"`
				Label             string  `json:"label"`
				UsedPercent       float64 `json:"usedPercent"`
				ResetsAt          *string `json:"resetsAt"`
				WindowDurationMin *int    `json:"windowDurationMins"`
			} `json:"windows"`
		} `json:"usageLimits"`
	} `json:"providers"`
}

type wireUsage struct {
	Buckets []struct {
		Day      string `json:"day"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Totals   struct {
			Uncached int64 `json:"uncachedInputTokens"`
			Cached   int64 `json:"cachedInputTokens"`
			Creation int64 `json:"cacheCreationTokens"`
			Output   int64 `json:"outputTokens"`
		} `json:"totals"`
		CostUSD         float64 `json:"costUsd"`
		CacheSavingsUSD float64 `json:"cacheSavingsUsd"`
		CostSource      string  `json:"costSource"`
		Sessions        int64   `json:"sessions"`
	} `json:"buckets"`
}

// MaxHistoryDays bounds a history read: T3 scans transcripts for every day asked.
const MaxHistoryDays = 90

// DayUsage is one day of one model as T3 buckets it, in UTC days.
type DayUsage struct {
	Day              string  `json:"day"`
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	UncachedTokens   int64   `json:"uncached_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	CacheSavingsUSD  float64 `json:"cache_savings_usd"`
	CostSource       string  `json:"cost_source"`
	Sessions         int64   `json:"sessions"`
}

// InstanceHistory is the daily consumption of one instance. Error is empty,
// unavailable or missing_scope. It is read on demand and never stored.
type InstanceHistory struct {
	InstanceID    string     `json:"instance_id"`
	EnvironmentID string     `json:"environment_id"`
	Error         string     `json:"error"`
	Days          []DayUsage `json:"days"`
}

// UsageHistory reads the daily usage buckets of the last days, today included.
func (c Client) UsageHistory(ctx context.Context, now time.Time, days int) ([]DayUsage, error) {
	ctx, cancel := context.WithTimeout(ctx, usageTimeout)
	defer cancel()
	ws, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer ws.CloseNow()
	ws.SetReadLimit(-1)
	var usage wireUsage
	day := now.UTC()
	if err := call(ctx, ws, "1", "server.getUsageSummary", map[string]any{
		"sinceDay": day.AddDate(0, 0, 1-days).Format(time.DateOnly),
		"untilDay": day.Format(time.DateOnly),
		"timeZone": "UTC",
	}, &usage); err != nil {
		return nil, err
	}
	out := make([]DayUsage, 0, len(usage.Buckets))
	for _, b := range usage.Buckets {
		out = append(out, DayUsage{Day: b.Day, Provider: b.Provider, Model: b.Model, UncachedTokens: b.Totals.Uncached, CachedTokens: b.Totals.Cached, CacheWriteTokens: b.Totals.Creation, OutputTokens: b.Totals.Output, CostUSD: b.CostUSD, CacheSavingsUSD: b.CacheSavingsUSD, CostSource: b.CostSource, Sessions: b.Sessions})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Day+out[i].Provider+out[i].Model < out[j].Day+out[j].Provider+out[j].Model
	})
	return out, nil
}

// Usage reads plan limits and consumption over one short-lived connection.
func (c Client) Usage(ctx context.Context, now time.Time) (InstanceUsage, error) {
	out := InstanceUsage{FetchedAt: now, Providers: []ProviderLimits{}, Models: []ModelUsage{}}
	ctx, cancel := context.WithTimeout(ctx, usageTimeout)
	defer cancel()
	ws, err := c.dial(ctx)
	if err != nil {
		return out, err
	}
	defer ws.CloseNow()
	ws.SetReadLimit(-1)

	var cfg wireConfig
	if err := call(ctx, ws, "1", "server.getConfig", map[string]any{}, &cfg); err != nil {
		return out, err
	}
	out.Providers = limits(cfg)

	var usage wireUsage
	day := now.UTC()
	err = call(ctx, ws, "2", "server.getUsageSummary", map[string]any{
		"sinceDay": day.AddDate(0, 0, 1-usageDays).Format(time.DateOnly),
		"untilDay": day.Format(time.DateOnly),
		"timeZone": "UTC",
	}, &usage)
	switch {
	case err == nil:
		out.Models = models(usage)
	case errors.Is(err, errMissingScope):
		out.ModelsError = UsageMissingScope
	default:
		out.ModelsError = UsageUnavailable
	}
	return out, nil
}

// RefreshProviders makes T3 probe its providers again, because server.getConfig
// only returns the limits of the last probe.
func (c Client) RefreshProviders(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, usageTimeout)
	defer cancel()
	ws, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer ws.CloseNow()
	ws.SetReadLimit(-1)
	var ignored json.RawMessage
	return call(ctx, ws, "1", "server.refreshProviders", map[string]any{}, &ignored)
}

func (c Client) dial(ctx context.Context) (*websocket.Conn, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/auth/websocket-ticket", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errUnauthorized
	}
	var ticket struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil || ticket.Ticket == "" {
		return nil, fmt.Errorf("t3: no websocket ticket, status %d", resp.StatusCode)
	}
	target := "ws" + strings.TrimPrefix(base, "http") + "/ws?wsTicket=" + url.QueryEscape(ticket.Ticket) +
		"&orchestrationProtocol=" + fmt.Sprint(protocolVersion)
	ws, _, err := websocket.Dial(ctx, target, &websocket.DialOptions{HTTPClient: c.HTTP})
	return ws, err
}

// call sends one Effect RPC request and waits for its exit message.
func call(ctx context.Context, ws *websocket.Conn, id, tag string, payload, out any) error {
	body, err := json.Marshal(map[string]any{"_tag": "Request", "id": id, "tag": tag, "payload": payload, "headers": []any{}})
	if err != nil {
		return err
	}
	if err := ws.Write(ctx, websocket.MessageText, append(body, '\n')); err != nil {
		return err
	}
	for {
		_, raw, err := ws.Read(ctx)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			var msg struct {
				Tag       string `json:"_tag"`
				RequestID string `json:"requestId"`
				Exit      struct {
					Tag   string          `json:"_tag"`
					Value json.RawMessage `json:"value"`
					Cause []struct {
						Error struct {
							Tag string `json:"_tag"`
						} `json:"error"`
					} `json:"cause"`
				} `json:"exit"`
			}
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				return fmt.Errorf("t3: bad rpc message: %w", err)
			}
			if msg.Tag == "Ping" {
				if err := ws.Write(ctx, websocket.MessageText, []byte("{\"_tag\":\"Pong\"}\n")); err != nil {
					return err
				}
			}
			if msg.Tag != "Exit" || msg.RequestID != id {
				continue
			}
			if msg.Exit.Tag == "Success" {
				return json.Unmarshal(msg.Exit.Value, out)
			}
			if len(msg.Exit.Cause) > 0 && msg.Exit.Cause[0].Error.Tag == "EnvironmentAuthorizationError" {
				return errMissingScope
			}
			return fmt.Errorf("t3: %s failed", tag)
		}
	}
}

func limits(cfg wireConfig) []ProviderLimits {
	out := []ProviderLimits{}
	for _, p := range cfg.Providers {
		if !p.Enabled {
			continue
		}
		item := ProviderLimits{Provider: p.InstanceID, Name: p.DisplayName, Windows: []UsageWindow{}}
		if p.UsageLimits != nil {
			item.CheckedAt = &p.UsageLimits.CheckedAt
			for _, w := range p.UsageLimits.Windows {
				item.Windows = append(item.Windows, UsageWindow{Kind: w.Kind, Label: w.Label, UsedPercent: w.UsedPercent, ResetsAt: w.ResetsAt, WindowMinutes: w.WindowDurationMin})
			}
		}
		out = append(out, item)
	}
	return out
}

// models keeps cost sources apart so an estimate never adds into a reported cost.
func models(u wireUsage) []ModelUsage {
	byKey := map[[3]string]*ModelUsage{}
	for _, b := range u.Buckets {
		key := [3]string{b.Provider, b.Model, b.CostSource}
		m := byKey[key]
		if m == nil {
			m = &ModelUsage{Provider: b.Provider, Model: b.Model, CostSource: b.CostSource}
			byKey[key] = m
		}
		m.InputTokens += b.Totals.Uncached + b.Totals.Creation
		m.CachedTokens += b.Totals.Cached
		m.OutputTokens += b.Totals.Output
		m.CostUSD += b.CostUSD
	}
	out := make([]ModelUsage, 0, len(byKey))
	for _, m := range byKey {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return a.Provider+a.Model+a.CostSource < b.Provider+b.Model+b.CostSource
	})
	return out
}
