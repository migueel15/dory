// Package t3 reads the state of T3 Code instances for the control center.
package t3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// protocolVersion is the only orchestration protocol this client understands.
const protocolVersion = 2

var (
	errUnauthorized = errors.New("t3: token rejected")
	errIncompatible = errors.New("t3: unsupported orchestration protocol")

	// ErrThreadNotFound means no available instance reports the thread.
	ErrThreadNotFound = errors.New("t3: thread not found")
)

// Discovery is the public identity of an instance.
type Discovery struct {
	EnvironmentID string `json:"environmentId"`
	Label         string `json:"label"`
	ServerVersion string `json:"serverVersion"`
	Protocol      int    `json:"orchestrationProtocolVersion"`
	Platform      struct {
		OS string `json:"os"`
	} `json:"platform"`
}

// Thread keeps only the fields of a T3 thread that Dory shows.
type Thread struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	UpdatedAt   string `json:"updated_at"`
	Attention   bool   `json:"attention"`
	SettledAt   string `json:"-"`
	RunID       string `json:"-"`
	CompletedAt string `json:"-"`
	RequestID   string `json:"-"`
	URL         string `json:"url"`
	// EnvironmentID is the T3 environment, used for app deep links.
	EnvironmentID string `json:"t3_environment_id"`
}

type wireThread struct {
	LatestRunID          string          `json:"latestRunId"`
	LatestRunCompletedAt string          `json:"latestRunCompletedAt"`
	ID                   string          `json:"id"`
	ProjectID            string          `json:"projectId"`
	Title                string          `json:"title"`
	Status               string          `json:"status"`
	UpdatedAt            string          `json:"updatedAt"`
	SettledAt            *string         `json:"settledAt"`
	SnoozedUntil         *string         `json:"snoozedUntil"`
	PendingRequest       json.RawMessage `json:"pendingRuntimeRequest"`
	ModelSelection       struct {
		Model   string `json:"model"`
		Options []struct {
			ID    string          `json:"id"`
			Value json.RawMessage `json:"value"`
		} `json:"options"`
	} `json:"modelSelection"`
}

func (w wireThread) effort() string {
	for _, id := range effortOptionIDs {
		for _, option := range w.ModelSelection.Options {
			var value string
			if option.ID == id && json.Unmarshal(option.Value, &value) == nil && value != "" {
				return value
			}
		}
	}
	return ""
}

func (w wireThread) attention() bool {
	if w.SettledAt != nil || w.SnoozedUntil != nil {
		return false
	}
	pending := len(w.PendingRequest) > 0 && string(w.PendingRequest) != "null"
	return pending || w.Status == "waiting" || w.Status == "failed"
}

// Client talks to one instance. A zero HTTP timeout is never used.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func (c Client) get(ctx context.Context, path string, auth bool, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("x-t3-orchestration-protocol", fmt.Sprint(protocolVersion))
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errUnauthorized
	}
	// Unknown T3 routes answer 200 with the web app, so require JSON.
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		return fmt.Errorf("t3: %s returned %d %s", path, resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Discover reads the public identity and rejects unknown protocols.
func (c Client) Discover(ctx context.Context) (Discovery, error) {
	var d Discovery
	if err := c.get(ctx, "/.well-known/t3/environment", false, &d); err != nil {
		return d, err
	}
	if d.Protocol != protocolVersion {
		return d, errIncompatible
	}
	return d, nil
}

// Threads returns the threads that are not settled or need attention.
func (c Client) Threads(ctx context.Context) ([]Thread, error) { return c.threads(ctx, false) }

func (c Client) threads(ctx context.Context, all bool) ([]Thread, error) {
	var snap struct {
		Threads []wireThread `json:"threads"`
	}
	path := "/api/orchestration/shell?omitIdleSubagents=true&omitOlderSettled=true"
	if all {
		path = "/api/orchestration/shell?omitIdleSubagents=false&omitOlderSettled=false"
	}
	if err := c.get(ctx, path, true, &snap); err != nil {
		return nil, err
	}
	out := make([]Thread, 0, len(snap.Threads))
	for _, w := range snap.Threads {
		if w.SettledAt != nil && !all {
			continue
		}
		settled := ""
		if w.SettledAt != nil {
			settled = *w.SettledAt
		}
		var pending struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(w.PendingRequest, &pending)
		out = append(out, Thread{RunID: w.LatestRunID, CompletedAt: w.LatestRunCompletedAt, RequestID: pending.ID, SettledAt: settled, ID: w.ID, ProjectID: w.ProjectID, Title: w.Title, Status: w.Status, Model: w.ModelSelection.Model, Effort: w.effort(), UpdatedAt: w.UpdatedAt, Attention: w.attention()})
	}
	return out, nil
}

// requestTimeout bounds each call so one dead machine cannot stall a poll.
const requestTimeout = 5 * time.Second
