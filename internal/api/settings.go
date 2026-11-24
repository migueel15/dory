package api

import (
	"context"
	"fmt"
	"log"
	"maps"
	"net/http"
	"path/filepath"
	"regexp"

	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/nodestate"
	"github.com/migueel15/dory/internal/runtime"
)

// Settings are the choices of a node about how finished work is cleaned up.
// They belong to the node and are not replicated: config.toml gives the
// defaults and SQLite keeps what the user changes from the interface.
type Settings struct {
	ArchiveThreadsOnFinish  bool `json:"archive_threads_on_finish"`
	RemoveWorktreeOnArchive bool `json:"remove_worktree_on_archive"`
	StopServicesOnShutdown  bool `json:"stop_services_on_shutdown"`
	// AgentsUseDevices lets the agents of this node drive Android devices
	// through the device_* tools; on by default.
	AgentsUseDevices bool `json:"agents_use_devices"`
	// BrowserTunnelDisabled keeps the desktop app from reaching the loopback
	// ports of this node through the browser of a thread.
	BrowserTunnelDisabled bool `json:"browser_tunnel_disabled"`
	// Input groups the keyboard and dictation choices of the interface. It is
	// nil until someone saves it: every node starts with the same defaults,
	// and the web app uses it to move the choices it kept in the browser.
	Input *InputSettings `json:"input,omitempty"`
	// SourceControl and Worktrees group the version control settings.
	SourceControl SourceControlSettings `json:"source_control"`
	Worktrees     WorktreeSettings      `json:"worktrees"`
	// Threads holds the defaults for new threads of this node.
	Threads ThreadDefaults `json:"threads"`
}

// InputSettings are the shortcuts and the dictation choices of a node.
type InputSettings struct {
	// Shortcuts maps a command id to its chord; a missing command uses its default.
	Shortcuts map[string]Chord  `json:"shortcuts"`
	Dictation DictationSettings `json:"dictation"`
}

// Chord is a keyboard shortcut: a physical key plus its modifiers.
type Chord struct {
	Code  string `json:"code"`
	Ctrl  bool   `json:"ctrl"`
	Alt   bool   `json:"alt"`
	Shift bool   `json:"shift"`
	Meta  bool   `json:"meta"`
}

// DictationSettings holds the speech model a node uses; empty picks the best installed one.
type DictationSettings struct {
	Model string `json:"model"`
}

var chordCode = regexp.MustCompile(`^(Key[A-Z]|Digit[0-9])$`)

// validate rejects chords that no keyboard event can produce.
func (in *InputSettings) validate() error {
	for id, c := range in.Shortcuts {
		if !chordCode.MatchString(c.Code) || !(c.Ctrl || c.Alt || c.Meta) {
			return fmt.Errorf("invalid shortcut for %q", id)
		}
	}
	return nil
}

// SourceControlSettings are the version control choices of a node.
type SourceControlSettings struct {
	// AutoPull fast-forwards the branch of a thread from its remote while the
	// checkout has no uncommitted changes.
	AutoPull bool `json:"auto_pull"`
}

// WorktreeSettings are the worktree choices of a node.
type WorktreeSettings struct {
	// Location is the absolute folder where new worktrees are created; empty
	// means the data directory of the node.
	Location string `json:"location"`
	// BranchNaming is a free-text instruction a small model follows to name
	// the branch of a new worktree; empty keeps the generated name.
	BranchNaming string `json:"branch_naming"`
	// RemoveOnMerge deletes the clean worktree of a thread whose task pull
	// request was merged.
	RemoveOnMerge bool `json:"remove_on_merge"`
}

// ModelChoice is a provider, model and reasoning effort; empty means unset.
type ModelChoice struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Effort   string `json:"effort"`
}

