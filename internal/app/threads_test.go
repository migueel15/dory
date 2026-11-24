package app

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/mesh/peerapi"
	"github.com/migueel15/dory/internal/runtime"
)

type testStore struct {
	ThreadStore
	parent data.Thread
}

func (s testStore) Thread(context.Context, string) (data.Thread, error) { return s.parent, nil }

func (testStore) RepositoryPath(context.Context, string) (string, error) { return "/repo", nil }

type testSettings struct{ settings ThreadSettings }

func (s testSettings) ThreadSettings() ThreadSettings { return s.settings }

type testRuntime struct {
	ThreadRuntime
	resolutions []string
	resolve     func(string) error
	saved       []data.Thread
	dir         string
	registerErr error
	sendErr     error
	delegated   bool
}

func (r *testRuntime) ResolveSettings(_ context.Context, _ data.Thread, s *runtime.ThreadSettings) error {
	r.resolutions = append(r.resolutions, *s.Provider)
	if r.resolve != nil {
		return r.resolve(*s.Provider)
	}
	return nil
}

func (r *testRuntime) Register(_ context.Context, th data.Thread, dir, _ string) (data.Thread, error) {
	r.saved, r.dir = append(r.saved, th), dir
	return th, r.registerErr
}

func (r *testRuntime) Send(_ context.Context, _, _ string, _ ...runtime.Attachment) error {
	return r.sendErr
}

func (*testRuntime) Dir(context.Context, string) (string, error) { return "/parent", nil }

func (*testRuntime) WorktreeUse(context.Context, string, string) (runtime.WorktreeUse, error) {
	return runtime.WorktreeUse{BaseBranch: "main"}, nil
}

func (*testRuntime) CanSpawn(context.Context, data.Thread) error { return nil }

func (r *testRuntime) RecordDelegation(_ context.Context, parent, child, node, title string) (runtime.Delegation, error) {
	r.delegated = true
	return runtime.Delegation{ParentThreadID: parent, ChildThreadID: child, ChildNodeID: node, Title: title}, nil
}

type testWorkspace struct {
	Workspace
	addError                 error
	createdBranch            bool
	added, removed, deleted  string
	forced, cleanupCancelled bool
}

func (w *testWorkspace) AddWorktree(_ context.Context, _, dir, _, _ string) (bool, error) {
	w.added = dir
	return w.createdBranch, w.addError
}

func (w *testWorkspace) RemoveWorktree(ctx context.Context, dir string, force bool) error {
	w.removed, w.forced = dir, force
	w.cleanupCancelled = ctx.Err() != nil
	return nil
}

func (w *testWorkspace) DeleteBranch(_ context.Context, _, branch string) error {
	w.deleted = branch
	return nil
}

func TestWorktreeFailureDoesNotRegisterOrTryFallback(t *testing.T) {
	gitErr := errors.New("git failed")
	w := &testWorkspace{addError: gitErr}
	r := &testRuntime{}
	s := Threads{Node: "local-id", Store: testStore{}, Workspace: w, Runtime: r,
		Settings: testSettings{ThreadSettings{DefaultModel: ModelChoice{Provider: "codex"}, FallbackModel: ModelChoice{Provider: "claude"}}}}
	yes := true
	th, err := s.Create(t.Context(), CreateThread{CreateThreadRequest: peerapi.CreateThreadRequest{RepositoryID: "repo", Worktree: &yes, Branch: "feat/test"}})
	if !errors.Is(err, gitErr) || th.ID != "" || len(r.saved) != 0 || !reflect.DeepEqual(r.resolutions, []string{"codex"}) || w.added == "" {
		t.Fatalf("thread %+v, error %v, saves %+v, settings %v, worktree %q", th, err, r.saved, r.resolutions, w.added)
	}
}

func TestPersistenceFailureCompensatesWorktree(t *testing.T) {
	for _, created := range []bool{false, true} {
		r := &testRuntime{registerErr: context.Canceled}
		w := &testWorkspace{createdBranch: created}
		s := Threads{Store: testStore{}, Runtime: r, Workspace: w, Settings: testSettings{}}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		yes := true
		th, err := s.Create(ctx, CreateThread{CreateThreadRequest: peerapi.CreateThreadRequest{RepositoryID: "repo", Worktree: &yes, Branch: "feat/test"}})
		if !errors.Is(err, context.Canceled) || th.ID != "" || len(r.saved) != 1 || filepath.Base(w.added) != r.saved[0].ID ||
			w.removed != w.added || !w.forced || w.cleanupCancelled || (w.deleted == "feat/test") != created {
			t.Fatalf("thread %+v, error %v, saves %+v, workspace %+v", th, err, r.saved, w)
		}
	}
}

