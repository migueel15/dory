package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

func (c Codex) Session(ctx context.Context) (bool, string) {
	out, err := kit.WithEnv(cmdexec.CommandContext(ctx, c.bin(), "login", "status"), c.Environ()).CombinedOutput()
	text := strings.TrimSpace(string(out))
	return err == nil && strings.HasPrefix(text, "Logged in"), text
}
func (c Codex) LoginCommand() []string  { return []string{c.bin(), "login", "--device-auth"} }
func (c Codex) UpdateCommand() []string { return []string{c.bin(), "update"} }

func (Codex) InstallCommand() []string {
	return kit.ShellInstall("https://chatgpt.com/codex/install.sh", "https://chatgpt.com/codex/install.ps1")
}

// Usage reads the account rate limits from codex app-server.
func (c Codex) Usage(ctx context.Context) ([]harness.Window, error) {
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"dory","title":"Dory","version":"2"}}}`,
		`{"jsonrpc":"2.0","method":"initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"account/rateLimits/read"}`,
	}
	type window struct {
		UsedPercent        float64 `json:"usedPercent"`
		WindowDurationMins int     `json:"windowDurationMins"`
		ResetsAt           int64   `json:"resetsAt"`
	}
	var out []harness.Window
	err := kit.Ask(ctx, kit.WithEnv(cmdexec.Command(c.bin(), "app-server"), c.Environ()), requests, func(line []byte) (bool, error) {
		var msg struct {
			ID     int                       `json:"id"`
			Error  *struct{ Message string } `json:"error"`
			Result struct {
				RateLimits struct {
					Primary   *window `json:"primary"`
					Secondary *window `json:"secondary"`
				} `json:"rateLimits"`
			} `json:"result"`
		}
		if json.Unmarshal(line, &msg) != nil || msg.ID != 2 {
			return false, nil
		}
		if msg.Error != nil {
			return true, fmt.Errorf("codex account/rateLimits/read: %s", msg.Error.Message)
		}
		for _, w := range []*window{msg.Result.RateLimits.Primary, msg.Result.RateLimits.Secondary} {
			if w != nil {
				out = append(out, harness.Window{Kind: kit.WindowKind(w.WindowDurationMins), Utilization: w.UsedPercent / 100, ResetsAt: time.Unix(w.ResetsAt, 0).UTC()})
			}
		}
		return true, nil
	})
	return kit.SortWindows(out), err
}
