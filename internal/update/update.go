// Package update moves this node to the latest release of its channel: the
// core binary, the desktop app when the machine has it, and optionally the
// harnesses. Releases are public on GitHub and are read over plain HTTPS,
// without credentials (ADR-016).
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/version"
)

// Repo publishes the releases.
const Repo = "migueel15/dory"

// checkTTL keeps the last read of the latest release; each read asks GitHub.
const checkTTL = 5 * time.Minute

// States of a node update.
const (
	StateIdle     = "idle"
	StateUpdating = "updating"
	StateFailed   = "failed"
)

var (
	// ErrDev is returned for local builds, which follow no channel.
	ErrDev = errors.New("esta compilación es local y no sigue ningún canal")
	// ErrBusy is returned when turns run here and the update is not forced.
	ErrBusy = errors.New("hay turnos en marcha en este nodo")
	// ErrRunning is returned while an update is already in progress.
	ErrRunning = errors.New("ya se está actualizando")
)

// Status is what this node runs and what its channel offers.
type Status struct {
	Channel   string `json:"channel"`
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`
	// Desktop is true when the desktop app is installed on this machine.
	Desktop     bool   `json:"desktop"`
	ActiveTurns int    `json:"active_turns"`
	State       string `json:"state"`
	Error       string `json:"error,omitempty"`
	// Providers are the harnesses of this node and the release npm offers.
	Providers []ProviderVersion `json:"providers"`
}

// Pending reports whether the core or any harness has a newer release.
func (s Status) Pending() bool {
	if s.Available {
		return true
	}
	for _, p := range s.Providers {
		if p.Available {
			return true
		}
	}
	return false
}

// Options of an update.
type Options struct {
	// Providers also updates the installed harnesses.
	Providers bool `json:"providers"`
	// Force updates even with turns in progress, which the restart cuts.
	Force bool `json:"force"`
}

// Updater updates this node.
type Updater struct {
	// DataDir holds the downloads; the desktop app reads its files from there.
	DataDir string
	// Active counts the turns in progress here.
	Active func() int
	// Providers updates the harnesses and returns the ones that failed.
	Providers func(ctx context.Context) map[string]string
	// Installed lists the harnesses installed here with their versions.
	Installed func(ctx context.Context) []Installed
	// Restart starts the new binary once it is in place.
	Restart func()

	mu      sync.Mutex
	state   string
	err     string
	latest  string
	tag     string
	checked time.Time
}

// DesktopDir is where the files for the desktop updater are left.
func (u *Updater) DesktopDir() string { return filepath.Join(u.DataDir, "updates", "desktop") }

// Status reads the current state; it asks GitHub at most every few minutes.
func (u *Updater) Status(ctx context.Context) Status {
	info := version.Read()
	st := Status{Channel: info.Channel, Current: info.Short, Desktop: desktopInstalled(), ActiveTurns: u.Active()}
	u.mu.Lock()
	st.State, st.Error = u.state, u.err
	if st.State == "" {
		st.State = StateIdle
	}
	fresh := time.Since(u.checked) < checkTTL
	u.mu.Unlock()
	st.Providers = []ProviderVersion{}
	if u.Installed != nil {
		st.Providers = providerVersions(ctx, u.Installed(ctx))
	}
	if info.Channel == "dev" {
		return st
	}
	if !fresh {
		tag, sha, err := latest(ctx, info.Channel)
		u.mu.Lock()
		if err != nil {
			if st.Error == "" {
				st.Error = err.Error()
			}
		} else {
			u.tag, u.latest, u.checked = tag, sha, time.Now()
		}
		u.mu.Unlock()
	}
	u.mu.Lock()
	if u.latest != "" {
		st.Latest = short(u.latest)
		st.Available = u.latest != info.Revision
	}
	u.mu.Unlock()
	return st
}

// Forget drops the cached latest releases of the core and the harnesses, so
// the next Status asks again.
func (u *Updater) Forget() {
	u.mu.Lock()
	u.checked = time.Time{}
	u.mu.Unlock()
	npmCache.Lock()
	npmCache.at = map[string]time.Time{}
	npmCache.Unlock()
}

// Start begins an update in the background.
func (u *Updater) Start(opts Options) error {
	// A local build can still update its harnesses.
	if version.Channel == "dev" && !opts.Providers {
		return ErrDev
	}
	if !opts.Force && u.Active() > 0 {
		return ErrBusy
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.state == StateUpdating {
		return ErrRunning
	}
	u.state, u.err = StateUpdating, ""
	go func() {
		err := u.run(context.Background(), opts)
		u.mu.Lock()
		defer u.mu.Unlock()
		if err != nil {
			log.Printf("update: %v", err)
			u.state, u.err = StateFailed, err.Error()
			return
		}
		// The restart replaces this process; the state only shows if it did not.
		u.state = StateIdle
		u.checked = time.Time{}
	}()
	return nil
}

// providerError turns the harnesses that failed into the update error.
func providerError(notes []string) error {
	if len(notes) == 0 {
		return nil
	}
	return errors.New("no se actualizaron: " + strings.Join(notes, "; "))
}

// Apply updates this node and waits; the CLI uses it.
func (u *Updater) Apply(ctx context.Context, opts Options) error {
	if version.Channel == "dev" {
		return ErrDev
	}
	return u.run(ctx, opts)
}

func (u *Updater) run(ctx context.Context, opts Options) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	var notes []string
	if opts.Providers && u.Providers != nil {
		for id, msg := range u.Providers(ctx) {
			notes = append(notes, fmt.Sprintf("%s: %s", id, msg))
		}
	}
	// The core only moves when its channel has another commit.
	if version.Channel == "dev" {
		return providerError(notes)
	}
	tag, sha, err := latest(ctx, version.Channel)
	if err != nil {
		return err
	}
	if sha == version.Read().Revision {
		return providerError(notes)
	}
	dir := filepath.Join(u.DataDir, "updates")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	asset := fmt.Sprintf("dory-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		asset += ".exe"
	}
	if err = download(ctx, tag, dir, asset); err != nil {
		return err
	}
	if desktopInstalled() {
		desk := u.DesktopDir()
		stage, stageErr := os.MkdirTemp(dir, "desktop-")
		if stageErr != nil {
			return stageErr
		}
		defer os.RemoveAll(stage)
		if err = download(ctx, tag, stage, desktopAssets()...); err != nil {
			return err
		}
		backup := desk + ".previous"
		if err = os.RemoveAll(backup); err != nil {
			return err
		}
		if err = os.Rename(desk, backup); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err = os.Rename(stage, desk); err != nil {
			_ = os.Rename(backup, desk)
			return err
		}
		_ = os.RemoveAll(backup)
	}
	// The core bundled inside an AppImage is read-only; the desktop update
	// brings it instead.
	if err = replace(filepath.Join(dir, asset)); err != nil && !desktopInstalled() {
		return err
	}
	if len(notes) > 0 {
		log.Printf("update: harnesses that failed: %s", strings.Join(notes, "; "))
	}
	if u.Restart != nil {
		u.Restart()
	}
	return nil
}

// api is the GitHub REST API; unauthenticated calls allow 60 an hour per
// address, and a node makes at most one check every few minutes.
var api = "https://api.github.com/repos/" + Repo

var client = &http.Client{Timeout: 10 * time.Minute}

// latest resolves one platform's immutable release. Older installations use
// the complete legacy release to reach a build that understands this format.
func latest(ctx context.Context, channel string) (tag, sha string, err error) {
	var manifest struct {
		Tag string `json:"tag"`
		SHA string `json:"sha"`
	}
	branch := "updates-" + channel + "-" + runtime.GOOS
	if err = getJSON(ctx, api+"/contents/manifest.json?ref="+url.QueryEscape(branch), &manifestFile{target: &manifest}); err == nil {
		if manifest.Tag == "" || manifest.SHA == "" {
			return "", "", errors.New("manifiesto de actualización incompleto")
		}
		return manifest.Tag, manifest.SHA, nil
	}
	return "", "", fmt.Errorf("no se pudo consultar la publicación de %s: %w", runtime.GOOS, err)
}

// GitHub's contents API returns the file encoded in base64.
type manifestFile struct{ target any }

func (m *manifestFile) UnmarshalJSON(data []byte) error {
	var file struct {
		Content []byte `json:"content"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	return json.Unmarshal(file.Content, m.target)
}

