// Package github reads issues and pull requests of the registered repositories.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/service"
)

const (
	apiBase  = "https://api.github.com"
	cacheTTL = time.Minute
	// maxDiff keeps huge pull requests from filling the browser.
	maxDiff = 2 << 20
)

var (
	ErrUnconfigured = errors.New("github: DORY_GITHUB_TOKEN is not set")
	ErrUnknownRepo  = errors.New("github: repository is not registered")
	remotePattern   = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`)
)

// Entry is an open issue or pull request.
type Entry struct {
	Repo      string `json:"repo"`
	ProjectID string `json:"project_id"`
	Kind      string `json:"kind"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	URL       string `json:"url"`
	Draft     bool   `json:"draft"`
	UpdatedAt string `json:"updated_at"`
}

type wireEntry struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	UpdatedAt   string    `json:"updated_at"`
	PullRequest *struct{} `json:"pull_request"`
	User        struct {
		Login string `json:"login"`
	} `json:"user"`
}

// ParseRemote extracts owner/name from a GitHub remote URL.
func ParseRemote(remote string) (string, bool) {
	m := remotePattern.FindStringSubmatch(remote)
	if m == nil {
		return "", false
	}
	return m[1] + "/" + m[2], true
}

// Client caches the open list because it costs two requests per repository.
type Client struct {
	Token string
	Base  string
	HTTP  *http.Client
	svc   service.Service

	mu      sync.Mutex
	cached  []Entry
	fetched time.Time
}

func New(svc service.Service, token string) *Client {
	return &Client{Token: token, Base: apiBase, HTTP: &http.Client{Timeout: 15 * time.Second}, svc: svc}
}

func (c *Client) get(ctx context.Context, path, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: %s returned %d", path, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxDiff))
}

func (c *Client) repos(ctx context.Context) (map[string]string, error) {
	repos, err := c.svc.Repositories().List(ctx, service.Filter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(repos))
	for _, r := range repos {
		if name, ok := ParseRemote(r.RemoteUrl); ok {
			out[name] = r.ProjectID
		}
	}
	return out, nil
}

// Open lists open issues and pull requests, newest first. Issues of the
// GitHub API include pull requests, so one request per repository is enough.
func (c *Client) Open(ctx context.Context) ([]Entry, error) {
	if c.Token == "" {
		return nil, ErrUnconfigured
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached != nil && time.Since(c.fetched) < cacheTTL {
		return c.cached, nil
	}
	repos, err := c.repos(ctx)
	if err != nil {
		return nil, err
	}
	out := []Entry{}
	for name, project := range repos {
		body, err := c.get(ctx, "/repos/"+name+"/issues?state=open&per_page=100", "application/vnd.github+json")
		if err != nil {
			// One inaccessible repository must not hide the others.
			log.Printf("%v", err)
			continue
		}
		var wire []wireEntry
		if err := json.Unmarshal(body, &wire); err != nil {
			return nil, err
		}
		for _, w := range wire {
			kind := "issue"
			if w.PullRequest != nil {
				kind = "pull"
			}
			out = append(out, Entry{Repo: name, ProjectID: project, Kind: kind, Number: w.Number, Title: w.Title, Author: w.User.Login, URL: w.HTMLURL, Draft: w.Draft, UpdatedAt: w.UpdatedAt})
		}
	}
	slices.SortFunc(out, func(a, b Entry) int { return -cmpStrings(a.UpdatedAt, b.UpdatedAt) })
	c.cached, c.fetched = out, time.Now()
	return out, nil
}

func cmpStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Diff returns a pull request diff, only for registered repositories.
func (c *Client) Diff(ctx context.Context, repo string, number int) (string, error) {
	if c.Token == "" {
		return "", ErrUnconfigured
	}
	repos, err := c.repos(ctx)
	if err != nil {
		return "", err
	}
	if _, ok := repos[repo]; !ok {
		return "", ErrUnknownRepo
	}
	body, err := c.get(ctx, fmt.Sprintf("/repos/%s/pulls/%d", repo, number), "application/vnd.github.diff")
	return string(body), err
}
