package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"
)

// closedPage is how many closed items each repository returns per page.
const closedPage = 20

// Closed is one page of closed issues or pull requests. Cursor is opaque and
// empty when every repository has been read to the end.
type Closed struct {
	Issues []Issue `json:"issues"`
	Pulls  []Pull  `json:"pulls"`
	Cursor string  `json:"cursor"`
}

var closedPullsQuery = `query($owner: String!, $name: String!, $after: String) { repository(owner: $owner, name: $name) {
  pullRequests(states: [CLOSED, MERGED], first: ` + strconv.Itoa(closedPage) + `, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}) {
    pageInfo { hasNextPage endCursor } nodes { ` + pullFields + ` } }
} }`

var closedIssuesQuery = `query($owner: String!, $name: String!, $after: String) { repository(owner: $owner, name: $name) {
  issues(states: CLOSED, first: ` + strconv.Itoa(closedPage) + `, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}) {
    pageInfo { hasNextPage endCursor } nodes { ` + issueFields + ` } }
} }`

type pageInfo struct {
	HasNext bool   `json:"hasNextPage"`
	End     string `json:"endCursor"`
}

// ClosedPage reads the next page of closed items of kind ("pulls" or
// "issues"), limited to one project when project is set. The cursor keeps, for
// each repository, where its next page starts; repositories missing from a
// non-empty cursor are finished.
func (c *Client) ClosedPage(ctx context.Context, kind, project, cursor string) (*Closed, error) {
	if !c.Configured(ctx) {
		return nil, ErrUnconfigured
	}
	if kind != "pulls" && kind != "issues" {
		return nil, ErrInvalid
	}
	all, err := c.repos(ctx)
	if err != nil {
		return nil, err
	}
	repos := map[string]string{}
	for repo, p := range unique(all) {
		if project == "" || p == project {
			repos[repo] = p
		}
	}
	after := map[string]string{}
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(raw, &after) != nil {
			return nil, ErrInvalid.Msgf("bad cursor")
		}
		for repo := range repos {
			if _, ok := after[repo]; !ok {
				delete(repos, repo)
			}
		}
	}
	out := &Closed{Issues: []Issue{}, Pulls: []Pull{}}
	next := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for repo, p := range repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			owner, short, _ := strings.Cut(repo, "/")
			vars := map[string]any{"owner": owner, "name": short, "after": nil}
			if a := after[repo]; a != "" {
				vars["after"] = a
			}
			var data struct {
				Repository struct {
					PullRequests struct {
						PageInfo pageInfo   `json:"pageInfo"`
						Nodes    []wirePull `json:"nodes"`
					} `json:"pullRequests"`
					Issues struct {
						PageInfo pageInfo    `json:"pageInfo"`
						Nodes    []wireIssue `json:"nodes"`
					} `json:"issues"`
				} `json:"repository"`
			}
			q := closedIssuesQuery
			if kind == "pulls" {
				q = closedPullsQuery
			}
			if err := c.graphql(ctx, q, vars, &data); err != nil {
				log.Printf("github: closed %s of %s: %v", kind, repo, err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			info := data.Repository.Issues.PageInfo
			if kind == "pulls" {
				info = data.Repository.PullRequests.PageInfo
				for _, w := range data.Repository.PullRequests.Nodes {
					out.Pulls = append(out.Pulls, w.pull(repo, p))
				}
			} else {
				for _, w := range data.Repository.Issues.Nodes {
					out.Issues = append(out.Issues, w.issue(repo, p))
				}
			}
			if info.HasNext {
				next[repo] = info.End
			}
		}()
	}
	wg.Wait()
	sortByUpdated(out.Pulls, func(p Pull) string { return p.UpdatedAt })
	sortByUpdated(out.Issues, func(i Issue) string { return i.UpdatedAt })
	if len(next) > 0 {
		raw, _ := json.Marshal(next)
		out.Cursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, nil
}
