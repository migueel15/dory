// Package update moves this node to the latest release of its channel: the
// core binary, the desktop app when the machine has it, and optionally the
// harnesses. Releases live in a private repository, so downloads go through
// the GitHub CLI, which carries the user's credentials (ADR-016).
package update

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
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

// Start begins an update in the background.
func (u *Updater) Start(opts Options) error {
	if version.Channel == "dev" {
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

func (u *Updater) run(ctx context.Context, opts Options) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	var notes []string
	if opts.Providers && u.Providers != nil {
		for id, msg := range u.Providers(ctx) {
			notes = append(notes, fmt.Sprintf("%s: %s", id, msg))
		}
	}
	tag, _, err := latest(ctx, version.Channel)
	if err != nil {
		return err
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
		if err = os.MkdirAll(desk, 0o700); err != nil {
			return err
		}
		if err = download(ctx, tag, desk, desktopAssets()...); err != nil {
			return err
		}
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

// latest resolves the tag of a channel and the commit it points to.
func latest(ctx context.Context, channel string) (tag, sha string, err error) {
	tag = "nightly"
	if channel == "stable" {
		if tag, err = gh(ctx, "release", "view", "--repo", Repo, "--json", "tagName", "-q", ".tagName"); err != nil {
			return "", "", err
		}
	}
	sha, err = gh(ctx, "api", "repos/"+Repo+"/commits/"+tag, "-q", ".sha")
	return tag, sha, err
}

func download(ctx context.Context, tag, dir string, patterns ...string) error {
	args := []string{"release", "download", tag, "--repo", Repo, "--dir", dir, "--clobber"}
	for _, p := range patterns {
		args = append(args, "--pattern", p)
	}
	_, err := gh(ctx, args...)
	return err
}

func gh(ctx context.Context, args ...string) (string, error) {
	run, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(run, "gh", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh %s: %s", args[0], strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// desktopAssets are the files electron-updater needs on this system.
func desktopAssets() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"*Setup*.exe", "*Setup*.exe.blockmap", "*.yml"}
	case "darwin":
		return []string{"*-mac.zip", "*-mac.zip.blockmap", "*.yml"}
	}
	return []string{"*.AppImage", "*.yml"}
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