func TestFallbackRequiresProviderUnavailable(t *testing.T) {
	for _, tc := range []struct {
		code apperr.Code
		want []string
	}{
		{apperr.ProviderUnavailable, []string{"codex", "claude"}},
		{apperr.Invalid, []string{"codex"}},
	} {
		r := &testRuntime{resolve: func(provider string) error {
			if provider == "codex" {
				return apperr.New(tc.code, "selection failed")
			}
			return nil
		}}
		s := Threads{Runtime: r, Settings: testSettings{ThreadSettings{FallbackModel: ModelChoice{Provider: "claude", Model: "sonnet"}}}}
		th, err := s.Create(t.Context(), CreateThread{CreateThreadRequest: peerapi.CreateThreadRequest{Provider: "codex"}})
		if !reflect.DeepEqual(r.resolutions, tc.want) {
			t.Fatalf("code %s resolved %v", tc.code, r.resolutions)
		}
		if tc.code == apperr.ProviderUnavailable && (err != nil || th.Provider != "claude" || th.Model != "sonnet") {
			t.Fatalf("fallback %+v: %v", th, err)
		}
		if tc.code == apperr.Invalid && (err == nil || len(r.saved) != 0) {
			t.Fatalf("invalid selection produced %+v: %v", th, err)
		}
	}
}

func TestSpawnInheritsParentSelectionAndWorkspace(t *testing.T) {
	task := "task"
	parent := data.Thread{ID: "parent", NodeID: "node", Provider: "codex", Model: "gpt", Effort: "high", RuntimeMode: "approval-required", Branch: "feature", TaskID: &task}
	r := &testRuntime{}
	s := Threads{Node: parent.NodeID, Store: testStore{parent: parent}, Runtime: r, Settings: testSettings{}}
	child, delegation, err := s.Spawn(t.Context(), parent.ID, CreateThread{CreateThreadRequest: peerapi.CreateThreadRequest{Text: "Primera línea\nSegunda línea"}})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID || child.TaskID != &task || child.Provider != parent.Provider || child.Model != parent.Model ||
		child.Effort != parent.Effort || child.RuntimeMode != parent.RuntimeMode || child.Branch != parent.Branch || r.dir != "/parent" ||
		delegation.ChildThreadID != child.ID || child.Title != "Primera línea" || !r.delegated {
		t.Fatalf("child %+v, dir %q, delegation %+v", child, r.dir, delegation)
	}
}

func TestSendFailureReturnsRegisteredThread(t *testing.T) {
	sendErr := errors.New("send failed")
	r := &testRuntime{sendErr: sendErr}
	s := Threads{Node: "node", Runtime: r, Settings: testSettings{}}
	th, err := s.Create(t.Context(), CreateThread{CreateThreadRequest: peerapi.CreateThreadRequest{Provider: "codex", Text: "hello"}})
	e := apperr.As(err)
	if len(r.saved) != 1 || th.ID != r.saved[0].ID || e == nil || e.Code != apperr.ThreadCreatedSendFailed || e.Params["thread_id"] != th.ID || !errors.Is(err, sendErr) {
		t.Fatalf("thread %+v, error %+v", th, e)
	}
}

type testPeer struct {
	PeerClient
	node string
	in   peerapi.CreateThreadRequest
}

func (p *testPeer) CreateThread(_ context.Context, node string, in peerapi.CreateThreadRequest) (peerapi.CreateThreadResponse, error) {
	p.node, p.in = node, in
	return peerapi.CreateThreadResponse{ThreadID: "remote-thread", Thread: &data.Thread{ID: "remote-thread", NodeID: node, Provider: "claude"}}, nil
}

func TestCreateDelegatesUnchangedInputToPeer(t *testing.T) {
	p := &testPeer{}
	s := Threads{Node: "here", Peers: p}
	in := peerapi.CreateThreadRequest{RepositoryID: "remote-repo", Text: "hello", Inherit: &data.Thread{Provider: "codex"}}
	got, err := s.Create(t.Context(), CreateThread{Node: "there", CreateThreadRequest: in})
	if err != nil || got.ID != "remote-thread" || got.Provider != "claude" || p.node != "there" || !reflect.DeepEqual(p.in, in) {
		t.Fatalf("remote thread %+v, error %v, node %q, input %+v", got, err, p.node, p.in)
	}
}
