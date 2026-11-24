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
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/i18n"
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
	ErrDev = apperr.New(apperr.UpdateRefused, "this build is local and follows no channel").With("reason", "dev")
	// ErrBusy is returned when turns run here and the update is not forced.
	ErrBusy = apperr.New(apperr.UpdateRefused, "turns are running on this node").With("reason", "busy")
	// ErrRunning is returned while an update is already in progress.
	ErrRunning = apperr.New(apperr.UpdateRefused, "an update is already running").With("reason", "running")
)

// Step is one stage of an update in progress.
type Step struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status"`
}

// Progress reports where an active update is.
type Progress struct {
	Percent int    `json:"percent"`
	Steps   []Step `json:"steps"`
}

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
	// Progress reports the current step and percentage while State is updating.
	Progress *Progress `json:"progress,omitempty"`
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

	mu       sync.Mutex
	state    string
	err      string
	latest   string
	tag      string
	checked  time.Time
	progress *Progress
}

func (u *Updater) setProgress(percent int, steps []Step) {
	u.mu.Lock()
	defer u.mu.Unlock()
	cp := make([]Step, len(steps))
	copy(cp, steps)
	u.progress = &Progress{Percent: percent, Steps: cp}
}

func (u *Updater) updateStep(id, status, detail string, percent int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.progress == nil {
		return
	}
	if percent >= 0 {
		u.progress.Percent = percent
	}
	for i := range u.progress.Steps {
		if u.progress.Steps[i].ID == id {
			u.progress.Steps[i].Status = status
			if detail != "" {
				u.progress.Steps[i].Detail = detail
			}
			break
		}
	}
}

func (u *Updater) snapshotProgress() *Progress {
	if u.progress == nil {
		return nil
	}
	cp := make([]Step, len(u.progress.Steps))
	copy(cp, u.progress.Steps)
	return &Progress{Percent: u.progress.Percent, Steps: cp}
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
	if st.State == StateUpdating {
		st.Progress = u.snapshotProgress()
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
			u.state, u.err, u.progress = StateFailed, err.Error(), nil
			return
		}
		// The restart replaces this process; the state only shows if it did not.
		u.state = StateIdle
		u.progress = nil
		u.checked = time.Time{}
	}()
	return nil
}

// providerError turns the harnesses that failed into the update error.
func providerError(notes []string) error {
	if len(notes) == 0 {
		return nil
	}
	return errors.New(i18n.Textf("update.providers_failed", strings.Join(notes, "; ")))
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

	inApp := os.Getenv(DesktopEnv) != ""
	hasCore := version.Channel != "dev"
	hasDesktop := inApp || desktopInstalled()

	var steps []Step
	if opts.Providers && u.Providers != nil {
		steps = append(steps, Step{ID: "providers", Label: "Actualizando harnesses", Status: "pending"})
	}
	if hasCore {
		steps = append(steps, Step{ID: "check", Label: "Comprobando actualización", Status: "pending"})
		steps = append(steps, Step{ID: "download", Label: "Descargando actualización", Status: "pending"})
		if hasDesktop {
			steps = append(steps, Step{ID: "stage", Label: "Preparando archivos", Status: "pending"})
			steps = append(steps, Step{ID: "verify", Label: "Verificando paquete", Status: "pending"})
		}
		steps = append(steps, Step{ID: "restart", Label: "Reiniciando Dory", Status: "pending"})
	}
	u.setProgress(0, steps)

	var notes []string
	if opts.Providers && u.Providers != nil {
		u.updateStep("providers", "active", "", 2)
		for id, msg := range u.Providers(ctx) {
			notes = append(notes, fmt.Sprintf("%s: %s", id, msg))
		}
		u.updateStep("providers", "done", "", 15)
	}
	// The core only moves when its channel has another commit.
	if !hasCore {
		return providerError(notes)
	}
	u.updateStep("check", "active", "", 18)
	tag, sha, err := latest(ctx, version.Channel)
	if err != nil {
		return err
	}
	info := version.Read()
	if sha == info.Revision {
		u.updateStep("check", "done", info.Short, 100)
		return providerError(notes)
	}
	u.updateStep("check", "done", fmt.Sprintf("Dory %s → %s", info.Short, short(sha)), 20)

	dir := filepath.Join(u.DataDir, "updates")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	asset := fmt.Sprintf("dory-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		asset += ".exe"
	}

	dlStart, dlEnd := 20, 75
	if !hasDesktop {
		dlEnd = 90
	}
	onDownload := func(done, total int64) {
		if total <= 0 {
			return
		}
		frac := float64(done) / float64(total)
		if frac > 1 {
			frac = 1
		}
		pct := dlStart + int(frac*float64(dlEnd-dlStart))
		detail := fmt.Sprintf("%.1f / %.1f MB · %d%%", float64(done)/(1024*1024), float64(total)/(1024*1024), int(frac*100))
		u.updateStep("download", "active", detail, pct)
	}

	u.updateStep("download", "active", "", dlStart)
	if !inApp {
		if err = downloadWithProgress(ctx, tag, dir, onDownload, asset); err != nil {
			return err
		}
	}
	if hasDesktop {
		desk := u.DesktopDir()
		stage, stageErr := os.MkdirTemp(dir, "desktop-")
		if stageErr != nil {
			return stageErr
		}
		defer os.RemoveAll(stage)
		if err = downloadWithProgress(ctx, tag, stage, onDownload, desktopAssets()...); err != nil {
			return err
		}
		u.updateStep("download", "done", "", dlEnd)
		u.updateStep("stage", "active", "", dlEnd+2)
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
		u.updateStep("stage", "done", "", 82)
	} else {
		u.updateStep("download", "done", "", dlEnd)
	}
	// The core the desktop app runs lives inside the app: the desktop update
	// brings it, and the app restarts with it.
	if inApp {
		u.updateStep("verify", "active", "", 84)
		return providerError(notes)
	}
	u.updateStep("restart", "active", "Aplicando cambios…", 95)
	if err = replaceCore(filepath.Join(dir, asset)); err != nil {
		return fmt.Errorf(i18n.Text("update.no_pudo_sustituir_nucleo"), err)
	}
	if len(notes) > 0 {
		log.Printf("update: harnesses that failed: %s", strings.Join(notes, "; "))
	}
	if u.Restart != nil {
		u.Restart()
	}
	return nil
}

