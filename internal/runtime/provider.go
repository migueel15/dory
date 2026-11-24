package runtime

import (
	"context"
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
func interruptible(cmd *exec.Cmd)                 { kit.Interruptible(cmd) }
func stopGroup(cmd *exec.Cmd, sig syscall.Signal) { kit.StopGroup(cmd, sig) }

func providerCommand(ctx context.Context, thread, bin string, args ...string) *exec.Cmd {
	return kit.ProviderCommand(ctx, thread, bin, args...)
}

func skillEntries(root, prefix string) []Entry { return kit.SkillEntries(root, prefix) }

const externalLimit = kit.ExternalLimit
const usageTimeout = kit.UsageTimeout
