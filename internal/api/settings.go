package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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
	// Threads holds the defaults for new threads of this node.
	Threads ThreadDefaults `json:"threads"`
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
	// default cannot start (provider missing, not logged in, error at start).
	DefaultModel  ModelChoice `json:"default_model"`
	FallbackModel ModelChoice `json:"fallback_model"`
	// DefaultWorkspace is "checkout" (current branch, the default) or "worktree".
	DefaultWorkspace string `json:"default_workspace"`
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
		if err := s.Settings.Set(v); err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}))
}