// DesktopEnv is set by the desktop app on the core it runs as its child.
const DesktopEnv = "DORY_DESKTOP_CORE"

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
			return "", "", errors.New(i18n.Text("update.manifiesto_actualizacion_incompleto"))
		}
		return manifest.Tag, manifest.SHA, nil
	}
	return "", "", fmt.Errorf(i18n.Text("update.no_pudo_consultar_publicacion"), runtime.GOOS, err)
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
	return downloadWithProgress(ctx, tag, dir, nil, patterns...)
}

func downloadWithProgress(ctx context.Context, tag, dir string, onProgress func(done, total int64), patterns ...string) error {
	var rel struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := getJSON(ctx, api+"/releases/tags/"+url.PathEscape(tag), &rel); err != nil {
		return err
	}
	type matched struct {
		name string
		url  string
		size int64
	}
	var files []matched
	var total int64
	for _, p := range patterns {
		found := false
		for _, a := range rel.Assets {
			if ok, _ := filepath.Match(p, a.Name); ok {
				files = append(files, matched{name: a.Name, url: a.URL, size: a.Size})
				total += a.Size
				found = true
			}
		}
		if !found {
			return fmt.Errorf(i18n.Text("update.release_no_tiene"), tag, p)
		}
	}
	var done int64
	for _, f := range files {
		base := done
		err := fetchWithProgress(ctx, f.url, filepath.Join(dir, f.name), func(n, contentLen int64) {
			if onProgress == nil {
				return
			}
			t := total
			if t <= 0 && contentLen > 0 {
				t = base + contentLen
			}
			onProgress(base+n, t)
		})
		if err != nil {
			return err
		}
		done += f.size
	}
	return nil
}

var errNotFound = errors.New("not found")

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
	return fetchWithProgress(ctx, target, path, nil)
}

func fetchWithProgress(ctx context.Context, target, path string, onProgress func(done, total int64)) error {
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
	var src io.Reader = resp.Body
	if onProgress != nil {
		src = &progressReader{r: resp.Body, total: resp.ContentLength, onProgress: onProgress}
	}
	if _, err = io.Copy(f, src); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type progressReader struct {
	r          io.Reader
	done       int64
	total      int64
	lastReport time.Time
	onProgress func(done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.done += int64(n)
		if now := time.Now(); err == io.EOF || now.Sub(p.lastReport) >= 150*time.Millisecond {
			p.lastReport = now
			p.onProgress(p.done, p.total)
		}
	}
	return n, err
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

// replaceCore is replace, swapped in tests.
var replaceCore = replace

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
