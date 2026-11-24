// Package api owns the REST routes of a Dory v2 node.
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/browser"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/devices"
	"github.com/migueel15/dory/internal/dictation"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/mesh"
	"github.com/migueel15/dory/internal/nodestate"
	"github.com/migueel15/dory/internal/runtime"
	"github.com/migueel15/dory/internal/skills"
	"github.com/migueel15/dory/internal/update"
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
	Icon     string `json:"icon"`
}

// Server holds what the handlers need.
type Server struct {
	Store        *data.Store
	NodeState    *nodestate.Store
	Runtime      *runtime.Manager
	Threads      *app.Threads
	Tasks        *app.Tasks
	Plans        *app.Plans
	Memories     *app.Memories
	Projects     *app.Projects
	Ideas        *app.Ideas
	GitHub       *github.Client
	Dictation    *dictation.Manager
	SpeechModels *dictation.LocalModels
	Node         Node
	Events       *Events
	// Peers lists the other nodes of the mesh; nil until the mesh starts.
	Peers func() any
	// Remote forwards thread calls to the node that owns the thread.
	Remote Remote
	// PeerClient calls another node from Go.
	PeerClient *mesh.Client
	// Updater moves this node to the latest release; nil disables the routes.
	Updater *update.Updater
	// Mesh lists the peers that answer now.
	Mesh func(ctx context.Context) []MeshNode
	// InfoChanged tells the mesh this node's description changed, so peers
	// sync it at once; nil when there is no mesh.
	InfoChanged func()
	// Settings holds what this node does when work finishes; nil uses the defaults.
	Settings *SettingsStore
	// Devices bridges Android devices and emulators; nil disables the routes.
	Devices *devices.Manager
	// Browser runs the browsers of the threads of this node; nil disables them.
	Browser *browser.Manager
	// MeshPort is the mesh port, which the browser tunnel never reaches.
	MeshPort int
	// Skills installs the catalog on this node; nil disables conflict resolution.
	Skills *skills.Reconciler
}

// Register mounts every /api route; auth owns /api/auth/* on the same mux.
func (s *Server) Register(mux *http.ServeMux) {
	mountResource(mux, "projects", s.Store.Projects())
	mountResource(mux, "repositories", s.Store.Repositories())
	mountResource(mux, "tasks", s.Store.Tasks(), resourceWrites[data.Task]{s.Tasks.Create, s.Tasks.Patch})
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
		s.Events.serve(w, r)
	})
	s.mountRepositoryPaths(mux)
	s.mountTasks(mux)
	s.mountIdeas(mux)
	s.mountThreads(mux)
	s.mountGitHub(mux)
	s.mountGitHubAuth(mux)
	s.mountPulls(mux)
	s.mountProviders(mux)
	s.mountSkills(mux)
	s.mountFolders(mux)
	s.mountBranches(mux)
	s.mountNewThreadCommands(mux)
	s.mountWorktrees(mux)
	s.mountWorktreeState(mux)
	s.mountSettings(mux)
	s.mountExternal(mux)
	s.mountUsageHistory(mux)
	s.mountDictation(mux)
	s.mountUpdate(mux)
	s.mountDevices(mux)
	s.mountBrowser(mux)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, r, apperr.New(apperr.NotFound, "route not found"))
	})
}

func (s *Server) node(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"node": s.Info(), "providers": s.Runtime.Providers(r.Context()), "usage": s.Runtime.Usage()}
	if s.Peers != nil {
		out["peers"] = s.Peers()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var hits []data.Hit
	var err error
	if r.URL.Query().Get("type") == "task" {
		hits, err = s.Memories.SearchTasks(r.Context(), r.URL.Query().Get("q"), limit)
	} else {
		hits, err = s.Memories.Search(r.Context(), r.URL.Query().Get("q"), limit)
	}
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, hits)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: encode response: %v", err)
	}
}

// writeErr answers with the status and envelope of a domain error. An error
// that is not a domain error is an internal one: its text only goes to the log.
func writeErr(w http.ResponseWriter, r *http.Request, err error) { apperr.WriteHTTP(w, r, err) }

