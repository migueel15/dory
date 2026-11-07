// Package github reads issues and pull requests of the registered repositories.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/service"
)

const (
	apiBase  = "https://api.github.com"
	cacheTTL = time.Minute
	rulesTTL = 10 * time.Minute
	// refreshTimeout bounds a background refresh of all repositories.
	refreshTimeout = 2 * time.Minute
	// maxDiff keeps huge pull requests from filling the browser.
	maxDiff = 2 << 20
)

var (
	ErrUnconfigured = errors.New("github: DORY_GITHUB_TOKEN is not set")
	ErrUnknownRepo  = errors.New("github: repository is not registered")
	remotePattern   = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`)
)

// ParseRemote extracts owner/name from a GitHub remote URL.
func ParseRemote(remote string) (string, bool) {
	m := remotePattern.FindStringSubmatch(remote)
	if m == nil {
		return "", false
	}
	return m[1] + "/" + m[2], true
}

// Client caches the overview per repository because it costs one query each.
type Client struct {
	Token string
	Base  string
	HTTP  *http.Client
	svc   service.Service

	mu         sync.Mutex
	byRepo     map[string]repoData
	rules      map[string]ruleEntry
	fetched    time.Time
	refreshing bool
	viewer     string
}

func New(svc service.Service, token string) *Client {
	return &Client{Token: token, Base: apiBase, HTTP: &http.Client{Timeout: 15 * time.Second}, svc: svc, byRepo: map[string]repoData{}, rules: map[string]ruleEntry{}}
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
	// Repositories of an archived project stay stored but are no longer followed.
	projects, err := c.svc.Projects().List(ctx, service.Filter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	active := make(map[string]bool, len(projects))
	for _, p := range projects {
		active[p.ID] = true
	}
	out := make(map[string]string, len(repos))
	for _, r := range repos {
		if !active[r.ProjectID] {
			continue
		}
		if name, ok := ParseRemote(r.RemoteUrl); ok {
			out[name] = r.ProjectID
		}
	}
	return out, nil
}

func sortByUpdated[T any](s []T, at func(T) string) {
	slices.SortFunc(s, func(a, b T) int { return strings.Compare(at(b), at(a)) })
}

// graphql runs a query and reports GitHub errors, whose messages explain
// failures such as a merge blocked by branch protection.
func (c *Client) graphql(ctx context.Context, query string, vars map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/graphql", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github: graphql returned %d", resp.StatusCode)
	}
	var result struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("github: %s", result.Errors[0].Message)
	}
	return json.Unmarshal(result.Data, out)
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
