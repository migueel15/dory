package runtime

import (
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

type Message = harness.Message
type Window = harness.Window
type Turn = harness.Turn
type ReportedSettings = harness.ReportedSettings
type Model = harness.Model
type Status = harness.Status
type Provider = harness.Driver

func now() string                                { return kit.Now() }
func threadEnv(env []string, id string) []string { return kit.ThreadEnv(env, id) }