// upstream wraps the failure of a service Dory calls (GitHub, a peer). Its text
// is meant for the user, so it travels as a parameter.
func upstream(err error) *apperr.Error {
	return apperr.Wrap(apperr.Upstream, err, "upstream request failed").With("detail", err.Error())
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	if err := dec.Decode(v); err != nil {
		writeErr(w, r, apperr.Wrap(apperr.Invalid, err, "invalid JSON body"))
		return false
	}
	return true
}

func actor(r *http.Request) string { return auth.Actor(r.Context()) }

type resourceWrites[T any] struct {
	create func(context.Context, T) (T, error)
	patch  func(context.Context, string, *string, data.Fields) (T, error)
}

// mountResource registers list/get/create/patch/archive for one entity.
func mountResource[T any](mux *http.ServeMux, path string, res data.Resource[T], custom ...resourceWrites[T]) {
	base := "/api/" + path
	writes := resourceWrites[T]{
		create: res.Create,
		patch: func(ctx context.Context, id string, expected *string, fields data.Fields) (T, error) {
			if expected != nil {
				return res.PatchIf(ctx, id, *expected, fields)
			}
			return res.Patch(ctx, id, fields)
		},
	}
	if len(custom) > 0 {
		writes = custom[0]
	}
	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		f, err := parseFilter(r)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		items, err := res.List(r.Context(), f)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	})
	mux.HandleFunc("GET "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := res.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST "+base, func(w http.ResponseWriter, r *http.Request) {
		body, ok := decodeResource(w, r)
		if !ok {
			return
		}
		delete(body, "updated_at")
		raw, _ := json.Marshal(body)
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			writeErr(w, r, apperr.Wrap(apperr.Invalid, err, "invalid body"))
			return
		}
		out, err := writes.create(r.Context(), v)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	patch := func(w http.ResponseWriter, r *http.Request) {
		body, ok := decodeResource(w, r)
		if !ok {
			return
		}
		expected, err := resourceVersion(r, body)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		delete(body, "id")
		delete(body, "updated_at")
		out, err := writes.patch(r.Context(), r.PathValue("id"), expected, body)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
	mux.HandleFunc("PUT "+base+"/{id}", patch)
	mux.HandleFunc("PATCH "+base+"/{id}", patch)
	mux.HandleFunc("DELETE "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := res.Archive(r.Context(), r.PathValue("id")); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func decodeResource(w http.ResponseWriter, r *http.Request) (data.Fields, bool) {
	var body data.Fields
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil || body == nil {
		writeErr(w, r, apperr.New(apperr.Invalid, "expected a JSON object"))
		return nil, false
	}
	delete(body, "created_at")
	delete(body, "archived_at")
	return body, true
}

func resourceVersion(r *http.Request, body data.Fields) (*string, error) {
	if match := strings.TrimSpace(r.Header.Get("If-Match")); match != "" && match != "*" {
		if strings.HasPrefix(match, `"`) {
			value, err := strconv.Unquote(match)
			if err != nil {
				return nil, data.ErrInvalid.Msgf("invalid If-Match")
			}
			match = value
		}
		return &match, nil
	}
	if v, present := body["updated_at"]; present {
		version, ok := v.(string)
		if !ok {
			return nil, data.ErrInvalid.Msgf("updated_at must be a string")
		}
		return &version, nil
	}
	return nil, nil
}

func parseFilter(r *http.Request) (data.Filter, error) {
	q := r.URL.Query()
	var f data.Filter
	for name, dst := range map[string]*int{"limit": &f.Limit, "offset": &f.Offset} {
		if s := q.Get(name); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				return f, apperr.Newf(apperr.Invalid, "%s is out of range", name)
			}
			*dst = n
		}
	}
	f.IncludeArchived = q.Get("include_archived") == "true"
	if s := q.Get("filter"); s != "" {
		if err := json.Unmarshal([]byte(s), &f.Fields); err != nil {
			return f, apperr.New(apperr.Invalid, "filter must be a JSON object")
		}
	}
	return f, nil
}
