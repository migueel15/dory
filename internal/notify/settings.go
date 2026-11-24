package notify

import (
	"context"
	"encoding/json"
	"github.com/migueel15/dory/internal/apperr"
	"net/http"
	"slices"

	"github.com/migueel15/dory/internal/data"
)

// How a kind of notice is delivered. The setting is shared by the mesh.
const (
	ModeAuto = "auto" // the delivery rule: in the app with focus, else push or system
	ModePush = "push" // always push, even with an interface in focus
	ModeApp  = "app"  // only inside the app and the inbox
	ModeOff  = "off"  // silenced
)

const settingPrefix = "notice."

// Mode returns how the kind is delivered; automatic unless set otherwise.
func (n *Notifier) Mode(ctx context.Context, kind string) string {
	s, err := n.store.Settings().Get(ctx, settingPrefix+kind)
	if err != nil || !slices.Contains([]string{ModeAuto, ModePush, ModeApp, ModeOff}, s.Value) {
		return ModeAuto
	}
	return s.Value
}

func (n *Notifier) mountSettings(mux *http.ServeMux) {
	// GET /api/notice-settings lists every kind with its text and mode.
	mux.HandleFunc("GET /api/notice-settings", func(w http.ResponseWriter, r *http.Request) {
		type item struct {
			Kind  string `json:"kind"`
			Title string `json:"title"`
			Mode  string `json:"mode"`
		}
		out := []item{}
		for _, kind := range kinds {
			out = append(out, item{kind, titles[kind], n.Mode(r.Context(), kind)})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("PUT /api/notice-settings", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Kind, Mode string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil || titles[body.Kind] == "" ||
			!slices.Contains([]string{ModeAuto, ModePush, ModeApp, ModeOff}, body.Mode) {
			apperr.WriteHTTP(w, r, apperr.New(apperr.Invalid, "invalid setting"))
			return
		}
		if _, err := n.store.Settings().Upsert(r.Context(), data.Setting{ID: settingPrefix + body.Kind, Value: body.Mode}); err != nil {
			apperr.WriteHTTP(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
