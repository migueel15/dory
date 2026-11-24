package runtime

import (
	"context"
	"errors"
	"github.com/migueel15/dory/internal/harness/claude"
	"github.com/migueel15/dory/internal/harness/codex"
	"github.com/migueel15/dory/internal/harness/opencode"
	"os"
	"path/filepath"
	"testing"
)

// fakeBin writes a shell script that reads stdin and prints out.
func fakeBin(t *testing.T, out string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake")
	script := "#!/bin/sh\nread line\ncat <<'EOF'\n" + out + "\nEOF\nsleep 5\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestClaudeUsage(t *testing.T) {
	bin := fakeBin(t, `{"type":"system","subtype":"init"}
{"type":"control_response","response":{"subtype":"success","request_id":"dory-usage","response":{"rate_limits":{"five_hour":{"utilization":26,"resets_at":"2026-11-19T01:50:00.859025+00:00"},"seven_day":{"utilization":53,"resets_at":"2026-11-20T08:00:00+00:00"},"seven_day_opus":null,"pace":"steady","extra_usage":{"is_enabled":false,"utilization":null}}}}}`)
	ws, err := claude.Claude{Bin: bin}.Usage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 || ws[0].Kind != "five_hour" || ws[0].Utilization != 0.26 || ws[1].Kind != "seven_day" || ws[1].ResetsAt.IsZero() {
		t.Fatalf("windows = %+v", ws)
	}
}

func TestCodexUsage(t *testing.T) {
	bin := fakeBin(t, `{"id":1,"result":{}}
{"id":2,"result":{"rateLimits":{"primary":{"usedPercent":0,"windowDurationMins":300,"resetsAt":1795061771},"secondary":{"usedPercent":17,"windowDurationMins":10080,"resetsAt":1795120134}}}}`)
	ws, err := codex.Codex{Bin: bin}.Usage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 || ws[0].Kind != "five_hour" || ws[1].Kind != "seven_day" || ws[1].Utilization != 0.17 {
		t.Fatalf("windows = %+v", ws)
	}
}

func TestUsageNotReported(t *testing.T) {
	if _, err := (opencode.OpenCode{}).Usage(context.Background()); !errors.Is(err, ErrNoUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestDisabledProvider(t *testing.T) {
	state := testNodeState(t)
	m := NewManager(nil, nil, "n", t.TempDir(), state, nil, opencode.OpenCode{})
	if err := m.SetEnabled(context.Background(), "opencode", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.provider("opencode"); ok {
		t.Fatal("disabled provider still usable")
	}
	again := NewManager(nil, nil, "n", m.dataDir, state, nil, opencode.OpenCode{})
	if again.Enabled("opencode") {
		t.Fatal("disabled provider not saved")
	}
	if err := again.SetEnabled(context.Background(), "opencode", true); err != nil || !again.Enabled("opencode") {
		t.Fatalf("enable: %v", err)
	}
}
