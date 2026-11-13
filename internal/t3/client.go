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
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Model     string `json:"model"`
	// Provider is the T3 provider instance running the thread (codex, claudeAgent…).
	Provider    string `json:"provider,omitempty"`
	Effort      string `json:"effort"`
	UpdatedAt   string `json:"updated_at"`
	Attention   bool   `json:"attention"`
	SettledAt   string `json:"settled_at,omitempty"`
	RunID       string `json:"-"`
	CompletedAt string `json:"-"`
	RequestID   string `json:"-"`
	URL         string `json:"url"`
	// EnvironmentID is the T3 environment, used for app deep links.
	EnvironmentID string `json:"t3_environment_id"`
	// Organization of the thread in T3 Code's sidebar.
	ArchivedAt    string `json:"archived_at,omitempty"`
	PinnedAt      string `json:"pinned_at,omitempty"`
	PinOrderKey   string `json:"pin_order_key,omitempty"`
	SnoozedUntil  string `json:"snoozed_until,omitempty"`
	LastVisitedAt string `json:"last_visited_at,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	Branch        string `json:"branch,omitempty"`
	WorktreePath  string `json:"worktree_path,omitempty"`
	// ParentID is the thread that spawned this one; Relation says how
	// ("subagent" or "fork").
	ParentID string `json:"parent_id,omitempty"`
	Relation string `json:"relation,omitempty"`
}

type wireThread struct {
	LatestRunID          string  `json:"latestRunId"`
	LatestRunCompletedAt string  `json:"latestRunCompletedAt"`
	ID                   string  `json:"id"`
	ProjectID            string  `json:"projectId"`
	Title                string  `json:"title"`
	Status               string  `json:"status"`
	UpdatedAt            string  `json:"updatedAt"`
	SettledAt            *string `json:"settledAt"`
	SnoozedUntil         *string `json:"snoozedUntil"`
	ArchivedAt           *string `json:"archivedAt"`
	DeletedAt            *string `json:"deletedAt"`
	PinnedAt             *string `json:"pinnedAt"`
	PinOrderKey          *string `json:"pinOrderKey"`
	LastVisitedAt        *string `json:"lastVisitedAt"`
	CreatedAt            string  `json:"createdAt"`
	Branch               *string `json:"branch"`
	WorktreePath         *string `json:"worktreePath"`
	Lineage              struct {
		ParentThreadID       *string `json:"parentThreadId"`
		RelationshipToParent *string `json:"relationshipToParent"`
	} `json:"lineage"`
	PendingRequest json.RawMessage `json:"pendingRuntimeRequest"`
	ModelSelection struct {
		InstanceID string `json:"instanceId"`
		Model      string `json:"model"`
		Options    []struct {
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
// Threads returns the threads T3 Code shows as active: neither settled nor
// archived, and subagents only while they work or need the user.
func (c Client) Threads(ctx context.Context) ([]Thread, error) {
	active, _, err := c.shell(ctx)
	return active, err
}

// shell reads every thread of the instance in one request (settled, archived
// and subagents included) and also returns the active ones.
func (c Client) shell(ctx context.Context) (active, all []Thread, err error) {
	var snap struct {
		Threads  []wireThread `json:"threads"`
		Archived []wireThread `json:"archivedThreads"`
	}
	if err := c.get(ctx, "/api/orchestration/shell?omitIdleSubagents=false&omitOlderSettled=false", true, &snap); err != nil {
		return nil, nil, err
	}
	// The shell leaves archived threads out (its archivedThreads stays empty);
	// T3 Code reads them with their own RPC. Without them the list still works.
	var archived struct {
		Threads []wireThread `json:"threads"`
	}
	if err := c.call(ctx, "orchestration.getArchivedShellSnapshot", map[string]any{}, &archived); err == nil {
		snap.Archived = append(snap.Archived, archived.Threads...)
	}
	all = make([]Thread, 0, len(snap.Threads)+len(snap.Archived))
	seen := map[string]bool{}
	for _, w := range append(snap.Threads, snap.Archived...) {
		if w.DeletedAt != nil || seen[w.ID] {
			continue
		}
		seen[w.ID] = true
		t := w.thread()
		all = append(all, t)
		if t.SettledAt != "" || t.ArchivedAt != "" || t.Relation == "subagent" && !t.Attention && !working(t.Status) {
			continue
		}
		active = append(active, t)
	}
	if active == nil {
		active = []Thread{}
	}
	return active, all, nil
}

func working(status string) bool {
	switch status {
	case "running", "starting", "preparing", "waiting":
		return true
	}
	return false
}

func (w wireThread) thread() Thread {
	var pending struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.PendingRequest, &pending)
	return Thread{
		RunID: w.LatestRunID, CompletedAt: w.LatestRunCompletedAt, RequestID: pending.ID, SettledAt: str(w.SettledAt),
		ID: w.ID, ProjectID: w.ProjectID, Title: w.Title, Status: w.Status, Model: w.ModelSelection.Model, Provider: w.ModelSelection.InstanceID, Effort: w.effort(),
		UpdatedAt: w.UpdatedAt, Attention: w.attention(),
		ArchivedAt: str(w.ArchivedAt), PinnedAt: str(w.PinnedAt), PinOrderKey: str(w.PinOrderKey), SnoozedUntil: str(w.SnoozedUntil),
		LastVisitedAt: str(w.LastVisitedAt), CreatedAt: w.CreatedAt, Branch: str(w.Branch), WorktreePath: str(w.WorktreePath),
		ParentID: str(w.Lineage.ParentThreadID), Relation: str(w.Lineage.RelationshipToParent),
	}
}

// requestTimeout bounds each call so one dead machine cannot stall a poll.
const requestTimeout = 5 * time.Second
