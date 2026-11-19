package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
)

// ErrNoUsage is what Provider.Usage returns when a harness does not report
// its usage.
var ErrNoUsage = errors.New("runtime: provider does not report usage")

// usageTimeout bounds one usage query, process start included.
const usageTimeout = 45 * time.Second

// Usage sends Claude Code the get_usage control request, which answers
// without a turn and costs nothing.
func (c Claude) Usage(ctx context.Context) ([]Window, error) {
	request := `{"type":"control_request","request_id":"dory-usage","request":{"subtype":"get_usage"}}`
	var out []Window
	err := ask(ctx, cmdexec.Command(c.bin(), "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose"), []string{request},
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
				out = append(out, Window{Kind: kind, Utilization: *w.Utilization / 100, ResetsAt: reset.UTC()})
			}
			return true, nil
		})
	return sortWindows(out), err
}

// Usage reads the account rate limits from codex app-server.
func (c Codex) Usage(ctx context.Context) ([]Window, error) {
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
	var out []Window
	err := ask(ctx, cmdexec.Command(c.bin(), "app-server"), requests, func(line []byte) (bool, error) {
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
				out = append(out, Window{Kind: windowKind(w.WindowDurationMins), Utilization: w.UsedPercent / 100, ResetsAt: time.Unix(w.ResetsAt, 0).UTC()})
			}
		}
		return true, nil
	})
	return sortWindows(out), err
}

func (OpenCode) Usage(context.Context) ([]Window, error) { return nil, ErrNoUsage }

// windowKind names a window by its length with the kinds Claude Code uses,
// so the interface labels every provider the same way.
func windowKind(minutes int) string {
	switch minutes {
	case 300:
		return "five_hour"
	case 7 * 24 * 60:
		return "seven_day"
	case 30 * 24 * 60:
		return "thirty_day"
	}
	return fmt.Sprintf("%d_min", minutes)
}

func sortWindows(ws []Window) []Window {
	sort.Slice(ws, func(i, j int) bool { return ws[i].Kind < ws[j].Kind })
	return ws
}

// ask starts cmd, writes each request as a line on its stdin and feeds every
// stdout line to read until it says it is done. It then closes the process.
func ask(ctx context.Context, cmd *exec.Cmd, requests []string, read func(line []byte) (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, usageTimeout)
	defer cancel()
	full := cmdexec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	full.Dir = cmd.Dir
	full.Env = cmd.Env
	full.WaitDelay = 5 * time.Second
	in, err := full.StdinPipe()
	if err != nil {
		return err
	}
	out, err := full.StdoutPipe()
	if err != nil {
		return err
	}
	if err = full.Start(); err != nil {
		return err
	}
	defer func() {
		in.Close()
		cancel()
		full.Wait()
	}()
	if _, err = io.WriteString(in, strings.Join(requests, "\n")+"\n"); err != nil {
		return err
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	for scanner.Scan() {
		if done, err := read(scanner.Bytes()); done {
			return err
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.New("runtime: usage query ended without an answer")
}
