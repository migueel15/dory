package update

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// Installed is a harness installed on this node and the version it reports.
type Installed struct {
	ID      string
	Name    string
	Version string
}

// ProviderVersion is a harness of this node and the release npm offers.
type ProviderVersion struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`
}

// packages are the npm packages the harnesses publish their releases in.
var packages = map[string]string{
	"claude":   "@anthropic-ai/claude-code",
	"codex":    "@openai/codex",
	"opencode": "opencode-ai",
}

var registry = "https://registry.npmjs.org/"

var semver = regexp.MustCompile(`\d+\.\d+\.\d+`)

// npmCache keeps the latest version of each package for checkTTL.
var npmCache = struct {
	sync.Mutex
	at     map[string]time.Time
	latest map[string]string
}{at: map[string]time.Time{}, latest: map[string]string{}}

// providerVersions compares each installed harness with its latest release.
func providerVersions(ctx context.Context, list []Installed) []ProviderVersion {
	out := []ProviderVersion{}
	for _, p := range list {
		pkg, ok := packages[p.ID]
		if !ok {
			continue
		}
		v := ProviderVersion{ID: p.ID, Name: p.Name, Current: semver.FindString(p.Version)}
		v.Latest = npmLatest(ctx, pkg)
		v.Available = v.Current != "" && v.Latest != "" && newer(v.Latest, v.Current)
		out = append(out, v)
	}
	return out
}

func npmLatest(ctx context.Context, pkg string) string {
	npmCache.Lock()
	if time.Since(npmCache.at[pkg]) < checkTTL {
		v := npmCache.latest[pkg]
		npmCache.Unlock()
		return v
	}
	npmCache.Unlock()
	var dist struct {
		Version string `json:"version"`
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if getJSON(ctx, registry+url.PathEscape(pkg)+"/latest", &dist) != nil {
		return ""
	}
	npmCache.Lock()
	npmCache.at[pkg], npmCache.latest[pkg] = time.Now(), dist.Version
	npmCache.Unlock()
	return dist.Version
}

// newer reports whether version a is above b; both are x.y.z.
func newer(a, b string) bool {
	pa, pb := semver.FindString(a), semver.FindString(b)
	na, nb := regexp.MustCompile(`\.`).Split(pa, 3), regexp.MustCompile(`\.`).Split(pb, 3)
	if len(na) != 3 || len(nb) != 3 {
		return false
	}
	for i := range 3 {
		x, _ := strconv.Atoi(na[i])
		y, _ := strconv.Atoi(nb[i])
		if x != y {
			return x > y
		}
	}
	return false
}
