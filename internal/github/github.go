// Package github reads issues and pull requests of the registered repositories.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/migueel15/dory/internal/apperr"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	apiBase  = "https://api.github.com"
	cacheTTL = time.Minute
	// hotTTL applies while a pull request has checks running.
	hotTTL   = 15 * time.Second
	rulesTTL = 10 * time.Minute
	// refreshTimeout bounds a background refresh of all repositories.
	refreshTimeout = 2 * time.Minute
	// maxDiff keeps huge pull requests from filling the browser.
	maxDiff = 2 << 20
)

var (
	ErrUnconfigured = apperr.New(apperr.Unavailable, "github: not logged in")
	ErrUnknownRepo  = apperr.New(apperr.NotFound, "github: repository is not registered")
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
	hot        bool
	viewer     string

	// watched keeps the pull list ETag and fingerprint of each repository.
	watched map[string]watchEntry
	// ciChecked tracks when each repo with pending CI was last refreshed.
	ciChecked map[string]time.Time
	// initialSynced records whether the watcher has run its first startup sync.
	initialSynced bool
	// beat is when a client last showed a view with branches or pulls.
	beat time.Time
	kick chan struct{}
}

// New follows the repositories that repos returns, as remote URL -> project id.
func New(repos func(context.Context) (map[string]string, error), auth *Auth) *Client {
	c := &Client{Auth: auth, Base: apiBase, HTTP: &http.Client{Timeout: 15 * time.Second}, listRepos: repos, byRepo: map[string]repoData{}, rules: map[string]ruleEntry{}, watched: map[string]watchEntry{}, ciChecked: map[string]time.Time{}, kick: make(chan struct{}, 1)}
	if auth != nil {
		auth.OnChange = c.reset
	}
	return c
}

// reset drops what the previous account saw.
func (c *Client) reset() {
	c.mu.Lock()
	c.byRepo, c.rules, c.fetched, c.viewer, c.watched, c.ciChecked, c.initialSynced = map[string]repoData{}, map[string]ruleEntry{}, time.Time{}, "", map[string]watchEntry{}, map[string]time.Time{}, false
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
	r, err := c.getTagged(ctx, path, accept, "")
	return r.body, err
}

// tagged is a GET answer: NotModified means the ETag still matches, which
// GitHub does not charge against the rate limit.
type tagged struct {
	body        []byte
	etag        string
	notModified bool
	retryAfter  time.Duration
}

// getTagged sends If-None-Match when etag is set.
func (c *Client) getTagged(ctx context.Context, path, accept, etag string) (tagged, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return tagged{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token(ctx))
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return tagged{}, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		return tagged{etag: etag, notModified: true}, nil
	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiff))
		return tagged{body: body, etag: resp.Header.Get("ETag")}, err
	case http.StatusForbidden, http.StatusTooManyRequests:
		wait := time.Minute
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			wait = time.Duration(s) * time.Second
		}
		return tagged{retryAfter: wait}, fmt.Errorf("github: %s returned %d", path, resp.StatusCode)
	}
	c.unauthorized(resp.StatusCode)
	return tagged{}, fmt.Errorf("github: %s returned %d", path, resp.StatusCode)
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
