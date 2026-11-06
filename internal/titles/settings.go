package titles

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultTimeout applies when the settings do not set one.
const DefaultTimeout = 2 * time.Minute

// Settings is what the web stores: where the thread runs and with which model.
type Settings struct {
	Enabled        bool   `json:"enabled"`
	InstanceID     string `json:"instance_id"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Effort         string `json:"effort"`
	Project        string `json:"project"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// Config turns the stored settings into a generation config. A disabled
// generation, or one without a model, has an empty Model.
func (s Settings) Config() Config {
	c := Config{Instance: s.InstanceID, Provider: s.Provider, Model: s.Model, Effort: s.Effort, Project: s.Project, Timeout: time.Duration(s.TimeoutSeconds) * time.Second}
	if !s.Enabled {
		c.Model = ""
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	return c
}

func (s Settings) valid() bool {
	return s.TimeoutSeconds >= 0 && s.TimeoutSeconds <= 3600 && (!s.Enabled || (s.InstanceID != "" && s.Model != ""))
}

// Store keeps the settings in the title_settings table.
type Store struct{ Pool *pgxpool.Pool }

// Load returns the stored settings; nothing stored means off.
func (s Store) Load(ctx context.Context) (Settings, error) {
	var raw []byte
	err := s.Pool.QueryRow(ctx, "SELECT settings FROM title_settings WHERE id").Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{TimeoutSeconds: int(DefaultTimeout.Seconds())}, nil
	}
	var out Settings
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	if out.TimeoutSeconds == 0 {
		out.TimeoutSeconds = int(DefaultTimeout.Seconds())
	}
	return out, err
}

// Save replaces the settings.
func (s Store) Save(ctx context.Context, in Settings) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, "INSERT INTO title_settings (id, settings) VALUES (true, $1) ON CONFLICT (id) DO UPDATE SET settings = excluded.settings", raw)
	return err
}

// Mount serves GET and PUT /api/settings/titles.
func (s Store) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/titles", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.Load(r.Context())
		if err != nil {
			http.Error(w, "settings unavailable", http.StatusInternalServerError)
			return
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("PUT /api/settings/titles", func(w http.ResponseWriter, r *http.Request) {
		var in Settings
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || !in.valid() {
			http.Error(w, "invalid settings: enabled needs instance_id and model; timeout_seconds 0-3600", http.StatusBadRequest)
			return
		}
		if err := s.Save(r.Context(), in); err != nil {
			http.Error(w, "settings unavailable", http.StatusInternalServerError)
			return
		}
		writeJSON(w, in)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
