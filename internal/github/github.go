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
	ErrUnconfigured = errors.New("github: not logged in")
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
	Auth *Auth
	Base string
	HTTP *http.Client
	// repos lists the followed repositories as owner/name -> project id.
	listRepos func(context.Context) (map[string]string, error)

	mu         sync.Mutex
	byRepo     map[string]repoData
	rules      map[string]ruleEntry
	fetched    time.Time
	refreshing bool
	viewer     string
}

// New follows the repositories that repos returns, as remote URL -> project id.
func New(repos func(context.Context) (map[string]string, error), auth *Auth) *Client {
	c := &Client{Auth: auth, Base: apiBase, HTTP: &http.Client{Timeout: 15 * time.Second}, listRepos: repos, byRepo: map[string]repoData{}, rules: map[string]ruleEntry{}}
	if auth != nil {
		auth.OnChange = c.reset
	}
	return c
}

// reset drops what the previous account saw.
func (c *Client) reset() {
	c.mu.Lock()
	c.byRepo, c.rules, c.fetched, c.viewer = map[string]repoData{}, map[string]ruleEntry{}, time.Time{}, ""
	c.mu.Unlock()
}

// token returns the current token, empty when no source has one.
func (c *Client) token(ctx context.Context) string {
	t, _ := c.Auth.Token(ctx)
	return t
}

// Configured reports whether a token is available.
func (c *Client) Configured(ctx context.Context) bool { return c.token(ctx) != "" }

// unauthorized lets the next request resolve the token again.
func (c *Client) unauthorized(code int) {
	if code == http.StatusUnauthorized {
		c.Auth.Invalidate()
	}
}

// Status reports where the token comes from and whose it is.
func (c *Client) Status(ctx context.Context) (source, login string) {
	token, source := c.Auth.Token(ctx)
	if token == "" {
		return source, ""
	}
	body, err := c.get(ctx, "/user", "application/vnd.github+json")
	if err != nil {
		return source, ""
	}
	var u struct {
		Login string `json:"login"`
	}
	_ = json.Unmarshal(body, &u)
	return source, u.Login
}

func (c *Client) get(ctx context.Context, path, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token(ctx))
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.unauthorized(resp.StatusCode)
		return nil, fmt.Errorf("github: %s returned %d", path, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxDiff))
}

func (c *Client) repos(ctx context.Context) (map[string]string, error) {
	remotes, err := c.listRepos(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(remotes))
	for remote, project := range remotes {
		if name, ok := ParseRemote(remote); ok {
			out[name] = project
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
	req.Header.Set("Authorization", "Bearer "+c.token(ctx))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.unauthorized(resp.StatusCode)
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
	if !c.Configured(ctx) {
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
