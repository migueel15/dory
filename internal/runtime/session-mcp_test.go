package runtime

import (
	"context"
	"github.com/migueel15/dory/internal/harness/claude"
	"github.com/migueel15/dory/internal/harness/codex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/config"
	"github.com/migueel15/dory/internal/data"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPrepareMCP(t *testing.T) {
	const secret = "test-node-secret"
	srv := sdk.NewServer(&sdk.Implementation{Name: "dory", Version: "test"}, nil)
	sdk.AddTool(srv, &sdk.Tool{Name: "browser_tabs"}, func(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, any, error) {
		return nil, struct{}{}, nil
	})
	mcp := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	server := httptest.NewServer(auth.Middleware(nil, config.Config{}, secret)(mcp))
	defer server.Close()

	m := NewManager(nil, nil, "", t.TempDir(), testNodeState(t), nil, codex.Codex{}, claude.Claude{})
	m.MCPURL, m.MCPToken = server.URL+"/mcp", secret
	turn := Turn{ThreadID: "one", Text: "continúa"}
	if err := m.prepareMCP(t.Context(), data.Thread{ID: "one", Provider: "codex"}, &turn); err != nil {
		t.Fatal(err)
	}
	if turn.Text != "continúa" || !strings.HasPrefix(turn.Instructions, "# Dory") || strings.Contains(turn.Instructions, "## Browser") {
		t.Fatalf("instructions must travel apart from the prompt: %q", turn.Text)
	}
	if _, err := checkMCP(t.Context(), server.URL+"/mcp?thread=two", turn.MCPToken); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("credential accepted for the wrong thread: %v", err)
	}
	server.Close()
	if err := m.prepareMCP(t.Context(), data.Thread{ID: "one", Provider: "claude"}, &turn); err == nil || !strings.Contains(err.Error(), "claude: MCP de Dory") {
		t.Fatalf("unexpected connection error: %v", err)
	}
}

func TestBuildInstructions(t *testing.T) {
	got := buildInstructions("claude", "opus", []string{"subagent_spawn", "subagent_status", "subagent_cancel", "device_list"}, true)
	if !strings.Contains(got, "through claude, as opus") || !strings.Contains(got, "ToolSearch") {
		t.Fatalf("missing runtime header: %q", got)
	}
	if !strings.Contains(got, "## Subagents") {
		t.Fatal("offered tools lack their guidance")
	}
	if strings.Contains(got, "## Devices") || strings.Contains(got, "## Threads") {
		t.Fatal("guidance sent for tools the catalog does not offer")
	}
	if strings.Contains(buildInstructions("codex", "", nil, false), "ToolSearch") {
		t.Fatal("ToolSearch hint is only for Claude Code")
	}
}
