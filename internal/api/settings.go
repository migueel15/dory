package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// Settings are the choices of a node about how finished work is cleaned up.
// They belong to the node and are not replicated: config.toml gives the
// defaults and settings.json in the data directory keeps what the user
// changes from the interface.
type Settings struct {
	ArchiveThreadsOnFinish  bool `json:"archive_threads_on_finish"`
	RemoveWorktreeOnArchive bool `json:"remove_worktree_on_archive"`
	StopServicesOnShutdown  bool `json:"stop_services_on_shutdown"`
	// AgentsUseDevices lets the agents of this node drive Android devices
	// through the device_* tools; off by default.
	AgentsUseDevices bool `json:"agents_use_devices"`
	// BrowserTunnelDisabled keeps the desktop app from reaching the loopback
	// ports of this node through the browser of a thread.
	BrowserTunnelDisabled bool `json:"browser_tunnel_disabled"`
	// Input groups the keyboard and dictation choices of the interface. It is
	// nil until someone saves it: every node starts with the same defaults,
	// and the web app uses it to move the choices it kept in the browser.
	Input *InputSettings `json:"input,omitempty"`
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

// SettingsStore reads and saves the Settings of a node.
type SettingsStore struct {
	mu   sync.Mutex
	path string
	cur  Settings
}

// NewSettingsStore starts from defaults and applies the saved file in dir.
func NewSettingsStore(dir string, defaults Settings) *SettingsStore {
	st := &SettingsStore{path: filepath.Join(dir, "settings.json"), cur: defaults}
	if raw, err := os.ReadFile(st.path); err == nil {
		_ = json.Unmarshal(raw, &st.cur)
	}
	return st
}

// Get returns the current settings; without a store it returns the built-in defaults.
func (st *SettingsStore) Get() Settings {
	if st == nil {
		return Settings{RemoveWorktreeOnArchive: true}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.cur
}

// Set saves the settings.
func (st *SettingsStore) Set(v Settings) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	raw, _ := json.Marshal(v)
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, st.path); err != nil {
		return err
	}
	st.cur = v
	return nil
}

func (s *Server) mountSettings(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/settings", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Settings.Get())
	}))
	mux.HandleFunc("PUT /api/nodes/{node}/settings", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		v := s.Settings.Get()
		if !decode(w, r, &v) {
			return
		}
		if s.Settings == nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "settings are not available")
			return
		}
		if v.Input != nil {
			if v.Input.Shortcuts == nil {
				v.Input.Shortcuts = map[string]Chord{}
			}
			if err := v.Input.validate(); err != nil {
				writeError(w, http.StatusBadRequest, "invalid", err.Error())
				return
			}
		}
		if err := s.Settings.Set(v); err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}))
}