// download saves the assets of a release whose names match the patterns.
func download(ctx context.Context, tag, dir string, patterns ...string) error {
	var rel struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := getJSON(ctx, api+"/releases/tags/"+url.PathEscape(tag), &rel); err != nil {
		return err
	}
	for _, p := range patterns {
		found := false
		for _, a := range rel.Assets {
			if ok, _ := filepath.Match(p, a.Name); ok {
				if err := fetch(ctx, a.URL, filepath.Join(dir, a.Name)); err != nil {
					return err
				}
				found = true
			}
		}
		if !found {
			return fmt.Errorf("la release %s no tiene %s", tag, p)
		}
	}
	return nil
}

var errNotFound = errors.New("no encontrado")

func get(ctx context.Context, target string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "dory-updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("GET %s: %w", target, errNotFound)
		}
		return nil, fmt.Errorf("GET %s: %s", target, resp.Status)
	}
	return resp, nil
}

func getJSON(ctx context.Context, target string, out any) error {
	resp, err := get(ctx, target)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

// fetch writes a file through a temporary name, so a cut download never
// replaces a good one.
func fetch(ctx context.Context, target, path string) error {
	resp, err := get(ctx, target)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err = io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// desktopAssets are the files electron-updater needs on this system.
func desktopAssets() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"*Setup*.exe", "*Setup*.exe.blockmap", "latest.yml"}
	case "darwin":
		return []string{"*-mac.zip", "*-mac.zip.blockmap", "latest-mac.yml"}
	}
	return []string{"*.AppImage", "latest-linux.yml"}
}

// desktopInstalled reports whether the desktop app has run on this machine:
// it keeps its profile in ~/.dory/desktop.
func desktopInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".dory", "desktop"))
	return err == nil
}

// replace puts the downloaded binary in place of the running one. Windows
// cannot overwrite a running executable, but it can rename it.
func replace(next string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return err
	}
	raw, err := os.ReadFile(next)
	if err != nil {
		return err
	}
	tmp := self + ".new"
	if err = os.WriteFile(tmp, raw, 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		old := self + ".old"
		os.Remove(old)
		if err = os.Rename(self, old); err != nil {
			return err
		}
	}
	return os.Rename(tmp, self)
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
