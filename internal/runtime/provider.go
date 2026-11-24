package runtime

import (
	"context"
	"io"
	"os/exec"
	"syscall"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

type Message = harness.Message
type Window = harness.Window
type ReportedSettings = harness.ReportedSettings
type Turn = harness.Turn
type Model = harness.Model
type Status = harness.Status
type Provider = harness.Driver

func now() string                                 { return kit.Now() }
func short(s string, n int) string                { return kit.Short(s, n) }
func clip(s string, n int) string                 { return kit.Clip(s, n) }
func threadEnv(env []string, id string) []string  { return kit.ThreadEnv(env, id) }
func promptWithInstructions(t Turn) string        { return kit.PromptWithInstructions(t) }
func interruptible(cmd *exec.Cmd)                 { kit.Interruptible(cmd) }
func stopGroup(cmd *exec.Cmd, sig syscall.Signal) { kit.StopGroup(cmd, sig) }

type limited struct {
	w io.Writer
	n int
}

func (l *limited) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		l.n -= k
		l.w.Write(p[:k])
	}
	return len(p), nil
}

func providerCommand(ctx context.Context, thread, bin string, args ...string) *exec.Cmd {
	return kit.ProviderCommand(ctx, thread, bin, args...)
}

func detect(ctx context.Context, bin string) (string, bool) { return kit.Detect(ctx, bin) }

func skillEntries(root, prefix string) []Entry { return kit.SkillEntries(root, prefix) }

const externalLimit = kit.ExternalLimit
const usageTimeout = kit.UsageTimeout
