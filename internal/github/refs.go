package github

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

// PullRef is a pull request in any state, for # references in the composer.
type PullRef struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	// State is open, draft, merged or closed.
	State string `json:"state"`
}

// PullRefs searches a repository's pull requests in every state (open, draft,
// merged, closed), newest first. query matches the title or the number.
func (c *Client) PullRefs(ctx context.Context, repo, query string) ([]PullRef, error) {
	if c.Token == "" {
		return nil, ErrUnconfigured
	}
	// Only followed repositories; the name may come lowercased.
	repos, err := c.repos(ctx)
	if err != nil {
		return nil, err
	}
	known := ""
	for name := range repos {
		if strings.EqualFold(name, repo) {
			known = name
		}
	}
	if known == "" {
		return nil, ErrUnknownRepo
	}
	repo = known
	q := "repo:" + repo + " is:pr"
	if t := strings.TrimSpace(query); t != "" {
		q += " " + t
	}
	raw, err := c.get(ctx, "/search/issues?"+url.Values{"q": {q}, "sort": {"updated"}, "order": {"desc"}, "per_page": {"30"}}.Encode(), "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var res struct {
		Items []struct {
			Number      int    `json:"number"`
			Title       string `json:"title"`
			State       string `json:"state"`
			Draft       bool   `json:"draft"`
			PullRequest struct {
				MergedAt *string `json:"merged_at"`
			} `json:"pull_request"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	out := make([]PullRef, 0, len(res.Items))
	for _, it := range res.Items {
		state := it.State
		switch {
		case it.PullRequest.MergedAt != nil:
			state = "merged"
		case state == "open" && it.Draft:
			state = "draft"
		}
		out = append(out, PullRef{Number: it.Number, Title: it.Title, State: state})
	}
	return out, nil
}