// ThreadDefaults are what a new thread gets when the request does not say.
type ThreadDefaults struct {
	// DefaultModel is used for new threads; FallbackModel replaces it when the
	// selected provider is unavailable.
	DefaultModel  ModelChoice `json:"default_model"`
	FallbackModel ModelChoice `json:"fallback_model"`
	// DefaultWorkspace is "checkout" (current branch, the default) or "worktree".
	DefaultWorkspace string `json:"default_workspace"`
}

// ThreadSettings supplies the application layer with the node's current defaults.
func (st *SettingsStore) ThreadSettings() app.ThreadSettings {
	s := st.Get()
	return app.ThreadSettings{
		DefaultModel: app.ModelChoice(s.Threads.DefaultModel), FallbackModel: app.ModelChoice(s.Threads.FallbackModel),
		DefaultWorkspace: s.Threads.DefaultWorkspace, WorktreeRoot: s.Worktrees.Location, BranchNaming: s.Worktrees.BranchNaming,
		AutoPull: s.SourceControl.AutoPull, RemoveWorktreeOnArchive: s.RemoveWorktreeOnArchive,
		RemoveWorktreeOnMerge: s.Worktrees.RemoveOnMerge,
	}
}

// SettingsStore reads and saves the Settings of a node.
type SettingsStore struct {
	value *nodestate.Value[Settings]
}

// NewSettingsStore starts from defaults and applies the saved settings.
func NewSettingsStore(store *nodestate.Store, defaults Settings) *SettingsStore {
	value := nodestate.NewValue(store, nodestate.KeySettings, cloneSettings(defaults))
	if err := value.Load(context.Background()); err != nil {
		log.Printf("api: load settings: %v", err)
	}
	return &SettingsStore{value: value}
}

func cloneSettings(v Settings) Settings {
	if v.Input != nil {
		input := *v.Input
		input.Shortcuts = maps.Clone(input.Shortcuts)
		v.Input = &input
	}
	return v
}

// Get returns the current settings; without a store it returns the built-in defaults.
func (st *SettingsStore) Get() Settings {
	if st == nil {
		return Settings{RemoveWorktreeOnArchive: true}
	}
	return cloneSettings(st.value.Get())
}

// Set saves the settings.
func (st *SettingsStore) Set(v Settings) error {
	return st.value.Set(context.Background(), cloneSettings(v))
}

type NodeDefaults struct {
	runtime.Defaults
	ClaudeHome string `json:"claude_home"`
	CodexHome  string `json:"codex_home"`
}

func (s *Server) mountSettings(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/settings", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Settings.Get())
	}))
	mux.HandleFunc("GET /api/nodes/{node}/defaults", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		defaults := s.Runtime.Defaults()
		writeJSON(w, http.StatusOK, NodeDefaults{
			Defaults:   defaults,
			ClaudeHome: harness.DefaultConfigDir(defaults.Home, "claude"),
			CodexHome:  harness.DefaultConfigDir(defaults.Home, "codex"),
		})
	}))
	mux.HandleFunc("PUT /api/nodes/{node}/settings", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		v := s.Settings.Get()
		prev := v.Input
		// Decode into a fresh Input: json merges into an existing pointer and map,
		// so a removed shortcut would survive and the stored settings would be mutated.
		v.Input = nil
		if !decode(w, r, &v) {
			return
		}
		if v.Input == nil {
			v.Input = prev
		}
		if s.Settings == nil {
			writeErr(w, r, apperr.New(apperr.Unavailable, "settings are not available"))
			return
		}
		if v.Input != nil {
			if v.Input.Shortcuts == nil {
				v.Input.Shortcuts = map[string]Chord{}
			}
			if err := v.Input.validate(); err != nil {
				writeErr(w, r, apperr.New(apperr.Invalid, err.Error()))
				return
			}
		}
		if loc := v.Worktrees.Location; loc != "" && !filepath.IsAbs(loc) {
			writeErr(w, r, apperr.New(apperr.Invalid, "the worktree location must be an absolute path").With("reason", "worktree_root_not_absolute"))
			return
		}
		if err := s.Settings.Set(v); err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}))
}
