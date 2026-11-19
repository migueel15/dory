// Package api owns the REST routes of a Dory v2 node.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/migueel15/dory/internal/update"

	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/dictation"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/runtime"
	"github.com/migueel15/dory/internal/version"
)

const maxBody = 1 << 20

// Node describes this node to the interface.
type Node struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	OS       string `json:"os"`
	Version  string `json:"version"`
	DataDir  string `json:"data_dir"`
	Listen   string `json:"listen"`
	Platform string `json:"platform"`
}

// Server holds what the handlers need.
type Server struct {
	Store        *data.Store
	Runtime      *runtime.Manager
	GitHub       *github.Client
	Dictation    *dictation.Manager
	SpeechModels *dictation.LocalModels
	Node         Node
	Events       *Events
	// Peers lists the other nodes of the mesh; nil until the mesh starts.
	Peers func() any
	// Remote forwards thread calls to the node that owns the thread.
	Remote Remote
	// Updater moves this node to the latest release; nil disables the routes.
	Updater *update.Updater
	// Mesh lists the peers that answer now.
	Mesh func(ctx context.Context) []MeshNode
}

// Register mounts every /api route; auth owns /api/auth/* on the same mux.
func (s *Server) Register(mux *http.ServeMux) {
	mountResource(mux, "projects", s.Store.Projects())
	mountResource(mux, "repositories", s.Store.Repositories())
	mountResource(mux, "tasks", s.Store.Tasks())
	mountResource(mux, "memories", s.Store.Memories())
	mux.HandleFunc("GET /api/version", version.Handler())
	mux.HandleFunc("GET /api/node", s.node)
	// health answers without a session: the CLI and the desktop app use it to
	// know whether the core of this machine is running.
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"app": "dory", "node_id": s.Node.ID, "name": s.Node.Name, "version": s.Node.Version, "pid": os.Getpid(), "data_dir": s.Node.DataDir})
	})
	mux.HandleFunc("GET /api/search", s.search)
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		// The live stream of another node's thread comes from that node.
		if id := r.URL.Query().Get("thread"); id != "" {
			if t, err := s.Store.Threads().Get(r.Context(), id); err == nil && t.NodeID != s.Node.ID && s.Remote != nil && s.Remote.Forward(w, r, t.NodeID) {
				return
			}
		}
		s.Events.serve(w, r)
	})
	s.mountRepositoryPaths(mux)
	s.mountTasks(mux)
	s.mountThreads(mux)
	s.mountGitHub(mux)
	s.mountPulls(mux)
	s.mountProviders(mux)
	s.mountSkills(mux)
	s.mountFolders(mux)
	s.mountBranches(mux)
	s.mountExternal(mux)
	s.mountDictation(mux)
	s.mountUpdate(mux)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
	})
}

func (s *Server) node(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"node": s.Node, "providers": s.Runtime.Providers(r.Context()), "usage": s.Runtime.Usage()}
	if s.Peers != nil {
		out["peers"] = s.Peers()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	hits, err := s.Store.Search(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		writeDataError(w, err)
		return
	}
	if hits == nil {
		hits = []data.Hit{}
	}
	writeJSON(w, http.StatusOK, hits)
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var b errorBody
	b.Error.Code, b.Error.Message = code, message
	writeJSON(w, status, b)
}

// writeDataError maps store and runtime errors to the JSON error envelope.
func writeDataError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, data.ErrNotFound), errors.Is(err, runtime.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, data.ErrInvalid), errors.Is(err, runtime.ErrProvider):
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	case errors.Is(err, runtime.ErrBusy):
		writeError(w, http.StatusConflict, "thread_busy", err.Error())
	case errors.Is(err, runtime.ErrRemote):
		writeError(w, http.StatusConflict, "remote", err.Error())
	default:
		log.Printf("api: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func actor(r *http.Request) string { return auth.Actor(r.Context()) }

// mountResource registers list/get/create/replace/archive for one entity.
func mountResource[T any](mux *http.ServeMux, path string, res data.Resource[T]) {
	base := "/api/" + path
	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		f, err := parseFilter(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
			return
		}
		items, err := res.List(r.Context(), f)
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	})
	mux.HandleFunc("GET "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := res.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})
	put := func(w http.ResponseWriter, r *http.Request, id string) {
		var body map[string]any
		if !decode(w, r, &body) {
			return
		}
		if id != "" {
			// Fields left out keep their stored value.
			if current, err := res.Get(r.Context(), id); err == nil {
				raw, _ := json.Marshal(current)
				merged := map[string]any{}
				json.Unmarshal(raw, &merged)
				for k, v := range body {
					merged[k] = v
				}
				body = merged
			}
			body["id"] = id
		}
		// Server-owned fields are never taken from the client.
		delete(body, "created_at")
		delete(body, "updated_at")
		delete(body, "archived_at")
		raw, _ := json.Marshal(body)
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
			return
		}
		out, err := res.Put(r.Context(), v)
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
	mux.HandleFunc("POST "+base, func(w http.ResponseWriter, r *http.Request) { put(w, r, "") })
	mux.HandleFunc("PUT "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) { put(w, r, r.PathValue("id")) })
	mux.HandleFunc("DELETE "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := res.Archive(r.Context(), r.PathValue("id")); err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func parseFilter(r *http.Request) (data.Filter, error) {
	q := r.URL.Query()
	var f data.Filter
	for name, dst := range map[string]*int{"limit": &f.Limit, "offset": &f.Offset} {
		if s := q.Get(name); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				return f, errors.New(name + " is out of range")
			}
			*dst = n
		}
	}
	f.IncludeArchived = q.Get("include_archived") == "true"
	if s := q.Get("filter"); s != "" {
		if err := json.Unmarshal([]byte(s), &f.Fields); err != nil {
			return f, errors.New("filter must be a JSON object")
		}
	}
	return f, nil
}
