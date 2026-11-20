package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

const (
	// watchFast applies while a client shows branches or pull requests.
	watchFast = 5 * time.Second
	watchSlow = time.Minute
	// beatTTL is how long a heartbeat keeps the fast pace.
	beatTTL = 45 * time.Second
)

type watchEntry struct {
	etag string
	// print is the last seen state of each listed pull request.
	print map[int]string
}

type watchedPull struct {
	Number    int     `json:"number"`
	State     string  `json:"state"`
	Draft     bool    `json:"draft"`
	MergedAt  *string `json:"merged_at"`
	UpdatedAt string  `json:"updated_at"`
	Head      struct {
		Ref string `json:"ref"`
	} `json:"head"`
}

// Beat keeps the fast pace while a client shows branches or pull requests.
func (c *Client) Beat() {
	c.mu.Lock()
	c.beat = time.Now()
	c.mu.Unlock()
	c.Kick()
}

// Kick asks the watcher to check now, after an action from Dory.
func (c *Client) Kick() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

// Watch polls the recently updated pull requests of every followed
// repository with one conditional request each. An unchanged list answers 304
// and costs no quota. When a pull request changes, it refreshes the caches
// and calls changed so clients refetch at once.
func (c *Client) Watch(ctx context.Context, changed func()) {
	var wait time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.kick:
		case <-time.After(wait):
		}
		backoff := c.watchOnce(ctx, changed)
		c.mu.Lock()
		wait = watchSlow
		if time.Since(c.beat) < beatTTL {
			wait = watchFast
		}
		c.mu.Unlock()
		wait = max(wait, backoff)
	}
}

func (c *Client) watchOnce(ctx context.Context, changed func()) time.Duration {
	// The token may be set later from the settings: keep waiting for it.
	if !c.Configured(ctx) {
		return 0
	}
	all, err := c.repos(ctx)
	if err != nil {
		return 0
	}
	var backoff time.Duration
	dirty := map[string]string{}
	for repo, project := range unique(all) {
		ok, wait, err := c.pollRepo(ctx, repo)
		backoff = max(backoff, wait)
		if err != nil {
			log.Printf("github: watch %s: %v", repo, err)
			if wait > 0 {
				break
			}
			continue
		}
		if ok {
			dirty[repo] = project
		}
	}
	if len(dirty) == 0 {
		return backoff
	}
	for repo, project := range dirty {
		if data, err := c.fetchRepo(ctx, repo, project); err == nil {
			c.mu.Lock()
			c.byRepo[repo] = data
			c.mu.Unlock()
		}
	}
	changed()
	return backoff
}

// pollRepo reports whether any listed pull request changed since the last
// poll. The first poll of a repository only records the list.
func (c *Client) pollRepo(ctx context.Context, repo string) (bool, time.Duration, error) {
	c.mu.Lock()
	prev, seen := c.watched[repo]
	c.mu.Unlock()
	path := "/repos/" + repo + "/pulls?state=all&sort=updated&direction=desc&per_page=30"
	r, err := c.getTagged(ctx, path, "application/vnd.github+json", prev.etag)
	if err != nil || r.notModified {
		return false, r.retryAfter, err
	}
	var pulls []watchedPull
	if err := json.Unmarshal(r.body, &pulls); err != nil {
		return false, 0, err
	}
	next := watchEntry{etag: r.etag, print: map[int]string{}}
	diff := false
	for _, p := range pulls {
		state := finalState(p.State, p.Draft, p.MergedAt != nil)
		next.print[p.Number] = state + "@" + p.UpdatedAt
		if prev.print[p.Number] != next.print[p.Number] {
			diff = true
			storeState(repo, p, state)
		}
	}
	c.mu.Lock()
	c.watched[repo] = next
	c.mu.Unlock()
	return seen && diff, 0, nil
}

// storeState updates the PullState cache by number and by head branch, so a
// task linked either way sees the change. The newest pull request of a
// branch comes first in the list, so an older one never overwrites it.
func storeState(repo string, p watchedPull, state string) {
	now := time.Now()
	ref := PullStatus{p.Number, state}
	states.Store(fmt.Sprintf("%s#%d", repo, p.Number), stateEntry{ref, now})
	if p.Head.Ref == "" {
		return
	}
	key := repo + "#" + p.Head.Ref
	if e, ok := states.Load(key); ok && e.(stateEntry).ref.Number > p.Number {
		return
	}
	states.Store(key, stateEntry{ref, now})
}
