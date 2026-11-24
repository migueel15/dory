package api

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/mesh"
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
		writeJSON(w, http.StatusOK, s.fanOut(r.Context(), nil, r.URL.Query().Get("fresh") != ""))
	})
	mux.HandleFunc("POST /api/mesh/update", func(w http.ResponseWriter, r *http.Request) {
		var opts update.Options
		if r.ContentLength != 0 && !decode(w, r, &opts) {
			return
		}
		writeJSON(w, http.StatusOK, s.fanOut(r.Context(), &opts, false))
	})
	// The desktop app of this machine updates from the files the core left.
	desktop := http.StripPrefix("/api/mesh/desktop/", http.FileServer(http.Dir(s.Updater.DesktopDir())))
	mux.Handle("GET /api/mesh/desktop/", desktop)
}

// fanOut calls the update route of every node: this one in place, the others
// through the mesh.
func (s *Server) fanOut(ctx context.Context, opts *update.Options, fresh bool) []NodeUpdate {
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
			out[i] = s.nodeUpdate(ctx, n, opts, fresh)
		}()
	}
	wg.Wait()
	return out
}

func (s *Server) nodeUpdate(ctx context.Context, n MeshNode, opts *update.Options, fresh bool) NodeUpdate {
	res := NodeUpdate{Node: n.ID, Name: n.Name}
	var st update.Status
	var err error
	switch {
	case n.ID != s.Node.ID && opts != nil:
		st, err = s.PeerClient.StartUpdate(ctx, n.ID, *opts)
	case n.ID != s.Node.ID:
		st, err = s.PeerClient.UpdateStatus(ctx, n.ID, fresh)
	case opts != nil:
		if err = s.Updater.Start(*opts); err == nil {
			st = s.Updater.Status(ctx)
		} else {
			err = updateFailure(err)
		}
	default:
		if fresh {
			s.Updater.Forget()
		}
		st = s.Updater.Status(ctx)
	}
	if err != nil {
		switch domainErr := apperr.As(err); {
		case errors.Is(err, mesh.ErrUnreachable):
			res.Error = "el nodo no está conectado"
		case domainErr != nil:
			res.Error = domainErr.Message
		default:
			res.Error = err.Error()
		}
		return res
	}
	res.Status = &st
	return res
}

// updateFailure keeps the refusals of the update package and turns any other
// failure into update_refused; its text is written for the user.
func updateFailure(err error) error {
	if apperr.As(err) != nil {
		return err
	}
	return apperr.Wrap(apperr.UpdateRefused, err, "the update could not start").With("detail", err.Error())
}
