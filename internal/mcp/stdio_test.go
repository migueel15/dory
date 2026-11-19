package mcp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRelayMCP(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "dory", Version: "test"}, nil)
	server.AddTool(&sdk.Tool{Name: "session_start", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, _ *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "context"}}}, nil
	})
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("thread") != "abc" || r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer endpoint.Close()
	for _, frame := range []struct{ message, want string }{
		{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"agy","version":"test"}}}`, `"name":"dory"`},
		{`{"jsonrpc":"2.0","method":"notifications/initialized"}`, ""},
		{`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"session_start","arguments":{}}}`, `"text":"context"`},
	} {
		out, err := relayMCP(t.Context(), endpoint.Client(), endpoint.URL+"/mcp?thread=abc", "test-token", []byte(frame.message))
		if err != nil || (frame.want == "" && len(out) != 0) || !bytes.Contains(out, []byte(frame.want)) {
			t.Fatalf("response %q, error %v", out, err)
		}
	}
}
