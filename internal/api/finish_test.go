package api

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/nodestate"
	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/runtime"
)

func newFinishServer(t *testing.T, settings Settings) (*Server, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := localdb.Open(ctx, filepath.Join(dir, "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	log, err := oplog.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	store := data.New(db, log)
	state := nodestate.New(db)
	s := &Server{Store: store, Node: Node{ID: log.Node()}, Settings: NewSettingsStore(state, settings)}
	s.Runtime = runtime.NewManager(db, store, log.Node(), dir, state, nil)
	s.Threads = &app.Threads{Store: store, Runtime: s.Runtime, Settings: s.Settings, Node: s.Node.ID}
	s.Tasks = &app.Tasks{Store: store, Threads: s.Threads, Node: s.Node.ID, ArchiveOnFinish: func() bool { return s.Settings.Get().ArchiveThreadsOnFinish }}
	return s, dir
}

func TestSaveTaskStage(t *testing.T) {
	ctx := context.Background()
	s, _ := newFinishServer(t, Settings{})
	task, err := s.Store.Tasks().Create(ctx, data.Task{Title: "a"})
	if err != nil {
		t.Fatal(err)
	}
	task.Stage = "done"
	if out, err := s.Tasks.Patch(ctx, task.ID, nil, data.Fields{"stage": task.Stage}); err != nil || out.Stage != "done" {
		t.Fatalf("sin pull request: %v %+v", err, out)
	}
	task.Stage = "bogus"
	if _, err := s.Tasks.Patch(ctx, task.ID, nil, data.Fields{"stage": task.Stage}); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("stage inválido: %v", err)
	}
	// Con una pull request sin estado conocido se rechaza.
	n := int64(3)
	task.Stage, task.PRNumber = "inbox", &n
	if _, err := s.Store.Tasks().Patch(ctx, task.ID, data.Fields{"stage": task.Stage, "pr_number": task.PRNumber}); err != nil {
		t.Fatal(err)
	}
	task.Stage = "done"
	if _, err := s.Tasks.Patch(ctx, task.ID, nil, data.Fields{"stage": task.Stage}); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("con pull request: %v", err)
	}
}

func addThread(t *testing.T, s *Server, task data.Task, worktree string) data.Thread {
	t.Helper()
	ctx := context.Background()
	th, err := s.Store.Threads().Create(ctx, data.Thread{NodeID: s.Node.ID, TaskID: &task.ID, Provider: "claude", Title: "h", Status: "idle"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.LocalDB().ExecContext(ctx, `INSERT INTO thread_runtime(thread_id, worktree_path) VALUES (?, ?)`, th.ID, worktree); err != nil {
		t.Fatal(err)
	}
	return th
}

func archived(t *testing.T, s *Server, id string) bool {
	t.Helper()
	// Get does not return archived rows.
	_, err := s.Store.Threads().Get(context.Background(), id)
	if err != nil && !errors.Is(err, data.ErrNotFound) {
		t.Fatal(err)
	}
	return errors.Is(err, data.ErrNotFound)
}

func TestFinishTask(t *testing.T) {
	ctx := context.Background()
	for _, on := range []bool{false, true} {
		s, _ := newFinishServer(t, Settings{ArchiveThreadsOnFinish: on})
		task, _ := s.Store.Tasks().Create(ctx, data.Task{Title: "a"})
		th := addThread(t, s, task, "")
		task.Stage = "done"
		if _, err := s.Tasks.Patch(ctx, task.ID, nil, data.Fields{"stage": task.Stage}); err != nil {
			t.Fatal(err)
		}
		if got := archived(t, s, th.ID); got != on {
			t.Errorf("opción %v: archivado = %v", on, got)
		}
		// Idempotente: una segunda llamada no falla ni duplica trabajo.
		s.Tasks.FinishTask(ctx, task.ID)
		if got := archived(t, s, th.ID); got != on {
			t.Errorf("opción %v tras repetir: archivado = %v", on, got)
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=a@b.c", "-c", "user.name=t"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func TestArchiveRemovesWorktree(t *testing.T) {
	ctx := context.Background()
	s, dir := newFinishServer(t, Settings{ArchiveThreadsOnFinish: true, RemoveWorktreeOnArchive: true})
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	task, _ := s.Store.Tasks().Create(ctx, data.Task{Title: "a"})
	mk := func(name string) (data.Thread, string) {
		wt := filepath.Join(dir, "worktrees", name)
		git(t, repo, "worktree", "add", "-q", "-b", name, wt)
		return addThread(t, s, task, wt), wt
	}
	clean, cleanDir := mk("limpio")
	dirty, dirtyDir := mk("sucio")
	if err := os.WriteFile(filepath.Join(dirtyDir, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Dos hilos comparten un worktree: se conserva mientras el otro no esté archivado.
	sharedA, sharedDir := mk("compartido")
	sharedB := addThread(t, s, task, sharedDir)

	if err := s.Threads.Archive(ctx, clean.ID, app.ArchiveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cleanDir); !os.IsNotExist(err) {
		t.Error("el worktree limpio debía borrarse")
	}
	if err := s.Threads.Archive(ctx, dirty.ID, app.ArchiveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dirtyDir); err != nil || !archived(t, s, dirty.ID) {
		t.Error("el worktree con cambios debía conservarse y el hilo quedar archivado")
	}
	if exists, isDirty, _, err := s.Runtime.WorktreeState(ctx, dirty.ID); err != nil || !exists || !isDirty {
		t.Errorf("estado: %v %v %v", exists, isDirty, err)
	}
	if err := s.Threads.Archive(ctx, sharedA.ID, app.ArchiveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sharedDir); err != nil {
		t.Error("el worktree compartido debía conservarse")
	}
	_ = sharedB
	if err := s.Runtime.Remove(ctx, dirty.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dirtyDir); !os.IsNotExist(err) {
		t.Error("la eliminación explícita debía borrar el worktree con cambios")
	}
}

func TestCardsUseThreadState(t *testing.T) {
	ctx := context.Background()
	s, _ := newFinishServer(t, Settings{})
	task, err := s.Store.Tasks().Create(ctx, data.Task{Title: "Estado del hilo"})
	if err != nil {
		t.Fatal(err)
	}
	thread := addThread(t, s, task, "")
	for status, want := range map[string]string{"working": "working", "waiting": "needs_you", "error": "needs_you"} {
		if err := s.Store.SetThreadState(ctx, thread.ID, status, ""); err != nil {
			t.Fatal(err)
		}
		cards, err := s.Cards(ctx, []data.Task{task})
		if err != nil || len(cards) != 1 || cards[0].Column != want || cards[0].Threads[0].Status != status {
			t.Fatalf("%s: cards = %+v: %v", status, cards, err)
		}
	}
}
