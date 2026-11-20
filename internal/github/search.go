package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type SearchHit struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Kind   string `json:"kind"`
	State  string `json:"state"`
}
type SearchResult struct {
	Items      []SearchHit `json:"items"`
	Incomplete bool        `json:"incomplete"`
}

var exactNumber = regexp.MustCompile(`^#[1-9][0-9]*$`)
var searchWord = regexp.MustCompile(`[\pL\pN]+`)

// Search searches only followed repositories, keeping open and closed limits separate.
// Individual upstream failures leave the other results usable and mark the response incomplete.
func (c *Client) Search(ctx context.Context, query string) (SearchResult, error) {
	out := SearchResult{Items: []SearchHit{}}
	if !c.Configured(ctx) {
		return out, ErrUnconfigured
	}
	repos, err := c.repos(ctx)
	if err != nil {
		return out, err
	}
	names := make([]string, 0, len(repos))
	known := map[string]string{}
	for name := range repos {
		key := strings.ToLower(name)
		if _, ok := known[key]; !ok {
			names = append(names, name)
			known[key] = name
		}
	}
	sort.Strings(names)
	query = strings.TrimSpace(query)
	if len(query) > 200 || query == "" {
		return out, ErrInvalid
	}
	number := 0
	if exactNumber.MatchString(query) {
		number, _ = strconv.Atoi(query[1:])
	}
	words := searchWord.FindAllString(query, -1)
	if number == 0 && len(words) == 0 {
		return out, nil
	}
	type item struct {
		Number        int    `json:"number"`
		Title         string `json:"title"`
		State         string `json:"state"`
		Draft         bool   `json:"draft"`
		RepositoryURL string `json:"repository_url"`
		PullRequest   *struct {
			MergedAt *string `json:"merged_at"`
		} `json:"pull_request"`
	}
	groups := [2][]SearchHit{}
	seen := map[string]bool{}
	add := func(it item, repo string, group int) {
		if it.Number <= 0 {
			return
		}
		kind := "issues"
		state := it.State
		if it.PullRequest != nil {
			kind = "pulls"
			if it.PullRequest.MergedAt != nil {
				state = "merged"
			} else if state == "open" && it.Draft {
				state = "draft"
			}
		}
		key := strings.ToLower(fmt.Sprintf("%s/%s/%d", repo, kind, it.Number))
		if seen[key] {
			return
		}
		seen[key] = true
		groups[group] = append(groups[group], SearchHit{repo, it.Number, it.Title, kind, state})
	}
	if number > 0 {
		for _, repo := range names {
			if ctx.Err() != nil {
				out.Incomplete = true
				break
			}
			raw, e := c.get(ctx, fmt.Sprintf("/repos/%s/issues/%d", repo, number), "application/vnd.github+json")
			if e != nil {
				if !strings.Contains(e.Error(), "returned 404") {
					out.Incomplete = true
				}
				continue
			}
			var it item
			if json.Unmarshal(raw, &it) != nil {
				out.Incomplete = true
				continue
			}
			group := 0
			if it.State != "open" {
				group = 1
			}
			add(it, repo, group)
		}
	} else if len(names) > 0 {

		terms := make([]string, len(words))
		for i, word := range words {
			terms[i] = `"` + word + `"`
		}
		for group, state := range []string{"open", "closed"} {
			for start := 0; start < len(names); start += 5 {
				if ctx.Err() != nil {
					out.Incomplete = true
					break
				}
				end := min(start+5, len(names))
				scopes := make([]string, 0, end-start)
				for _, name := range names[start:end] {
					scopes = append(scopes, "repo:"+name)
				}
				q := strings.Join(scopes, " ") + " is:" + state + " in:title,body " + strings.Join(terms, " ")
				raw, e := c.get(ctx, "/search/issues?"+url.Values{"q": {q}, "per_page": {"30"}}.Encode(), "application/vnd.github+json")
				if e != nil {
					out.Incomplete = true
					continue
				}
				var res struct {
					Items      []item `json:"items"`
					Incomplete bool   `json:"incomplete_results"`
				}
				if json.Unmarshal(raw, &res) != nil {
					out.Incomplete = true
					continue
				}
				out.Incomplete = out.Incomplete || res.Incomplete
				for _, it := range res.Items {
					prefix := c.Base + "/repos/"
					if !strings.HasPrefix(it.RepositoryURL, prefix) {
						continue
					}
					repo, ok := known[strings.ToLower(strings.TrimPrefix(it.RepositoryURL, prefix))]
					if !ok {
						continue
					}
					add(it, repo, group)
				}
			}
		}
	}

	out.Items = append(groups[0], groups[1]...)
	return out, nil
}
