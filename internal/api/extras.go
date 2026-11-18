package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/dictation"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/oplog"
)

func (s *Server) mountRepositoryPaths(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/repository-paths", func(w http.ResponseWriter, r *http.Request) {
		paths, err := s.Store.RepositoryPaths(r.Context())
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, paths)
	})
	// Every environment that keeps each repository, published by each node.
	mux.HandleFunc("GET /api/repository-locations", func(w http.ResponseWriter, r *http.Request) {
		locs, err := s.Store.RepositoryLocations().List(r.Context(), data.Filter{Limit: 1000})
		if err != nil {
			writeDataError(w, err)
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
			writeDataError(w, err)
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
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (s *Server) projectFromRepository(ctx context.Context, name, slug, description, remote, branch string) (data.Repository, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return data.Repository{}, errors.Join(data.ErrInvalid, errors.New("remote_url required"))
	}
	if repo, err := s.Store.Repositories().Get(ctx, oplog.RepositoryID(remote)); err == nil {
		if _, err = s.Store.Projects().Get(ctx, repo.ProjectID); err == nil {
			return repo, nil
		}
	}
	p, err := s.Store.Projects().Get(ctx, oplog.ProjectID(remote))
	if err != nil {
		if p, err = s.Store.Projects().Put(ctx, data.Project{ID: oplog.ProjectID(remote), Name: name, Slug: slug, Description: description}); err != nil {
			return data.Repository{}, err
		}
	}
	if branch == "" {
		branch = "main"
	}
	return s.Store.Repositories().Put(ctx, data.Repository{ProjectID: p.ID, RemoteURL: remote, DefaultBranch: branch})
}

func writeGithubError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, github.ErrUnconfigured):
		writeError(w, http.StatusServiceUnavailable, "unavailable", err.Error())
	case errors.Is(err, github.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	case errors.Is(err, github.ErrUnknownRepo):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	default:
		writeError(w, http.StatusBadGateway, "upstream", err.Error())
	}
}

func (s *Server) mountGitHub(mux *http.ServeMux) {
	gh := s.GitHub
	repo := func(r *http.Request) string { return r.PathValue("owner") + "/" + r.PathValue("repo") }
	number := func(w http.ResponseWriter, r *http.Request) (int, bool) {
		n, err := strconv.Atoi(r.PathValue("number"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid number")
		}
		return n, err == nil
	}
	mux.HandleFunc("GET /api/github/open", func(w http.ResponseWriter, r *http.Request) {
		overview, err := gh.Open(r.Context(), r.URL.Query().Get("refresh") == "true")
		if err != nil {
			writeGithubError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, overview)
	})
	mux.HandleFunc("GET /api/github/closed", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, err := gh.ClosedPage(r.Context(), q.Get("kind"), q.Get("project"), q.Get("cursor"))
		if err != nil {
			writeGithubError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	})
	mux.HandleFunc("GET /api/github/repos/{owner}/{repo}/branches", func(w http.ResponseWriter, r *http.Request) {
		out, err := gh.Branches(r.Context(), repo(r))
		if err != nil {
			writeGithubError(w, err)
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
			writeGithubError(w, err)
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
			writeGithubError(w, err)
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
			writeGithubError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"diff": diff})
	})
	mux.HandleFunc("GET /api/github/repos/{owner}/{repo}/pull-refs", func(w http.ResponseWriter, r *http.Request) {
		refs, err := gh.PullRefs(r.Context(), repo(r), r.URL.Query().Get("q"))
		if err != nil {
			writeGithubError(w, err)
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
			writeDataError(w, err)
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
			return s.Store.Tasks().Put(ctx, data.Task{ProjectID: &rp.ProjectID, RepositoryID: &rp.ID, IssueNumber: &n, Title: title, Description: description})
		}
	}
	return data.Task{}, errors.Join(data.ErrNotFound, errors.New("repository not registered: "+repo))
}

const maxChunkBytes = 2 << 20

func (s *Server) mountDictation(mux *http.ServeMux) {
	if models := s.SpeechModels; models != nil {
		mux.HandleFunc("GET /api/dictation/models", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, models.Catalog())
		})
		mux.HandleFunc("POST /api/dictation/models/{id}/download", func(w http.ResponseWriter, r *http.Request) {
			if err := models.Download(r.PathValue("id")); err != nil {
				writeDictationError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, models.Catalog())
		})
		mux.HandleFunc("POST /api/dictation/models/{id}/transcribe", func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > 12<<20 {
				writeError(w, http.StatusRequestEntityTooLarge, "audio_too_large", "la grabación supera los 12 MiB")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
			if err := models.Transcribe(w, r, r.PathValue("id"), false); err != nil {
				writeDictationError(w, err)
			}
		})
		mux.HandleFunc("GET /api/dictation/models/{id}/stream", func(w http.ResponseWriter, r *http.Request) {
			if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				writeError(w, http.StatusBadRequest, "invalid_argument", "se requiere WebSocket")
				return
			}
			if err := models.Transcribe(w, r, r.PathValue("id"), true); err != nil {
				writeDictationError(w, err)
			}
		})
	}
	dict := s.Dictation
	mux.HandleFunc("POST /api/dictation", func(w http.ResponseWriter, r *http.Request) {
		st, err := dict.Create(actor(r))
		if err != nil {
			writeDictationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, st)
	})
	mux.HandleFunc("GET /api/dictation/{id}", func(w http.ResponseWriter, r *http.Request) {
		st, err := dict.Get(actor(r), r.PathValue("id"))
		if err != nil {
			writeDictationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("POST /api/dictation/{id}/chunks", func(w http.ResponseWriter, r *http.Request) {
		seq, err := strconv.Atoi(r.URL.Query().Get("seq"))
		if err != nil || seq < 0 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "seq must be a non-negative integer")
			return
		}
		wav, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxChunkBytes))
		if err != nil || len(wav) == 0 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "chunk must be a WAV body up to 2 MiB")
			return
		}
		st, err := dict.Add(actor(r), r.PathValue("id"), seq, wav)
		if err != nil {
			writeDictationError(w, err)
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
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid body")
			return
		}
		var deliver dictation.Deliver
		if body.Mode == "send" {
			switch body.Target.Kind {
			case "capture":
				project := body.Target.ProjectID
				deliver = func(ctx context.Context, text string) error {
					_, err := s.Store.Tasks().Put(ctx, data.Task{ProjectID: project, Title: shortTitle(text), Description: text})
					return err
				}
			case "thread":
				id := body.Target.ThreadID
				deliver = func(ctx context.Context, text string) error { return s.Runtime.Send(ctx, id, text) }
			default:
				writeError(w, http.StatusBadRequest, "invalid_argument", "target.kind must be capture or thread")
				return
			}
		} else if body.Mode != "insert" {
			writeError(w, http.StatusBadRequest, "invalid_argument", "mode must be insert or send")
			return
		}
		st, err := dict.Finish(r.Context(), actor(r), r.PathValue("id"), deliver)
		if err != nil {
			writeDictationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
}

func writeDictationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dictation.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, dictation.ErrUnavailable), errors.Is(err, dictation.ErrModelMissing):
		writeError(w, http.StatusServiceUnavailable, "unavailable", err.Error())
	case errors.Is(err, dictation.ErrFinished), errors.Is(err, dictation.ErrOutOfOrder):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, dictation.ErrQueueFull):
		writeError(w, http.StatusTooManyRequests, "too_many_requests", err.Error())
	default:
		writeError(w, http.StatusBadGateway, "upstream", err.Error())
	}
}
