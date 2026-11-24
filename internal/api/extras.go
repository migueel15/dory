package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/migueel15/dory/internal/apperr"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/dictation"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/oplog"
)

func (s *Server) mountRepositoryPaths(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/repository-paths", func(w http.ResponseWriter, r *http.Request) {
		paths, err := s.Store.RepositoryPaths(r.Context())
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, paths)
	})
	// Every environment that keeps each repository, published by each node.
	mux.HandleFunc("GET /api/repository-locations", func(w http.ResponseWriter, r *http.Request) {
		locs, err := s.Store.RepositoryLocations().List(r.Context(), data.Filter{Limit: 1000})
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, locs)
	})
	// A project added from a repository is one project in every environment:
	// it reuses the project of the repository, or creates one whose id derives
	// from the remote so two environments offline converge on the same row.
	mux.HandleFunc("POST /api/projects/from-repository", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name          string `json:"name"`
			Slug          string `json:"slug"`
			Description   string `json:"description"`
			RemoteURL     string `json:"remote_url"`
			DefaultBranch string `json:"default_branch"`
		}
		if !decode(w, r, &body) {
			return
		}
		repo, err := s.projectFromRepository(r.Context(), body.Name, body.Slug, body.Description, body.RemoteURL, body.DefaultBranch)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, repo)
	})
	mux.HandleFunc("PUT /api/repositories/{id}/path", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
		}
		if !decode(w, r, &body) {
			return
		}
		if err := s.Store.SetRepositoryPath(r.Context(), r.PathValue("id"), body.Path); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (s *Server) projectFromRepository(ctx context.Context, name, slug, description, remote, branch string) (data.Repository, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return data.Repository{}, data.ErrInvalid.Msgf("remote_url required")
	}
	if repo, err := s.Store.Repositories().Get(ctx, oplog.RepositoryID(remote)); err == nil {
		if _, err = s.Store.Projects().Get(ctx, repo.ProjectID); err == nil {
			return repo, nil
		}
	}
	projectID := oplog.ProjectID(remote)
	p, err := s.Store.Projects().Get(ctx, projectID)
	if errors.Is(err, data.ErrNotFound) {
		p, err = s.Store.Projects().Create(ctx, data.Project{ID: projectID, Name: name, Slug: slug, Description: description})
		if errors.Is(err, data.ErrConflict) {
			p, err = s.Store.Projects().Get(ctx, projectID)
		}
	}
	if err != nil {
		return data.Repository{}, err
	}
	if branch == "" {
		branch = "main"
	}
	return s.Store.Repositories().Upsert(ctx, data.Repository{ProjectID: p.ID, RemoteURL: remote, DefaultBranch: branch})
}

// githubFailure keeps the domain errors of the github package and turns any
// other failure into an upstream one.
func githubFailure(err error) error {
	if apperr.As(err) != nil {
		return err
	}
	return upstream(err)
}

