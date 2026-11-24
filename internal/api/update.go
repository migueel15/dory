package api

import (
	"context"
	"encoding/json"
	"github.com/migueel15/dory/internal/apperr"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/migueel15/dory/internal/update"
)

// MeshNode is a node of the mesh that answers now.
type MeshNode struct {
	ID   string
	Name string
}

// NodeUpdate is the update state of one node of the mesh.
type NodeUpdate struct {
	Node   string         `json:"node"`
	Name   string         `json:"name"`
	Status *update.Status `json:"status,omitempty"`
	Error  string         `json:"error,omitempty"`
}

func (s *Server) mountUpdate(mux *http.ServeMux) {
	if s.Updater == nil {
		return
	}
	mux.HandleFunc("GET /api/nodes/{node}/update", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fresh") != "" {
			s.Updater.Forget()
		}
		writeJSON(w, http.StatusOK, s.Updater.Status(r.Context()))
	}))
	mux.HandleFunc("POST /api/nodes/{node}/update", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		var opts update.Options
		if r.ContentLength != 0 && !decode(w, r, &opts) {
			return
		}
		switch err := s.Updater.Start(opts); {
		case err != nil:
			writeErr(w, r, updateFailure(err))
		default:
			writeJSON(w, http.StatusAccepted, s.Updater.Status(r.Context()))
		}
	}))
	// The mesh routes reach this node and every connected peer.
	mux.HandleFunc("GET /api/mesh/update", func(w http.ResponseWriter, r *http.Request) {
		// ?fresh=1 skips the cached check of every node.
		path := ""
		if r.URL.Query().Get("fresh") != "" {
			path = "?fresh=1"
		}
		writeJSON(w, http.StatusOK, s.fanOut(r.Context(), http.MethodGet, nil, path))
	})
	mux.HandleFunc("POST /api/mesh/update", func(w http.ResponseWriter, r *http.Request) {
		var opts update.Options
		if r.ContentLength != 0 && !decode(w, r, &opts) {
			return
		}
		body, _ := json.Marshal(opts)
		writeJSON(w, http.StatusOK, s.fanOut(r.Context(), http.MethodPost, body, ""))
	})
	// The desktop app of this machine updates from the files the core left.
	desktop := http.StripPrefix("/api/mesh/desktop/", http.FileServer(http.Dir(s.Updater.DesktopDir())))
	mux.Handle("GET /api/mesh/desktop/", desktop)
}

// fanOut calls the update route of every node: this one in place, the others
// through the mesh.
func (s *Server) fanOut(ctx context.Context, method string, body []byte, query string) []NodeUpdate {
	nodes := []MeshNode{{ID: s.Node.ID, Name: s.Node.Name}}
	if s.Mesh != nil {
		nodes = append(nodes, s.Mesh(ctx)...)
	}
	out := make([]NodeUpdate, len(nodes))
	var wg sync.WaitGroup
	for i, n := range nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = s.nodeUpdate(ctx, n, method, body, query)
		}()
	}
	wg.Wait()
	return out
}

func (s *Server) nodeUpdate(ctx context.Context, n MeshNode, method string, body []byte, query string) NodeUpdate {
	res := NodeUpdate{Node: n.ID, Name: n.Name}
	path := "/api/nodes/" + n.ID + "/update" + query
	req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(string(body)))
	if len(body) == 0 {
		req.ContentLength = 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("node", n.ID)
	rec := httptest.NewRecorder()
	if n.ID == s.Node.ID {
		s.local(rec, req, method)
	} else if s.Remote == nil || !s.Remote.Forward(rec, req, n.ID) {
		res.Error = "el nodo no está conectado"
		return res
	}
	if rec.Code >= 300 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		res.Error = e.Error.Message
		if res.Error == "" {
			res.Error = strings.TrimSpace(rec.Body.String())
		}
		return res
	}
	var st update.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		res.Error = "respuesta no válida: " + err.Error()
		return res
	}
	res.Status = &st
	return res
}

func (s *Server) local(w http.ResponseWriter, r *http.Request, method string) {
	if method == http.MethodGet {
		if r.URL.Query().Get("fresh") != "" {
			s.Updater.Forget()
		}
		writeJSON(w, http.StatusOK, s.Updater.Status(r.Context()))
		return
	}
	var opts update.Options
	_ = json.NewDecoder(r.Body).Decode(&opts)
	if err := s.Updater.Start(opts); err != nil {
		writeErr(w, r, updateFailure(err))
		return
	}
	writeJSON(w, http.StatusAccepted, s.Updater.Status(r.Context()))
}

// updateFailure keeps the refusals of the update package and turns any other
// failure into update_refused; its text is written for the user.
func updateFailure(err error) error {
	if apperr.As(err) != nil {
		return err
	}
	return apperr.Wrap(apperr.UpdateRefused, err, "the update could not start").With("detail", err.Error())
}
