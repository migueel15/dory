package github

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"strings"
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
}

// Overview groups open work by type.
type Overview struct {
	Issues    []Issue `json:"issues"`
	Pulls     []Pull  `json:"pulls"`
	FetchedAt string  `json:"fetched_at"`
}

const pullFields = `id number title url state isDraft updatedAt headRefName baseRefName reviewDecision mergeable mergeStateStatus
  author { login } labels(first: 10) { nodes { name } }
  closingIssuesReferences(first: 10) { nodes { number title } }
  latestOpinionatedReviews(first: 20) { nodes { state author { login } } }
  reviewRequests(first: 10) { nodes { requestedReviewer { ... on User { login } ... on Team { name } } } }
  commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
  baseRef { refUpdateRule { requiredApprovingReviewCount } }`

const overviewQuery = `query($owner: String!, $name: String!) { repository(owner: $owner, name: $name) {
  pullRequests(states: OPEN, first: 50, orderBy: {field: UPDATED_AT, direction: DESC}) { nodes { ` + pullFields + ` } }
  issues(states: OPEN, first: 50, orderBy: {field: UPDATED_AT, direction: DESC}) { nodes {
    number title url state updatedAt author { login } labels(first: 10) { nodes { name } }
    closedByPullRequestsReferences(first: 10) { nodes { number title } }
  } }
} }`

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
	Author           *login `json:"author"`
	Labels           labels `json:"labels"`
	Closing          refs   `json:"closingIssuesReferences"`
	Reviews          struct {
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
	ID        string `json:"id"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	State     string `json:"state"`
	UpdatedAt string `json:"updatedAt"`
	Author    *login `json:"author"`
	Labels    labels `json:"labels"`
	Closed    refs   `json:"closedByPullRequestsReferences"`
}

func name(l *login) string {
	if l == nil {
		return "ghost"
	}
	return l.Login
}

func (w wirePull) pull(repo, project string) Pull {
	p := Pull{Repo: repo, ProjectID: project, Number: w.Number, Title: w.Title, Author: name(w.Author), URL: w.URL, State: w.State, UpdatedAt: w.UpdatedAt, Labels: w.Labels.names(), Issues: w.Closing.Nodes, Draft: w.IsDraft, HeadRef: w.HeadRefName, BaseRef: w.BaseRefName, ReviewDecision: w.ReviewDecision, ApprovedBy: []string{}, ChangesBy: []string{}, ReviewRequests: []string{}, Mergeable: w.Mergeable, MergeState: w.MergeStateStatus}
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
func (c *Client) fillRequired(ctx context.Context, p *Pull) {
	if p.RequiredApprovals != nil {
		return
	}
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
	for _, r := range rules {
		if r.Type == "pull_request" && (p.RequiredApprovals == nil || r.Parameters.Required > *p.RequiredApprovals) {
			required := r.Parameters.Required
			p.RequiredApprovals = &required
		}
	}
}

// Open lists open issues and pull requests, using the cache unless refresh is set.
func (c *Client) Open(ctx context.Context, refresh bool) (*Overview, error) {
	if c.Token == "" {
		return nil, ErrUnconfigured
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached != nil && !refresh && time.Since(c.fetched) < cacheTTL {
		return c.cached, nil
	}
	repos, err := c.repos(ctx)
	if err != nil {
		return nil, err
	}
	out := &Overview{Issues: []Issue{}, Pulls: []Pull{}, FetchedAt: time.Now().UTC().Format(time.RFC3339)}
	for repo, project := range repos {
		owner, short, _ := strings.Cut(repo, "/")
		var data struct {
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
			// One inaccessible repository must not hide the others.
			log.Printf("github: %s: %v", repo, err)
			continue
		}
		for _, w := range data.Repository.PullRequests.Nodes {
			p := w.pull(repo, project)
			c.fillRequired(ctx, &p)
			out.Pulls = append(out.Pulls, p)
		}
		for _, w := range data.Repository.Issues.Nodes {
			out.Issues = append(out.Issues, Issue{Repo: repo, ProjectID: project, Number: w.Number, Title: w.Title, Author: name(w.Author), URL: w.URL, State: w.State, UpdatedAt: w.UpdatedAt, Labels: w.Labels.names(), Pulls: w.Closed.Nodes})
		}
	}
	sortByUpdated(out.Pulls, func(p Pull) string { return p.UpdatedAt })
	sortByUpdated(out.Issues, func(i Issue) string { return i.UpdatedAt })
	c.cached, c.fetched = out, time.Now()
	return out, nil
}

func (c *Client) forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cached = nil
}
