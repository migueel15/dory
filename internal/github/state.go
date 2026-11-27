package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// PullStatus is a pull request and its final state: open, draft, merged or closed.
type PullStatus struct {
	Number int    `json:"number"`
	State  string `json:"state"`
}

var states sync.Map // key -> stateEntry

type stateEntry struct {
	ref PullStatus
	at  time.Time
}

// CachedPullState returns a pull request status already held in memory
// (for instance from pollRepo) without making a network request.
func (c *Client) CachedPullState(repo string, number *int64, branch string) (PullStatus, bool) {
	key := repo + "#" + branch
	if number != nil {
		key = fmt.Sprintf("%s#%d", repo, *number)
	}
	e, ok := states.Load(key)
	if !ok {
		return PullStatus{}, false
	}
	return e.(stateEntry).ref, true
}

// PullState finds a pull request by number or, without one, by head branch,
// including closed and merged ones. A merged pull request never changes, so
// it stays cached; any other answer older than a minute is still returned at
// once while a background request refreshes it.
func (c *Client) PullState(ctx context.Context, repo string, number *int64, branch string) (PullStatus, error) {
	if !c.Configured(ctx) {
		return PullStatus{}, ErrUnconfigured
	}
	key := repo + "#" + branch
	if number != nil {
		key = fmt.Sprintf("%s#%d", repo, *number)
	}
	if e, ok := states.Load(key); ok {
		entry := e.(stateEntry)
		if entry.ref.State != "merged" && time.Since(entry.at) >= cacheTTL {
			if _, busy := refreshingStates.LoadOrStore(key, true); !busy {
				go func() {
					defer refreshingStates.Delete(key)
					bg, cancel := context.WithTimeout(context.Background(), refreshTimeout)
					defer cancel()
					_, _ = c.fetchPullState(bg, key, repo, number, branch)
				}()
			}
		}
		return entry.ref, nil
	}
	return c.fetchPullState(ctx, key, repo, number, branch)
}

var refreshingStates sync.Map // key -> true while a background refresh runs

func (c *Client) fetchPullState(ctx context.Context, key, repo string, number *int64, branch string) (PullStatus, error) {
	type wire struct {
		Number   int     `json:"number"`
		State    string  `json:"state"`
		Draft    bool    `json:"draft"`
		MergedAt *string `json:"merged_at"`
	}
	var pulls []wire
	if number != nil {
		body, err := c.get(ctx, fmt.Sprintf("/repos/%s/pulls/%d", repo, *number), "application/vnd.github+json")
		if err != nil {
			return PullStatus{}, err
		}
		var p wire
		if err = json.Unmarshal(body, &p); err != nil {
			return PullStatus{}, err
		}
		pulls = []wire{p}
	} else {
		owner, _, _ := strings.Cut(repo, "/")
		q := url.Values{"head": {owner + ":" + branch}, "state": {"all"}, "per_page": {"5"}}
		body, err := c.get(ctx, "/repos/"+repo+"/pulls?"+q.Encode(), "application/vnd.github+json")
		if err != nil {
			return PullStatus{}, err
		}
		if err = json.Unmarshal(body, &pulls); err != nil {
			return PullStatus{}, err
		}
	}
	var ref PullStatus
	if len(pulls) > 0 {
		// The newest pull request of the branch wins.
		p := pulls[0]
		ref = PullStatus{p.Number, finalState(p.State, p.Draft, p.MergedAt != nil)}
	}
	states.Store(key, stateEntry{ref, time.Now()})
	return ref, nil
}

// finalState maps a REST pull request to open, draft, merged or closed.
func finalState(state string, draft, merged bool) string {
	switch {
	case merged:
		return "merged"
	case state == "closed":
		return "closed"
	case draft:
		return "draft"
	}
	return "open"
}
