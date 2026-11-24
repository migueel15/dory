package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/apperr"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolErrorCarriesCodeMessageAndParams(t *testing.T) {
	res := toolError(apperr.New(apperr.Conflict, "plan changed").With("revision", 3))
	if !res.IsError {
		t.Fatal("a failed tool must be flagged as an error")
	}
	text := res.Content[0].(*sdk.TextContent).Text
	if text != `conflict: plan changed {"revision":3}` {
		t.Fatalf("text = %q", text)
	}
}

func TestAddReportsDomainErrorsAsToolResults(t *testing.T) {
	ctx := t.Context()
	server := sdk.NewServer(&sdk.Implementation{Name: "test"}, nil)
	add(server, "get_thing", "fails", func(_ context.Context, _ struct{}) (any, error) {
		return nil, apperr.New(apperr.NotFound, "no such thing")
	})
	serverT, clientT := sdk.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverT, nil); err != nil {
		t.Fatal(err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "c"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	// Without an identity the call is refused before the tool runs.
	res, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get_thing", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("a refused tool is a result, not a protocol error: %v", err)
	}
	if text := res.Content[0].(*sdk.TextContent).Text; !res.IsError || !strings.HasPrefix(text, "forbidden: ") {
		t.Fatalf("result = %v %q", res.IsError, text)
	}
}
