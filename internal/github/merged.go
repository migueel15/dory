package github

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/migueel15/dory/internal/service"
)

const syncActor = "github-sync"

var refPattern = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/(pull|issues)/(\d+)/?$`)

// ghRef is a pull request or issue URL found in the refs of an item.
type ghRef struct {
	repo   string
	number int
	pull   bool
}

func parseRef(url string) (ghRef, bool) {
	m := refPattern.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return ghRef{}, false
	}
	n, _ := strconv.Atoi(m[4])
	return ghRef{repo: m[1] + "/" + m[2], number: n, pull: m[3] == "pull"}, true
}

// prState is a linked pull request and its GitHub state (OPEN, CLOSED, MERGED).
type prState struct {
	URL   string
	State string
}

// mergedBy reports the pull request that finishes an item: every linked pull
// request must be merged or closed, and at least one merged. A pull request
// closed without merge alone changes nothing.
func mergedBy(prs []prState) (string, bool) {
	merged := ""
	for _, p := range prs {
		switch p.State {
		case "MERGED":
			if merged == "" {
				merged = p.URL
			}
		case "CLOSED":
		default:
			return "", false
		}
	}
	return merged, merged != ""
}

const mergedPullQuery = `query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) {
  pullRequest(number: $number) { url state } } }`

const mergedIssueQuery = `query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) {
  issue(number: $number) { closedByPullRequestsReferences(first: 10, includeClosedPrs: true) { nodes { url state } } } } }`

type wirePRState struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

// linkedPulls returns the pull requests behind a ref: itself, or those closing the issue.
func (c *Client) linkedPulls(ctx context.Context, r ghRef) ([]prState, error) {
	owner, name, _ := strings.Cut(r.repo, "/")
	vars := map[string]any{"owner": owner, "name": name, "number": r.number}
	var data struct {
		Repository struct {
			PullRequest *wirePRState `json:"pullRequest"`
			Issue       *struct {
				Closed struct {
					Nodes []wirePRState `json:"nodes"`
				} `json:"closedByPullRequestsReferences"`
			} `json:"issue"`
		} `json:"repository"`
	}
	q := mergedIssueQuery
	if r.pull {
		q = mergedPullQuery
	}
	if err := c.graphql(ctx, q, vars, &data); err != nil {
		return nil, err
	}
	var out []prState
	if p := data.Repository.PullRequest; p != nil {
		out = append(out, prState{p.URL, p.State})
	}
	if i := data.Repository.Issue; i != nil {
		for _, p := range i.Closed.Nodes {
			out = append(out, prState{p.URL, p.State})
		}
	}
	return out, nil
}

// closeMerged moves to done the in_progress and validating items whose linked
// pull requests are merged: the merge counts as validation.
func (c *Client) closeMerged(ctx context.Context) {
	items, err := c.svc.Items().List(ctx, service.Filter{AnyOf: map[string][]string{"status": {"in_progress", "validating"}}, Limit: 1000})
	if err != nil {
		log.Printf("github: items to close: %v", err)
		return
	}
	for _, it := range items {
		var prs []prState
		failed := false
		for _, url := range it.Refs {
			r, ok := parseRef(url)
			if !ok {
				continue
			}
			got, err := c.linkedPulls(ctx, r)
			if err != nil {
				log.Printf("github: %s: %v", url, err)
				failed = true
				break
			}
			prs = append(prs, got...)
		}
		if failed {
			continue
		}
		url, ok := mergedBy(prs)
		if !ok {
			continue
		}
		it.Status = "done"
		it.BlockedReason = ""
		it.Progress = fmt.Sprintf("Cerrado al mergearse %s", url)
		if _, err := c.svc.Items().Put(ctx, syncActor, it); err != nil {
			log.Printf("github: close item %s: %v", it.ID, err)
		}
	}
}
