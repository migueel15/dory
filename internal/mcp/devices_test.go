package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/devices"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func toolsOf(t *testing.T, srv *sdk.Server) map[string]*sdk.Tool {
	t.Helper()
	ct, st := sdk.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "t"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*sdk.Tool{}
	for _, tool := range res.Tools {
		out[tool.Name] = tool
	}
	return out
}

func TestDeviceToolsOnlyWhenEnabled(t *testing.T) {
	s := &api.Server{Devices: devices.NewManager()}
	for name := range toolsOf(t, build(s, false)) {
		if strings.HasPrefix(name, "device_") {
			t.Errorf("%s should be hidden", name)
		}
	}
	tools := toolsOf(t, build(s, true))
	for _, n := range []string{"device_list", "device_open", "device_close", "device_screenshot", "device_ui", "device_tap", "device_swipe", "device_type", "device_key", "device_launch", "device_open_url", "device_apps"} {
		if tools[n] == nil {
			t.Errorf("missing %s", n)
		}
	}
	raw, _ := json.Marshal(tools["device_tap"].InputSchema)
	for _, f := range []string{`"serial"`, `"index"`, `"text"`, `"x"`} {
		if !strings.Contains(string(raw), f) {
			t.Errorf("device_tap schema lacks %s: %s", f, raw)
		}
	}
}
