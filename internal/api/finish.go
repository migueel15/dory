package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/runtime"
	"github.com/migueel15/dory/internal/workspace"
)

var stages = map[string]bool{"inbox": true, "ready": true, "done": true, "cancelled": true}

// CreateTask validates the initial stage and applies its side effects.
func (s *Server) CreateTask(ctx context.Context, t data.Task) (data.Task, error) {
	if t.Stage != "" && !stages[t.Stage] {
		return t, invalidStage()
	}
	if t.Stage == "done" {
		if err := s.checkDone(ctx, t); err != nil {
			return t, err
		}
	}
	out, err := s.Store.Tasks().Create(ctx, t)
	if err == nil && (out.Stage == "done" || out.Stage == "cancelled") {
		s.finishTask(ctx, out.ID)
	}
	return out, err
}

// PatchTask checks stage transitions and applies them against the version
// used for the check, so a concurrent edit cannot invalidate the decision.
func (s *Server) PatchTask(ctx context.Context, id string, expected *string, fields data.Fields) (data.Task, error) {
	tasks := s.Store.Tasks()
	stage, hasStage := fields["stage"]
	if !hasStage {
		if expected != nil {
			return tasks.PatchIf(ctx, id, *expected, fields)
		}
		return tasks.Patch(ctx, id, fields)
	}
	next, ok := stage.(string)
	if !ok || !stages[next] {
		return data.Task{}, invalidStage()
	}
	current, err := tasks.Get(ctx, id)
	if err != nil {
		return current, err
	}
	if expected != nil && *expected != current.UpdatedAt {
		return current, data.ErrRevisionConflict
	}
	changed := current.Stage != next
	if changed && next == "done" {
		// Check the pull request and repository as they will be after the patch.
		candidate := current
		for field, dst := range map[string]any{"repository_id": &candidate.RepositoryID, "pr_number": &candidate.PRNumber, "branch": &candidate.Branch} {
			if value, ok := fields[field]; ok {
				raw, err := json.Marshal(value)
				if err != nil || json.Unmarshal(raw, dst) != nil {
					return current, data.ErrInvalid.Msgf("invalid %s", field)
				}
			}
		}
		if err := s.checkDone(ctx, candidate); err != nil {
			return current, err
		}
	}
	out, err := tasks.PatchIf(ctx, id, current.UpdatedAt, fields)
	if err == nil && changed && (out.Stage == "done" || out.Stage == "cancelled") {
		s.finishTask(ctx, out.ID)
	}
	return out, err
}

func invalidStage() error {
	return data.ErrInvalid.Msgf("stage must be inbox, ready, done or cancelled")
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
	return data.ErrInvalid.Msgf("the task has an unmerged pull request (#%d); it cannot be marked done", *c.PRNumber).With("reason", "open_pull_request").With("number", *c.PRNumber)
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
		s.Runtime.CloseLive(t.ID)
		runtime.StopThreadServices(t.ID)
		s.Runtime.DropSnapshots(ctx, t.ID)
		if s.Settings.Get().RemoveWorktreeOnArchive {
			err := s.Runtime.Remove(ctx, t.ID, false)
			if err != nil && !errors.Is(err, workspace.ErrDirtyWorktree) && !errors.Is(err, runtime.ErrNotFound) {
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
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, kept)
	}))
	mux.HandleFunc("DELETE /api/threads/{id}/worktree", s.ownedArchived(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		if err := s.Runtime.Remove(r.Context(), t.ID, true); err != nil {
			writeErr(w, r, err)
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
			writeErr(w, r, err)
			return
		}
		if len(found) == 0 {
			writeErr(w, r, data.ErrNotFound)
			return
		}
		t := found[0]
		if t.NodeID != s.Node.ID {
			if s.Remote != nil && s.Remote.Forward(w, r, t.NodeID) {
				return
			}
			writeErr(w, r, apperr.New(apperr.NodeOffline, "the node that runs this thread is not connected").With("thread", true))
			return
		}
		h(w, r, t)
	}
}
