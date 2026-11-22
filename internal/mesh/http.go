package mesh

import (
	"encoding/json"
	"net/http"
	"time"
)

// Mount adds the mesh routes of the local API: list, pair and revoke.
func (m *Mesh) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/mesh", func(w http.ResponseWriter, r *http.Request) {
		peers, err := m.Peers(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		reply(w, map[string]any{"node_id": m.log.Node(), "fingerprint": Fingerprint(m.id.PublicKey()), "port": m.port, "peers": peers, "discovered": m.Discovered()})
	})
	mux.HandleFunc("POST /api/mesh/pair/code", func(w http.ResponseWriter, r *http.Request) {
		code, exp := m.PairCode()
		reply(w, map[string]any{"code": code, "expires_at": exp.UTC().Format(time.RFC3339), "fingerprint": Fingerprint(m.id.PublicKey()), "port": m.port})
	})
	mux.HandleFunc("POST /api/mesh/pair", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Address string `json:"address"`
			Code    string `json:"code"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Address == "" || body.Code == "" {
			writeErr(w, http.StatusBadRequest, "address and code are required")
			return
		}
		p, err := m.Pair(r.Context(), body.Address, body.Code)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		reply(w, p)
	})
	mux.HandleFunc("PUT /api/mesh/peers/{id}/enabled", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			writeErr(w, http.StatusBadRequest, "enabled is required")
			return
		}
		if err := m.SetEnabled(r.Context(), r.PathValue("id"), body.Enabled); err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/mesh/peers/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := m.Revoke(r.Context(), r.PathValue("id")); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": http.StatusText(status), "message": msg}})
}
