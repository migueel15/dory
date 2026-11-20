package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/runtime"
)

var stages = map[string]bool{"inbox": true, "ready": true, "done": true, "cancelled": true}

// SaveTask stores a task after checking its stage, and cleans up the threads
// of the task when it becomes done or cancelled. The API and the MCP share it.
func (s *Server) SaveTask(ctx context.Context, t data.Task) (data.Task, error) {
	var prev *data.Task
	if t.ID != "" {
		if p, err := s.Store.Tasks().Get(ctx, t.ID); err == nil {
			prev = &p
		}
	}
	if t.Stage != "" && !stages[t.Stage] {
		return t, fmt.Errorf("%w: stage must be inbox, ready, done or cancelled", data.ErrInvalid)
	}
	changed := prev == nil || prev.Stage != t.Stage
	if t.Stage == "done" && changed {
		if err := s.checkDone(ctx, t); err != nil {
			return t, err
		}
	}
	out, err := s.Store.Tasks().Put(ctx, t)
	if err == nil && changed && (out.Stage == "done" || out.Stage == "cancelled") {
		s.finishTask(ctx, out.ID)
	}
	return out, err
}

// checkDone accepts a task without pull request or whose pull request is
// merged. When GitHub cannot tell the state of its pull request it refuses, so
// the rule is never skipped.
func (s *Server) checkDone(ctx context.Context, t data.Task) error {
	c := Card{Task: t}
	if t.RepositoryID != nil {
		if r, err := s.Store.Repositories().Get(ctx, *t.RepositoryID); err == nil {
			c.Repo, _ = github.ParseRemote(r.RemoteURL)
		}
	}
	s.attachPull(ctx, &c, s.openPulls(ctx))
	if c.PRNumber == nil || c.PRState == "merged" {
		return nil
	}
	return fmt.Errorf("%w: La tarea tiene una pull request sin mergear (#%d); no se puede marcar como hecha", data.ErrInvalid, *c.PRNumber)
}

// finishTask archives the threads of a task that is done or cancelled, when
// the node is set to. Archived threads are skipped, so it is safe to call it
// again from the manual change and from the pull request refresh.
func (s *Server) finishTask(ctx context.Context, taskID string) {
	if !s.Settings.Get().ArchiveThreadsOnFinish {
		return
	}
	threads, err := s.Store.Threads().List(ctx, data.Filter{Fields: map[string]any{"task_id": taskID}, Limit: 1000})
	if err != nil {
		log.Printf("api: finish task %s: %v", taskID, err)
		return
	}
	for _, t := range threads {
		if t.NodeID != s.Node.ID {
			continue
		}
		if err := s.archiveThread(ctx, t); err != nil {
			log.Printf("api: archive thread %s: %v", t.ID, err)
		}
	}
}

// FinishTasks archives the threads of every task whose pull request was
// merged or closed. The GitHub watcher calls it when a pull request changes.
func (s *Server) FinishTasks(ctx context.Context) {
	if !s.Settings.Get().ArchiveThreadsOnFinish {
		return
	}
	threads, err := s.Store.Threads().List(ctx, data.Filter{Limit: 1000})
	if err != nil {
		return
	}
	seen := map[string]bool{}
	var tasks []data.Task
	for _, th := range threads {
		if th.TaskID == nil || th.NodeID != s.Node.ID || seen[*th.TaskID] {
			continue
		}
		seen[*th.TaskID] = true
		if t, err := s.Store.Tasks().Get(ctx, *th.TaskID); err == nil {
			tasks = append(tasks, t)
		}
	}
	cards, err := s.Cards(ctx, tasks)
	if err != nil {
		return
	}
	for _, c := range cards {
		if c.Column == "done" || c.Column == "cancelled" {
			s.finishTask(ctx, c.ID)
		}
	}
}

// archiveThread stops a thread of this node, deletes its worktree when the
// node is set to and it is clean, and archives it. A worktree with
// uncommitted changes is kept; the thread shows it so the user can delete it.
func (s *Server) archiveThread(ctx context.Context, t data.Thread) error {
	if t.NodeID == s.Node.ID {
		_ = s.Runtime.Interrupt(ctx, t.ID)
		if s.Settings.Get().RemoveWorktreeOnArchive {
			err := s.Runtime.Remove(ctx, t.ID, false)
			if err != nil && !errors.Is(err, runtime.ErrDirtyWorktree) && !errors.Is(err, runtime.ErrNotFound) {
				return err
			}
		}
	}
	return s.Store.Threads().Archive(ctx, t.ID)
}

// mountWorktreeState lets the interface warn about the worktree an archived
// thread kept and delete it on request.
func (s *Server) mountWorktreeState(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/kept-worktrees", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		kept, err := s.Runtime.KeptWorktrees(r.Context())
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, kept)
	}))
	mux.HandleFunc("DELETE /api/threads/{id}/worktree", s.ownedArchived(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		if err := s.Runtime.Remove(r.Context(), t.ID, true); err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

// ownedArchived is owned for threads that may be archived.
func (s *Server) ownedArchived(h func(http.ResponseWriter, *http.Request, data.Thread)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		found, err := s.Store.Threads().List(r.Context(), data.Filter{Fields: map[string]any{"id": r.PathValue("id")}, IncludeArchived: true, Limit: 1})
		if err != nil {
			writeDataError(w, err)
			return
		}
		if len(found) == 0 {
			writeDataError(w, data.ErrNotFound)
			return
		}
		t := found[0]
		if t.NodeID != s.Node.ID {
			if s.Remote != nil && s.Remote.Forward(w, r, t.NodeID) {
				return
			}
			writeError(w, http.StatusServiceUnavailable, "node_offline", "el nodo que ejecuta este hilo no está conectado")
			return
		}
		h(w, r, t)
	}
}
