// Package mcp exposes Dory to the agents of this node over streamable HTTP.
// Agents read everything they need about the work in progress; they write
// only what is theirs: memories, plans, questions and tasks. The kanban
// column is derived; the only state they set is the stage of a task, with
// the same rules as the interface.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/browser"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/devices"
	"github.com/migueel15/dory/internal/mesh"
	"github.com/migueel15/dory/internal/runtime"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type threadKey struct{}

type sdkServer = sdk.Server

// add registers a tool. Tools whose name starts with a read verb need the
// read scope; the rest need write. Results are JSON text.
// receiptDrops are the long or empty fields a write echoes back: the agent
// just sent that text, so repeating it only spends tokens. Read tools such as
// get_task, plan_read and recall return it in full.
var receiptDrops = map[string]bool{"description": true, "content": true, "body_md": true, "created_at": true, "archived_at": true, "position": true}

// receipt shortens a write tool's JSON result to what identifies the change.
func receipt(raw []byte) []byte {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return raw
	}
	var trim func(any) any
	trim = func(v any) any {
		switch v := v.(type) {
		case map[string]any:
			for k, field := range v {
				if receiptDrops[k] || field == nil || field == "" {
					delete(v, k)
					continue
				}
				v[k] = trim(field)
			}
		case []any:
			for i := range v {
				v[i] = trim(v[i])
			}
		}
		return v
	}
	out, err := json.Marshal(trim(value))
	if err != nil {
		return raw
	}
	return out
}

// toolError is the result of a failed tool: the agent reads "<code>: <message>"
// and any parameters, whatever the layer the error comes from.
func toolError(err error) *sdk.CallToolResult {
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: apperr.Format(err)}}}
}

func add[I any](server *sdk.Server, name, description string, fn func(context.Context, I) (any, error)) {
	sdk.AddTool(server, &sdk.Tool{Name: name, Description: description}, func(ctx context.Context, _ *sdk.CallToolRequest, in I) (*sdk.CallToolResult, any, error) {
		scope := "write"
		for _, prefix := range []string{"list_", "get_", "search", "recall", "session_start", "plan_read", "browser_tabs", "browser_read", "browser_screenshot", "browser_console", "html_preview", "render_html"} {
			if strings.HasPrefix(name, prefix) {
				scope = "read"
			}
		}
		if !auth.Allowed(ctx, scope) {
			return toolError(apperr.Newf(apperr.Forbidden, "the %s scope is required", scope)), nil, nil
		}
		value, err := fn(ctx, in)
		if err != nil {
			return toolError(err), nil, nil
		}
		if p, ok := value.(previewResult); ok {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.ImageContent{Data: p.png, MIMEType: "image/png"}, &sdk.TextContent{Text: p.info}}}, nil, nil
		}
		if img, ok := value.(pngImage); ok {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.ImageContent{Data: img, MIMEType: "image/png"}}}, nil, nil
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, nil, err
		}
		if scope == "write" && !strings.HasPrefix(name, "browser_") && !strings.HasPrefix(name, "device_") {
			raw = receipt(raw)
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(raw)}}}, nil, nil
	})
}

// Node identifies this node to the tools.
type Node struct {
	ID   string
	Name string
}

// Deps is what the tools need. The MCP depends on the application layer, not
// on the HTTP server.
type Deps struct {
	// Store is read by the thread, link and search tools that have no service yet.
	Store    *data.Store
	Node     Node
	Threads  *app.Threads
	Tasks    *app.Tasks
	Plans    *app.Plans
	Memories *app.Memories
	Projects *app.Projects
	Session  *app.Session
	Runtime  *runtime.Manager
	// Peers lists the mesh nodes; nil when there is no mesh.
	Peers PeerLister
	// PeerClient calls another node.
	PeerClient *mesh.Client
	// Devices bridges Android devices; nil disables the device tools.
	Devices *devices.Manager
	// Browser runs the browsers of the threads; nil disables them.
	Browser *browser.Manager
	// AgentsUseDevices says whether the node setting lets agents drive devices.
	AgentsUseDevices func() bool
	// Notify tells the open clients about something that is not a stored change.
	Notify func(entity, id string, data any)
}

// Handler serves the tools. The caller authenticates the request; a thread
// passes its id as ?thread= so questions reach the right conversation.
func Handler(s *Deps) http.Handler {
	plain, withDevices := build(s, false), build(s, true)
	h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server {
		// The device tools exist only while the node setting allows agents to use devices.
		if s.Devices != nil && s.AgentsUseDevices != nil && s.AgentsUseDevices() {
			return withDevices
		}
		return plain
	}, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := r.URL.Query().Get("thread"); id != "" {
			r = r.WithContext(context.WithValue(r.Context(), threadKey{}, id))
		}
		h.ServeHTTP(w, r)
	})
}

func build(s *Deps, devices bool) *sdk.Server {
	srv := sdk.NewServer(&sdk.Implementation{Name: "dory", Version: "2.0.0"}, nil)
	addTaskTools(srv, s)
	addPlanTools(srv, s)
	addMemoryTools(srv, s)
	addUITools(srv, s)
	addServiceTools(srv, s)
	addThreadTools(srv, s)
	addBrowser(srv, s)
	if devices {
		addDeviceTools(srv, s)
	}
	return srv
}

// threadID returns the given thread or the one of this conversation.
func threadID(ctx context.Context, id string) (string, error) {
	if thread := auth.AgentThread(ctx); thread != "" && id != "" && id != thread {
		return "", apperr.New(apperr.Forbidden, "the MCP credential is bound to another thread")
	}
	if id == "" {
		id, _ = ctx.Value(threadKey{}).(string)
	}
	if id == "" {
		return "", apperr.New(apperr.Invalid, "thread_id is required outside a Dory thread")
	}
	return id, nil
}
