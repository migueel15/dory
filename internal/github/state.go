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

// PullState finds a pull request by number or, without one, by head branch,
// including closed and merged ones. Results are cached for a minute.
func (c *Client) PullState(ctx context.Context, repo string, number *int64, branch string) (PullStatus, error) {
	if c.Token == "" {
		return PullStatus{}, ErrUnconfigured
	}
	key := repo + "#" + branch
	if number != nil {
		key = fmt.Sprintf("%s#%d", repo, *number)
	}
	if e, ok := states.Load(key); ok && time.Since(e.(stateEntry).at) < cacheTTL {
		return e.(stateEntry).ref, nil
	}
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
		ref.Number = p.Number
		switch {
		case p.MergedAt != nil:
			ref.State = "merged"
		case p.State == "closed":
			ref.State = "closed"
		case p.Draft:
			ref.State = "draft"
		default:
			ref.State = "open"
		}
	}
	states.Store(key, stateEntry{ref, time.Now()})
	return ref, nil
}
