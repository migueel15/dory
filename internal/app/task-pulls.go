package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
)

type pullKey struct{ repo, ref string }

func pullNumberKey(repo string, n int64) pullKey {
	return pullKey{strings.ToLower(repo), fmt.Sprintf("#%d", n)}
}

func pullBranchKey(repo, branch string) pullKey {
	return pullKey{strings.ToLower(repo), "branch:" + branch}
}

func taskPullKey(repo string, t data.Task) (pullKey, bool) {
	if repo == "" {
		return pullKey{}, false
	}
	if t.PRNumber != nil {
		return pullNumberKey(repo, *t.PRNumber), true
	}
	if t.Branch != "" {
		return pullBranchKey(repo, t.Branch), true
	}
	return pullKey{}, false
}

type cachedPull struct {
	number int
	state  string
	raw    string
	at     time.Time
}

func (a *Tasks) cachedPulls(ctx context.Context) (map[pullKey]cachedPull, error) {
	rows, err := a.Store.LocalDB().QueryContext(ctx, `SELECT repo, ref, COALESCE(number, 0), state, data, updated_at FROM task_pulls`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[pullKey]cachedPull{}
	for rows.Next() {
		var k pullKey
		var p cachedPull
		var at string
		if err := rows.Scan(&k.repo, &k.ref, &p.number, &p.state, &p.raw, &at); err != nil {
			return nil, err
		}
		p.at, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, err
		}
		out[k] = p
	}
	return out, rows.Err()
}

type TaskPullSource interface {
	Open(context.Context, bool) (*github.Overview, error)
	PullState(context.Context, string, *int64, string) (github.PullStatus, error)
}

type cachedPullStateSource interface {
	CachedPullState(repo string, number *int64, branch string) (github.PullStatus, bool)
}

const (
	taskPullLookups = 8
	taskPullTTL     = time.Hour
)

// RefreshPulls is called serially by the GitHub watcher. It commits a batch
// before reporting a change, so readers never see only one alias of a PR.
// more reports whether uncached tasks remain after this batch.
func (a *Tasks) RefreshPulls(ctx context.Context, source TaskPullSource) (changed, more bool, err error) {
	overview, err := source.Open(ctx, false)
	if err != nil {
		return false, false, err
	}
	existing, err := a.cachedPulls(ctx)
	if err != nil {
		return false, false, err
	}
	tasks, err := a.List(ctx, data.Filter{})
	if err != nil {
		return false, false, err
	}
	threadPulls, err := a.Store.ThreadPulls().ListAll(ctx, data.Filter{})
	if err != nil {
		return false, false, err
	}
	names, err := a.repositoryNames(ctx)
	if err != nil {
		return false, false, err
	}
	now := time.Now().UTC()
	updates := map[pullKey]cachedPull{}
	for _, p := range overview.Pulls {
		raw, err := json.Marshal(p)
		if err != nil {
			return false, false, err
		}
		info := cachedPull{number: p.Number, state: "open", raw: string(raw), at: now}
		if p.Draft {
			info.state = "draft"
		}
		updates[pullNumberKey(p.Repo, int64(p.Number))] = info
		if p.HeadRef != "" {
			key := pullBranchKey(p.Repo, p.HeadRef)
			if updates[key].number < p.Number {
				updates[key] = info
			}
		}
	}

	type pullTarget struct {
		key    pullKey
		repo   string
		number *int64
		branch string
	}
	var targets []pullTarget
	added := map[pullKey]bool{}
	for _, tp := range threadPulls {
		repo := names[tp.RepositoryID]
		if repo == "" || tp.Number <= 0 {
			continue
		}
		n := tp.Number
		key := pullNumberKey(repo, n)
		if added[key] {
			continue
		}
		added[key] = true
		targets = append(targets, pullTarget{key: key, repo: repo, number: &n})
	}
	for _, t := range tasks {
		repo := taskRepo(t, names)
		key, ok := taskPullKey(repo, t)
		if !ok || added[key] {
			continue
		}
		added[key] = true
		targets = append(targets, pullTarget{key: key, repo: repo, number: t.PRNumber, branch: t.Branch})
	}

	recordResolved := func(target pullTarget, ref github.PullStatus) {
		info := cachedPull{number: ref.Number, state: ref.State, raw: "{}", at: now}
		updates[target.key] = info
		if ref.Number != 0 {
			numKey := pullNumberKey(target.repo, int64(ref.Number))
			if updates[numKey].number == 0 {
				updates[numKey] = info
			}
		}
	}

	// Seed task_pulls from in-memory states already fetched by pollRepo
	// (state=all&per_page=30) without spending individual lookup quota.
	if cached, ok := source.(cachedPullStateSource); ok {
		for _, target := range targets {
			if updates[target.key].number != 0 {
				continue
			}
			p := existing[target.key]
			if p.state == "merged" || p.state == "closed" {
				continue
			}
			if ref, ok := cached.CachedPullState(target.repo, target.number, target.branch); ok && ref.Number != 0 {
				recordResolved(target, ref)
			}
		}
	}

	// Rotate even when a lookup fails so inaccessible repositories cannot
	// starve later tasks. Successful misses are also cached for an hour.
	if len(targets) > 0 {
		start := a.pullCursor % len(targets)
		next := start
		remaining := taskPullLookups
		succeeded := 0
		scanned := 0
		for ; scanned < len(targets) && remaining > 0; scanned++ {
			target := targets[next]
			next = (next + 1) % len(targets)
			if updates[target.key].number != 0 {
				continue
			}
			p := existing[target.key]
			if p.state == "merged" || p.state == "closed" {
				continue
			}
			// An open PR disappearing from the overview loses its known
			// status. Wait for the first overview if it is still loading.
			missing := p.raw != "" && p.raw != "{}" && !overview.Refreshing
			if !missing && now.Sub(p.at) < taskPullTTL {
				continue
			}
			remaining--
			ref, err := source.PullState(ctx, target.repo, target.number, target.branch)
			if err != nil {
				log.Printf("app: refresh pull %s %s: %v", target.key.repo, target.key.ref, err)
				continue
			}
			succeeded++
			recordResolved(target, ref)
		}
		a.pullCursor = next
		more = remaining == 0 && scanned < len(targets) && succeeded > 0
	}

	if len(updates) == 0 {
		return false, more, nil
	}
	changed, err = a.savePulls(ctx, updates, existing)
	return changed, more, err
}

