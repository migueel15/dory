package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/config"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/devices"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/mesh/peerapi"
	"github.com/migueel15/dory/internal/nodestate"
	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/runtime"
)

func TestThreadControlTools(t *testing.T) {
	tools := toolsOf(t, build(&api.Server{Devices: devices.NewManager()}, false))
	for _, n := range []string{"list_models", "list_threads", "get_thread", "thread_configure", "thread_send", "thread_stop", "subagent_spawn", "thread_create", "list_environments", "subagent_status", "subagent_cancel"} {
		if tools[n] == nil {
			t.Errorf("missing %s", n)
		}
	}
	if !strings.Contains(tools["thread_create"].Description, "Not for subagents") || !strings.Contains(tools["subagent_spawn"].Description, "Not for a new chat") {
		t.Error("thread_create and subagent_spawn do not say which one to use")
	}
	if !strings.Contains(tools["thread_configure"].Description, "continue_with") {
		t.Error("thread_configure does not explain continue_with")
	}
}

func TestTargetDefaultsToCaller(t *testing.T) {
	ctx := context.WithValue(context.Background(), threadKey{}, "self")
	if id, _ := target(ctx, ""); id != "self" {
		t.Fatalf("default target %q", id)
	}
	if id, _ := target(ctx, "other"); id != "other" {
		t.Fatalf("explicit target %q", id)
	}
	if _, err := target(context.Background(), ""); err == nil {
		t.Fatal("no thread accepted")
	}
}

func TestFindProjectAndEnvironment(t *testing.T) {
	ctx := t.Context()
	db, err := localdb.Open(ctx, filepath.Join(t.TempDir(), "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	log, err := oplog.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	store := data.New(db, log)
	p, _ := store.Projects().Create(ctx, data.Project{Slug: "dory", Name: "Dory"})
	for _, name := range []string{p.ID, "dory", "DORY"} {
		if got, err := findProjectByName(ctx, store, name); err != nil || got.ID != p.ID {
			t.Fatalf("project %q: %+v %v", name, got, err)
		}
	}
	if _, err := findProjectByName(ctx, store, "nope"); err == nil {
		t.Fatal("unknown project accepted")
	}
	s := &api.Server{Node: api.Node{ID: "n1", Name: "Portátil"}}
	for _, name := range []string{"", "local", "n1", "portátil"} {
		if got, err := findEnvironment(ctx, s, name); err != nil || got != "n1" {
			t.Fatalf("environment %q: %q %v", name, got, err)
		}
	}
	if _, err := findEnvironment(ctx, s, "servidor"); err == nil {
		t.Fatal("unknown environment accepted")
	}
}

type selectionProvider struct{ runtime.Provider }

func (selectionProvider) ID() string { return "test" }

func (selectionProvider) Detect(context.Context) runtime.Status {
	return runtime.Status{ID: "test", Installed: true, Models: []runtime.Model{{Slug: "model", Name: "Test Model", Efforts: []string{"high"}}}}
}

func TestAPIAndMCPUseSameThreadSelection(t *testing.T) {
	ctx := t.Context()
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
	rt := runtime.NewManager(db, store, log.Node(), dir, state, selectionProvider{})
	settings := api.NewSettingsStore(state, api.Settings{})
	s := &api.Server{Store: store, Runtime: rt, Node: api.Node{ID: log.Node()}, Settings: settings,
		Threads: &app.Threads{Store: store, Runtime: rt, Settings: settings, Node: log.Node(), DataDir: dir}}
	mux := http.NewServeMux()
	s.Register(mux)

	const selection = `{"provider":"test","model":"Test Model","effort":"HIGH"}`
	httpResponse := httptest.NewRecorder()
	mux.ServeHTTP(httpResponse, httptest.NewRequestWithContext(ctx, "POST", "/api/nodes/local/threads", strings.NewReader(selection)))
	var web peerapi.CreateThreadResponse
	if httpResponse.Code != http.StatusOK || json.Unmarshal(httpResponse.Body.Bytes(), &web) != nil || web.Thread == nil {
		t.Fatalf("API response %d: %s", httpResponse.Code, httpResponse.Body)
	}

	const secret = "test-agent-secret"
	handler := auth.Middleware(nil, config.Config{}, secret)(Handler(s))
	req := httptest.NewRequestWithContext(ctx, "POST", "/mcp?thread="+web.ThreadID, strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"thread_create","arguments":`+selection+`}}`))
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+auth.AgentMCPToken(secret, web.ThreadID))
	mcpResponse := httptest.NewRecorder()
	handler.ServeHTTP(mcpResponse, req)
	var reply struct {
		Result struct {
			Content []struct{ Text string }
			IsError bool
		}
	}
	if mcpResponse.Code != http.StatusOK || json.Unmarshal(mcpResponse.Body.Bytes(), &reply) != nil || reply.Result.IsError || len(reply.Result.Content) != 1 {
		t.Fatalf("MCP response %d: %s", mcpResponse.Code, mcpResponse.Body)
	}
	var created peerapi.CreateThreadResponse
	if err := json.Unmarshal([]byte(reply.Result.Content[0].Text), &created); err != nil {
		t.Fatal(err)
	}
	got, err := store.Threads().Get(ctx, created.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != web.Thread.Provider || got.Model != web.Thread.Model || got.Effort != web.Thread.Effort || got.Model != "model" || got.Effort != "high" {
		t.Fatalf("MCP selection %+v, API selection %+v", got, *web.Thread)
	}
}
