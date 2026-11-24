package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

func (c Claude) Session(ctx context.Context) (bool, string) {
	out, err := kit.WithEnv(cmdexec.CommandContext(ctx, c.bin(), "auth", "status"), c.Environ()).Output()
	var st struct {
		LoggedIn   bool   `json:"loggedIn"`
		AuthMethod string `json:"authMethod"`
	}
	if err != nil || json.Unmarshal(out, &st) != nil {
		return false, ""
	}
	return st.LoggedIn, st.AuthMethod
}
func (c Claude) LoginCommand() []string  { return []string{c.bin(), "auth", "login"} }
func (c Claude) UpdateCommand() []string { return []string{c.bin(), "update"} }

func (Claude) InstallCommand() []string {
	return kit.ShellInstall("https://claude.ai/install.sh", "https://claude.ai/install.ps1")
}

// Usage sends Claude Code the get_usage control request, which answers
// without a turn and costs nothing.
func (c Claude) Usage(ctx context.Context) ([]harness.Window, error) {
	request := `{"type":"control_request","request_id":"dory-usage","request":{"subtype":"get_usage"}}`
	var out []harness.Window
	err := kit.Ask(ctx, kit.WithEnv(cmdexec.Command(c.bin(), "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose"), c.Environ()), []string{request},
		func(line []byte) (bool, error) {
			var msg struct {
				Type     string `json:"type"`
				Response struct {
					Subtype   string `json:"subtype"`
					RequestID string `json:"request_id"`
					Error     string `json:"error"`
					Response  struct {
						// Not every entry is a window, so each one is decoded on its own.
						RateLimits map[string]json.RawMessage `json:"rate_limits"`
					} `json:"response"`
				} `json:"response"`
			}
			if json.Unmarshal(line, &msg) != nil || msg.Type != "control_response" || msg.Response.RequestID != "dory-usage" {
				return false, nil
			}
			if msg.Response.Subtype != "success" {
				return true, fmt.Errorf("claude get_usage: %s", msg.Response.Error)
			}
			for kind, raw := range msg.Response.Response.RateLimits {
				var w struct {
					Utilization *float64 `json:"utilization"`
					ResetsAt    string   `json:"resets_at"`
				}
				if json.Unmarshal(raw, &w) != nil || w.Utilization == nil || w.ResetsAt == "" {
					continue
				}
				reset, _ := time.Parse(time.RFC3339Nano, w.ResetsAt)
				out = append(out, harness.Window{Kind: kind, Utilization: *w.Utilization / 100, ResetsAt: reset.UTC()})
			}
			return true, nil
		})
	return kit.SortWindows(out), err
}
