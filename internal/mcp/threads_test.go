package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/devices"
)

func TestThreadControlTools(t *testing.T) {
	tools := toolsOf(t, build(&api.Server{Devices: devices.NewManager()}, false))
	for _, n := range []string{"list_models", "list_threads", "get_thread", "thread_configure", "thread_send", "thread_stop", "thread_spawn"} {
		if tools[n] == nil {
			t.Errorf("missing %s", n)
		}
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
