package github

import (
	"context"
	"encoding/json"
	"log"
	"maps"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// Ref points to the other side of an issue and pull request link.
type Ref struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// Issue is an open issue with the pull requests that will close it.
type Issue struct {
	Repo      string   `json:"repo"`
	ProjectID string   `json:"project_id"`
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	URL       string   `json:"url"`
	State     string   `json:"state"`
	UpdatedAt string   `json:"updated_at"`
	Labels    []string `json:"labels"`
	Pulls     []Ref    `json:"pulls"`
	// StateReason is COMPLETED, NOT_PLANNED or REOPENED; empty when unknown.
	StateReason string `json:"state_reason"`
	Meta
}

// Meta is the extra context shown on issue and pull request cards.
type Meta struct {
	CreatedAt string   `json:"created_at"`
	ClosedAt  string   `json:"closed_at"`
	Comments  int      `json:"comments"`
	Assignees []string `json:"assignees"`
	Milestone string   `json:"milestone"`
	// LabelColors maps each label to its hex color, without "#".
	LabelColors map[string]string `json:"label_colors"`
}

// Pull is an open pull request with its review, CI and merge state.
type Pull struct {
	Repo              string   `json:"repo"`
	ProjectID         string   `json:"project_id"`
	Number            int      `json:"number"`
	Title             string   `json:"title"`
	Author            string   `json:"author"`
	URL               string   `json:"url"`
	State             string   `json:"state"`
	UpdatedAt         string   `json:"updated_at"`
	Labels            []string `json:"labels"`
	Issues            []Ref    `json:"issues"`
	Draft             bool     `json:"draft"`
	HeadRef           string   `json:"head_ref"`
	BaseRef           string   `json:"base_ref"`
	ReviewDecision    string   `json:"review_decision"`
	Approvals         int      `json:"approvals"`
	ApprovedBy        []string `json:"approved_by"`
	ChangesBy         []string `json:"changes_by"`
	RequiredApprovals *int     `json:"required_approvals"`
	ReviewRequests    []string `json:"review_requests"`
	CI                string   `json:"ci"`
	Mergeable         string   `json:"mergeable"`
	MergeState        string   `json:"merge_state"`
	Additions         int      `json:"additions"`
	Deletions         int      `json:"deletions"`
	ChangedFiles      int      `json:"changed_files"`
	Meta
}

// Overview groups open work by type.
type Overview struct {
	Issues     []Issue `json:"issues"`
	Pulls      []Pull  `json:"pulls"`
	FetchedAt  string  `json:"fetched_at"`
	Refreshing bool    `json:"refreshing"`
	// Viewer is the GitHub login of the token's owner, empty until known.
	Viewer string `json:"viewer"`
}

type repoData struct {
	issues []Issue
	pulls  []Pull
}

type ruleEntry struct {
	required *int
	fetched  time.Time
}

// metaFields uses aliases so it never clashes with the same fields asked with other arguments.
const metaFields = `createdAt closedAt comments { totalCount } assigneeList: assignees(first: 10) { nodes { login } } labelList: labels(first: 10) { nodes { name color } } milestoneInfo: milestone { title }`

const pullFields = `id number title url state isDraft updatedAt headRefName baseRefName reviewDecision mergeable mergeStateStatus additions deletions changedFiles ` + metaFields + `
  author { login } labels(first: 10) { nodes { name } }
  closingIssuesReferences(first: 10) { nodes { number title } }
  latestOpinionatedReviews(first: 20) { nodes { state author { login } } }
  reviewRequests(first: 10) { nodes { requestedReviewer { ... on User { login } ... on Team { name } } } }
  commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
  baseRef { refUpdateRule { requiredApprovingReviewCount } }`

const overviewQuery = `query($owner: String!, $name: String!) { viewer { login } repository(owner: $owner, name: $name) {
  pullRequests(states: OPEN, first: 50, orderBy: {field: UPDATED_AT, direction: DESC}) { nodes { ` + pullFields + ` } }
  issues(states: OPEN, first: 50, orderBy: {field: UPDATED_AT, direction: DESC}) { nodes { ` + issueFields + ` } }
} }`

const issueFields = `number title url state stateReason updatedAt author { login } labels(first: 10) { nodes { name } } ` + metaFields + `
    closedByPullRequestsReferences(first: 10) { nodes { number title } }`

type login struct {
	Login string `json:"login"`
}
type refs struct {
	Nodes []Ref `json:"nodes"`
}
type labels struct {
	Nodes []struct {
		Name string `json:"name"`
	} `json:"nodes"`
}

// wireMeta decodes metaFields.
type wireMeta struct {
	CreatedAt string `json:"createdAt"`
	ClosedAt  string `json:"closedAt"`
	Comments  struct {
		Total int `json:"totalCount"`
	} `json:"comments"`
	AssigneeList struct {
		Nodes []login `json:"nodes"`
	} `json:"assigneeList"`
	LabelList struct {
		Nodes []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"nodes"`
	} `json:"labelList"`
	MilestoneInfo *struct {
		Title string `json:"title"`
	} `json:"milestoneInfo"`
}

func (w wireMeta) meta() Meta {
	m := Meta{CreatedAt: w.CreatedAt, ClosedAt: w.ClosedAt, Comments: w.Comments.Total, Assignees: []string{}, LabelColors: map[string]string{}}
	if w.MilestoneInfo != nil {
		m.Milestone = w.MilestoneInfo.Title
	}
	for _, a := range w.AssigneeList.Nodes {
		m.Assignees = append(m.Assignees, a.Login)
	}
	for _, l := range w.LabelList.Nodes {
		m.LabelColors[l.Name] = l.Color
	}
	return m
}

func (l labels) names() []string {
	out := []string{}
	for _, n := range l.Nodes {
		out = append(out, n.Name)
	}
	return out
}

type wirePull struct {
	ID               string `json:"id"`
	Number           int    `json:"number"`
	Title            string `json:"title"`
	URL              string `json:"url"`
	State            string `json:"state"`
	IsDraft          bool   `json:"isDraft"`
	UpdatedAt        string `json:"updatedAt"`
	HeadRefName      string `json:"headRefName"`
	BaseRefName      string `json:"baseRefName"`
	ReviewDecision   string `json:"reviewDecision"`
	Mergeable        string `json:"mergeable"`
	MergeStateStatus string `json:"mergeStateStatus"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	ChangedFiles     int    `json:"changedFiles"`
	wireMeta
	Author  *login `json:"author"`
	Labels  labels `json:"labels"`
	Closing refs   `json:"closingIssuesReferences"`
	Reviews struct {
		Nodes []struct {
			State  string `json:"state"`
			Author *login `json:"author"`
		} `json:"nodes"`
	} `json:"latestOpinionatedReviews"`
	ReviewRequests struct {
		Nodes []struct {
			Reviewer *struct {
				Login string `json:"login"`
				Name  string `json:"name"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				Rollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	BaseRef *struct {
		Rule *struct {
			Required *int `json:"requiredApprovingReviewCount"`
		} `json:"refUpdateRule"`
	} `json:"baseRef"`
}

type wireIssue struct {
	ID          string `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	State       string `json:"state"`
	UpdatedAt   string `json:"updatedAt"`
	Author      *login `json:"author"`
	Labels      labels `json:"labels"`
	Closed      refs   `json:"closedByPullRequestsReferences"`
	StateReason string `json:"stateReason"`
	wireMeta
}

func (w wireIssue) issue(repo, project string) Issue {
	i := Issue{Repo: repo, ProjectID: project, Number: w.Number, Title: w.Title, Author: name(w.Author), URL: w.URL, State: w.State, UpdatedAt: w.UpdatedAt, Labels: w.Labels.names(), Pulls: w.Closed.Nodes, StateReason: w.StateReason, Meta: w.meta()}
	return i
}

func name(l *login) string {
	if l == nil {
		return "ghost"
	}
	return l.Login
}

func (w wirePull) pull(repo, project string) Pull {
	p := Pull{Repo: repo, ProjectID: project, Number: w.Number, Title: w.Title, Author: name(w.Author), URL: w.URL, State: w.State, UpdatedAt: w.UpdatedAt, Labels: w.Labels.names(), Issues: w.Closing.Nodes, Draft: w.IsDraft, HeadRef: w.HeadRefName, BaseRef: w.BaseRefName, ReviewDecision: w.ReviewDecision, ApprovedBy: []string{}, ChangesBy: []string{}, ReviewRequests: []string{}, Mergeable: w.Mergeable, MergeState: w.MergeStateStatus, Additions: w.Additions, Deletions: w.Deletions, ChangedFiles: w.ChangedFiles, Meta: w.meta()}
	for _, r := range w.Reviews.Nodes {
		switch r.State {
		case "APPROVED":
			p.ApprovedBy = append(p.ApprovedBy, name(r.Author))
		case "CHANGES_REQUESTED":
			p.ChangesBy = append(p.ChangesBy, name(r.Author))
		}
	}
	p.Approvals = len(p.ApprovedBy)
	for _, r := range w.ReviewRequests.Nodes {
		if r.Reviewer != nil {
			p.ReviewRequests = append(p.ReviewRequests, r.Reviewer.Login+r.Reviewer.Name)
		}
	}
	if len(w.Commits.Nodes) > 0 && w.Commits.Nodes[0].Commit.Rollup != nil {
		p.CI = w.Commits.Nodes[0].Commit.Rollup.State
	}
	if w.BaseRef != nil && w.BaseRef.Rule != nil {
		p.RequiredApprovals = w.BaseRef.Rule.Required
	}
	return p
}

// fillRequired covers repository rulesets, which refUpdateRule does not report.
// Rules change rarely, so they are cached per repository and branch.
func (c *Client) fillRequired(ctx context.Context, p *Pull) {
	if p.RequiredApprovals != nil {
		return
	}
	key := p.Repo + "@" + p.BaseRef
	c.mu.Lock()
	r, ok := c.rules[key]
	c.mu.Unlock()
	if !ok || time.Since(r.fetched) > rulesTTL {
		body, err := c.get(ctx, "/repos/"+p.Repo+"/rules/branches/"+url.PathEscape(p.BaseRef), "application/vnd.github+json")
		if err != nil {
			log.Printf("github: rules of %s: %v", p.Repo, err)
			return
		}
		var rules []struct {
			Type       string `json:"type"`
			Parameters struct {
				Required int `json:"required_approving_review_count"`
			} `json:"parameters"`
		}
		if json.Unmarshal(body, &rules) != nil {
			return
		}
		r = ruleEntry{fetched: time.Now()}
		for _, rule := range rules {
			if rule.Type == "pull_request" && (r.required == nil || rule.Parameters.Required > *r.required) {
				required := rule.Parameters.Required
				r.required = &required
			}
		}
		c.mu.Lock()
		c.rules[key] = r
		c.mu.Unlock()
	}
	p.RequiredApprovals = r.required
}

// Open returns the cached issues and pull requests at once. When the cache is
// stale, or refresh is set, it starts a background refresh that updates each
// repository as soon as it answers; Refreshing tells the caller to poll.
func (c *Client) Open(ctx context.Context, refresh bool) (*Overview, error) {
	if c.Token == "" {
		return nil, ErrUnconfigured
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.refreshing && (refresh || time.Since(c.fetched) >= cacheTTL) {
		all, err := c.repos(ctx)
		if err != nil {
			return nil, err
		}
		repos := unique(all)
		for repo := range c.byRepo {
			if _, ok := repos[repo]; !ok {
				delete(c.byRepo, repo)
			}
		}
		c.refreshing = true
		go c.refresh(repos)
	}
	out := &Overview{Issues: []Issue{}, Pulls: []Pull{}, Refreshing: c.refreshing, Viewer: c.viewer}
	if !c.fetched.IsZero() {
		out.FetchedAt = c.fetched.UTC().Format(time.RFC3339)
	}
	for _, r := range c.byRepo {
		out.Pulls = append(out.Pulls, r.pulls...)
		out.Issues = append(out.Issues, r.issues...)
	}
	sortByUpdated(out.Pulls, func(p Pull) string { return p.UpdatedAt })
	sortByUpdated(out.Issues, func(i Issue) string { return i.UpdatedAt })
	return out, nil
}

// unique keeps one entry per GitHub repository: names are case-insensitive, so
// the same repository registered twice must not be queried, and shown, twice.
// The first name in sorted order wins, which keeps the choice stable.
func unique(repos map[string]string) map[string]string {
	names := slices.Sorted(maps.Keys(repos))
	seen := map[string]bool{}
	out := make(map[string]string, len(repos))
	for _, name := range names {
		if key := strings.ToLower(name); !seen[key] {
			seen[key] = true
			out[name] = repos[name]
		}
	}
	return out
}

// refresh queries every repository in parallel and stores each one as it finishes.
func (c *Client) refresh(repos map[string]string) {
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	var wg sync.WaitGroup
	for repo, project := range repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := c.fetchRepo(ctx, repo, project)
			if err != nil {
				// One inaccessible repository must not hide the others.
				log.Printf("github: %s: %v", repo, err)
				return
			}
			c.mu.Lock()
			c.byRepo[repo] = data
			c.mu.Unlock()
		}()
	}
	wg.Wait()
	c.closeMerged(ctx)
	c.mu.Lock()
	c.refreshing, c.fetched = false, time.Now()
	c.mu.Unlock()
}

func (c *Client) fetchRepo(ctx context.Context, repo, project string) (repoData, error) {
	owner, short, _ := strings.Cut(repo, "/")
	var data struct {
		Viewer     login `json:"viewer"`
		Repository struct {
			PullRequests struct {
				Nodes []wirePull `json:"nodes"`
			} `json:"pullRequests"`
			Issues struct {
				Nodes []wireIssue `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
	}
	if err := c.graphql(ctx, overviewQuery, map[string]any{"owner": owner, "name": short}, &data); err != nil {
		return repoData{}, err
	}
	if data.Viewer.Login != "" {
		c.mu.Lock()
		c.viewer = data.Viewer.Login
		c.mu.Unlock()
	}
	out := repoData{pulls: make([]Pull, len(data.Repository.PullRequests.Nodes)), issues: []Issue{}}
	var wg sync.WaitGroup
	for i, w := range data.Repository.PullRequests.Nodes {
		out.pulls[i] = w.pull(repo, project)
		wg.Add(1)
		go func() { defer wg.Done(); c.fillRequired(ctx, &out.pulls[i]) }()
	}
	wg.Wait()
	for _, w := range data.Repository.Issues.Nodes {
		out.issues = append(out.issues, w.issue(repo, project))
	}
	return out, nil
}

// forget marks the cache stale so the next Open refreshes it in the background.
func (c *Client) forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fetched = time.Time{}
}