// RecordPullState updates the local pull request cache immediately after a
// local GitHub action (such as merge or close), updating both the #number key
// and any branch alias pointing to the same pull request.
func (a *Tasks) RecordPullState(ctx context.Context, repo string, number int, state string) error {
	if repo == "" || number <= 0 || state == "" {
		return nil
	}
	now := time.Now().UTC()
	repoKey := strings.ToLower(repo)
	existing, err := a.cachedPulls(ctx)
	if err != nil {
		return err
	}
	updates := map[pullKey]cachedPull{
		pullNumberKey(repo, int64(number)): {number: number, state: state, raw: "{}", at: now},
	}
	for k, p := range existing {
		if k.repo == repoKey && p.number == number {
			updates[k] = cachedPull{number: number, state: state, raw: "{}", at: now}
		}
	}
	_, err = a.savePulls(ctx, updates, existing)
	return err
}

func (a *Tasks) savePulls(ctx context.Context, updates, existing map[pullKey]cachedPull) (bool, error) {
	tx, err := a.Store.LocalDB().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	changed := false
	for k, p := range updates {
		old := existing[k]
		if old.number != p.number || old.state != p.state || old.raw != p.raw && p.number != 0 {
			changed = true
		}
		// Limit timestamp-only writes to keep the cache fresh on disk.
		if old.number == p.number && old.state == p.state && old.raw == p.raw && p.at.Sub(old.at) < taskPullTTL/2 {
			continue
		}
		var details github.Pull
		if err := json.Unmarshal([]byte(p.raw), &details); err != nil {
			return false, err
		}
		reviews, err := json.Marshal(details.ReviewRequests)
		if err != nil {
			return false, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO task_pulls
			(repo, ref, number, state, draft, ci, review_decision, review_requests, data, updated_at)
			VALUES (?, ?, NULLIF(?, 0), ?, ?, ?, ?, COALESCE(NULLIF(?, 'null'), '[]'), ?, ?)
			ON CONFLICT (repo, ref) DO UPDATE SET number=excluded.number, state=excluded.state,
			draft=excluded.draft, ci=excluded.ci, review_decision=excluded.review_decision,
			review_requests=excluded.review_requests, data=excluded.data, updated_at=excluded.updated_at`,
			k.repo, k.ref, p.number, p.state, p.state == "draft", details.CI, details.ReviewDecision,
			string(reviews), p.raw, p.at.Format(time.RFC3339Nano))
		if err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed, nil
}

func (a *Tasks) repoName(ctx context.Context, t data.Task) (string, error) {
	if t.RepositoryID == nil {
		return "", nil
	}
	r, err := a.Store.Repositories().Get(ctx, *t.RepositoryID)
	if errors.Is(err, data.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	name, _ := github.ParseRemote(r.RemoteURL)
	return name, nil
}
