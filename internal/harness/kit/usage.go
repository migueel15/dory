package kit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
)

// usageTimeout bounds one usage query, process start included.
const usageTimeout = 45 * time.Second

// windowKind names a window by its length with the kinds Claude Code uses,
// so the interface labels every provider the same way.
func WindowKind(minutes int) string {
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

func SortWindows(ws []harness.Window) []harness.Window {
	sort.Slice(ws, func(i, j int) bool { return ws[i].Kind < ws[j].Kind })
	return ws
}

// ask starts cmd, writes each request as a line on its stdin and feeds every
// stdout line to read until it says it is done. It then closes the process.
func Ask(ctx context.Context, cmd *exec.Cmd, requests []string, read func(line []byte) (bool, error)) error {
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
