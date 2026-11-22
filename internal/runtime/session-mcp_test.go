package runtime

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/config"
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

	m := Manager{MCPURL: server.URL + "/mcp", MCPToken: secret}
	turn := Turn{ThreadID: "one", Text: "continúa"}
	if err := m.prepareMCP(t.Context(), "codex", &turn); err != nil {
		t.Fatal(err)
	}
	if turn.Text != "continúa" || turn.Instructions != doryInstructions {
		t.Fatalf("instructions must travel apart from the prompt: %q", turn.Text)
	}
	if err := checkMCP(t.Context(), server.URL+"/mcp?thread=two", turn.MCPToken); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("credential accepted for the wrong thread: %v", err)
	}
	server.Close()
	if err := m.prepareMCP(t.Context(), "claude", &turn); err == nil || !strings.Contains(err.Error(), "claude: MCP de Dory") {
		t.Fatalf("unexpected connection error: %v", err)
	}
}

func TestOpenCodeRegisterDory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "PUT /api/experimental/mcp/dory":
			var body struct {
				Config struct {
					URL     string
					Headers map[string]string
				}
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
				body.Config.URL != "http://localhost/mcp?thread=one" ||
				body.Config.Headers["Authorization"] != "Bearer thread-token" {
				t.Error("incorrect OpenCode MCP configuration")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/mcp":
			io.WriteString(w, `{"data":[{"name":"dory","status":{"status":"connected"}}]}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	api := openCodeAPI{base: server.URL}
	if err := api.registerDory(t.Context(), Turn{MCPURL: "http://localhost/mcp?thread=one", MCPToken: "thread-token"}); err != nil {
		t.Fatal(err)
	}
}

func TestPromptWithInstructions(t *testing.T) {
	turn := Turn{Text: "hola", Instructions: "guía"}
	if got := promptWithInstructions(turn); got != "<dory_instructions>\nguía\n</dory_instructions>\n\nhola" {
		t.Fatalf("new session: %q", got)
	}
	turn.Session = "s1"
	if got := promptWithInstructions(turn); got != "hola" {
		t.Fatalf("resumed session repeats instructions: %q", got)
	}
}
