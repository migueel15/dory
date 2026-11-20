package api

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/runtime"
)

func TestColumnDoneByHand(t *testing.T) {
	n := int64(7)
	cases := []struct {
		name string
		c    Card
		want string
	}{
		{"sin pull request", Card{Task: data.Task{Stage: "done"}}, "done"},
		{"abierta", Card{Task: data.Task{Stage: "done", PRNumber: &n}, PRState: "open", Pull: &github.Pull{}}, "ready_to_merge"},
		{"borrador", Card{Task: data.Task{Stage: "done", PRNumber: &n}, PRState: "draft", Pull: &github.Pull{Draft: true}}, "working"},
		{"mergeada", Card{Task: data.Task{Stage: "done", PRNumber: &n}, PRState: "merged"}, "done"},
		{"cerrada", Card{Task: data.Task{Stage: "done", PRNumber: &n}, PRState: "closed"}, "cancelled"},
	}
	for _, tc := range cases {
		if got := column(tc.c); got != tc.want {
			t.Errorf("%s: column = %s, want %s", tc.name, got, tc.want)
		}
	}
}


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
	s := &Server{Store: store, Node: Node{ID: log.Node()}, Settings: NewSettingsStore(dir, settings)}
	s.Runtime = runtime.NewManager(db, store, log.Node(), dir)
	return s, dir
}

func TestSaveTaskStage(t *testing.T) {
	ctx := context.Background()
	s, _ := newFinishServer(t, Settings{})
	task, err := s.Store.Tasks().Put(ctx, data.Task{Title: "a"})
	if err != nil {
		t.Fatal(err)
	}
	task.Stage = "done"
	if out, err := s.SaveTask(ctx, task); err != nil || out.Stage != "done" {
		t.Fatalf("sin pull request: %v %+v", err, out)
	}
	task.Stage = "bogus"
	if _, err := s.SaveTask(ctx, task); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("stage inválido: %v", err)
	}
	// Con una pull request sin estado conocido se rechaza.
	n := int64(3)
	task.Stage, task.PRNumber = "inbox", &n
	if _, err := s.Store.Tasks().Put(ctx, task); err != nil {
		t.Fatal(err)
	}
	task.Stage = "done"
	if _, err := s.SaveTask(ctx, task); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("con pull request: %v", err)
	}
}

func addThread(t *testing.T, s *Server, task data.Task, worktree string) data.Thread {
	t.Helper()
	ctx := context.Background()
	th, err := s.Store.Threads().Put(ctx, data.Thread{NodeID: s.Node.ID, TaskID: &task.ID, Provider: "claude", Title: "h", Status: "idle"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB().ExecContext(ctx, `INSERT INTO thread_runtime(thread_id, worktree_path) VALUES (?, ?)`, th.ID, worktree); err != nil {
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
		task, _ := s.Store.Tasks().Put(ctx, data.Task{Title: "a"})
		th := addThread(t, s, task, "")
		task.Stage = "done"
		if _, err := s.SaveTask(ctx, task); err != nil {
			t.Fatal(err)
		}
		if got := archived(t, s, th.ID); got != on {
			t.Errorf("opción %v: archivado = %v", on, got)
		}
		// Idempotente: una segunda llamada no falla ni duplica trabajo.
		s.finishTask(ctx, task.ID)
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
	task, _ := s.Store.Tasks().Put(ctx, data.Task{Title: "a"})
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

	if err := s.archiveThread(ctx, clean); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cleanDir); !os.IsNotExist(err) {
		t.Error("el worktree limpio debía borrarse")
	}
	if err := s.archiveThread(ctx, dirty); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dirtyDir); err != nil || !archived(t, s, dirty.ID) {
		t.Error("el worktree con cambios debía conservarse y el hilo quedar archivado")
	}
	if exists, isDirty, _, err := s.Runtime.WorktreeState(ctx, dirty.ID); err != nil || !exists || !isDirty {
		t.Errorf("estado: %v %v %v", exists, isDirty, err)
	}
	if err := s.archiveThread(ctx, sharedA); err != nil {
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