func (s *Server) mountGitHub(mux *http.ServeMux) {
	gh := s.GitHub
	repo := func(r *http.Request) string { return r.PathValue("owner") + "/" + r.PathValue("repo") }
	number := func(w http.ResponseWriter, r *http.Request) (int, bool) {
		n, err := strconv.Atoi(r.PathValue("number"))
		if err != nil {
			writeErr(w, r, apperr.New(apperr.Invalid, "invalid number"))
		}
		return n, err == nil
	}
	// A view with branches or pull requests is open: poll GitHub fast.
	mux.HandleFunc("POST /api/github/watch", func(w http.ResponseWriter, r *http.Request) {
		if gh != nil {
			gh.Beat()
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/github/search", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		out, err := gh.Search(ctx, r.URL.Query().Get("q"))
		if err != nil {
			writeErr(w, r, githubFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("GET /api/github/open", func(w http.ResponseWriter, r *http.Request) {
		overview, err := gh.Open(r.Context(), r.URL.Query().Get("refresh") == "true")
		if err != nil {
			writeErr(w, r, githubFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, overview)
	})
	mux.HandleFunc("GET /api/github/closed", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, err := gh.ClosedPage(r.Context(), q.Get("kind"), q.Get("project"), q.Get("cursor"))
		if err != nil {
			writeErr(w, r, githubFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, page)
	})
	mux.HandleFunc("GET /api/github/repos/{owner}/{repo}/branches", func(w http.ResponseWriter, r *http.Request) {
		out, err := gh.Branches(r.Context(), repo(r))
		if err != nil {
			writeErr(w, r, githubFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("GET /api/github/repos/{owner}/{repo}/{kind}/{number}", func(w http.ResponseWriter, r *http.Request) {
		n, ok := number(w, r)
		if !ok {
			return
		}
		detail, err := gh.Detail(r.Context(), repo(r), r.PathValue("kind"), n)
		if err != nil {
			writeErr(w, r, githubFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, detail)
	})
	mux.HandleFunc("POST /api/github/repos/{owner}/{repo}/{kind}/{number}/actions", func(w http.ResponseWriter, r *http.Request) {
		n, ok := number(w, r)
		if !ok {
			return
		}
		var action github.Action
		if !decode(w, r, &action) {
			return
		}
		if err := gh.Act(r.Context(), repo(r), r.PathValue("kind"), n, action); err != nil {
			writeErr(w, r, githubFailure(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/github/repos/{owner}/{repo}/pulls/{number}/diff", func(w http.ResponseWriter, r *http.Request) {
		n, ok := number(w, r)
		if !ok {
			return
		}
		diff, err := gh.Diff(r.Context(), repo(r), n)
		if err != nil {
			writeErr(w, r, githubFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"diff": diff})
	})
	mux.HandleFunc("GET /api/github/repos/{owner}/{repo}/pull-refs", func(w http.ResponseWriter, r *http.Request) {
		refs, err := gh.PullRefs(r.Context(), repo(r), r.URL.Query().Get("q"))
		if err != nil {
			writeErr(w, r, githubFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"pulls": refs})
	})
	// An issue becomes a task of its project (requirement RF-26).
	mux.HandleFunc("POST /api/github/repos/{owner}/{repo}/issues/{number}/tasks", func(w http.ResponseWriter, r *http.Request) {
		n, ok := number(w, r)
		if !ok {
			return
		}
		var body struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		}
		if !decode(w, r, &body) {
			return
		}
		t, err := s.issueTask(r.Context(), repo(r), n, body.Title, body.Description)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, t)
	})
}

// issueTask creates a task linked to an issue of a registered repository.
func (s *Server) issueTask(ctx context.Context, repo string, number int, title, description string) (data.Task, error) {
	repos, err := s.Store.Repositories().List(ctx, data.Filter{Limit: 1000})
	if err != nil {
		return data.Task{}, err
	}
	for _, rp := range repos {
		if name, ok := github.ParseRemote(rp.RemoteURL); ok && strings.EqualFold(name, repo) {
			n := int64(number)
			if title == "" {
				title = "Issue #" + strconv.Itoa(number)
			}
			return s.Store.Tasks().Create(ctx, data.Task{ProjectID: &rp.ProjectID, RepositoryID: &rp.ID, IssueNumber: &n, Title: title, Description: description})
		}
	}
	return data.Task{}, data.ErrNotFound.Msgf("repository not registered: %s", repo)
}

const maxChunkBytes = 2 << 20

func (s *Server) mountDictation(mux *http.ServeMux) {
	if models := s.SpeechModels; models != nil {
		mux.HandleFunc("GET /api/dictation/models", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, models.Catalog())
		})
		mux.HandleFunc("POST /api/dictation/models/{id}/download", func(w http.ResponseWriter, r *http.Request) {
			if err := models.Download(r.PathValue("id")); err != nil {
				writeErr(w, r, dictationFailure(err))
				return
			}
			writeJSON(w, http.StatusAccepted, models.Catalog())
		})
		mux.HandleFunc("POST /api/dictation/models/{id}/transcribe", func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > 12<<20 {
				writeErr(w, r, apperr.New(apperr.PayloadTooLarge, "the recording exceeds 12 MiB").With("reason", "audio"))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
			if err := models.Transcribe(w, r, r.PathValue("id"), false); err != nil {
				writeErr(w, r, dictationFailure(err))
			}
		})
		mux.HandleFunc("GET /api/dictation/models/{id}/stream", func(w http.ResponseWriter, r *http.Request) {
			if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				writeErr(w, r, apperr.New(apperr.Invalid, "a WebSocket is required").With("reason", "websocket_required"))
				return
			}
			if err := models.Transcribe(w, r, r.PathValue("id"), true); err != nil {
				writeErr(w, r, dictationFailure(err))
			}
		})
	}
	dict := s.Dictation
	mux.HandleFunc("POST /api/dictation", func(w http.ResponseWriter, r *http.Request) {
		st, err := dict.Create(actor(r))
		if err != nil {
			writeErr(w, r, dictationFailure(err))
			return
		}
		writeJSON(w, http.StatusCreated, st)
	})
	mux.HandleFunc("GET /api/dictation/{id}", func(w http.ResponseWriter, r *http.Request) {
		st, err := dict.Get(actor(r), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, dictationFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("POST /api/dictation/{id}/chunks", func(w http.ResponseWriter, r *http.Request) {
		seq, err := strconv.Atoi(r.URL.Query().Get("seq"))
		if err != nil || seq < 0 {
			writeErr(w, r, apperr.New(apperr.Invalid, "seq must be a non-negative integer"))
			return
		}
		wav, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxChunkBytes))
		if err != nil || len(wav) == 0 {
			writeErr(w, r, apperr.New(apperr.Invalid, "chunk must be a WAV body up to 2 MiB"))
			return
		}
		st, err := dict.Add(actor(r), r.PathValue("id"), seq, wav)
		if err != nil {
			writeErr(w, r, dictationFailure(err))
			return
		}
		writeJSON(w, http.StatusAccepted, st)
	})
	// finish either returns the text (insert) or delivers it: to a new
	// inbox task (capture) or as a message to a thread.
	mux.HandleFunc("POST /api/dictation/{id}/finish", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Mode   string `json:"mode"`
			Target struct {
				Kind      string  `json:"kind"`
				ProjectID *string `json:"project_id"`
				ThreadID  string  `json:"thread_id"`
			} `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, r, apperr.New(apperr.Invalid, "invalid body"))
			return
		}
		var deliver dictation.Deliver
		if body.Mode == "send" {
			switch body.Target.Kind {
			case "capture":
				project := body.Target.ProjectID
				deliver = func(ctx context.Context, text string) error {
					_, err := s.Store.Tasks().Create(ctx, data.Task{ProjectID: project, Title: shortTitle(text), Description: text})
					return err
				}
			case "thread":
				id := body.Target.ThreadID
				deliver = func(ctx context.Context, text string) error { return s.Runtime.Send(ctx, id, text) }
			default:
				writeErr(w, r, apperr.New(apperr.Invalid, "target.kind must be capture or thread"))
				return
			}
		} else if body.Mode != "insert" {
			writeErr(w, r, apperr.New(apperr.Invalid, "mode must be insert or send"))
			return
		}
		st, err := dict.Finish(r.Context(), actor(r), r.PathValue("id"), deliver)
		if err != nil {
			writeErr(w, r, dictationFailure(err))
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
}

// dictationFailure keeps the domain errors of the dictation package and turns
// any other failure of the engine into an upstream one.
func dictationFailure(err error) error {
	if apperr.As(err) != nil {
		return err
	}
	return upstream(err)
}

func shortTitle(text string) string {
	r := []rune(text)
	for i, c := range r {
		if c == '\n' {
			r = r[:i]
			break
		}
	}
	if len(r) > 60 {
		r = append(r[:59], '…')
	}
	if len(r) == 0 {
		return "Nuevo hilo"
	}
	return string(r)
}
