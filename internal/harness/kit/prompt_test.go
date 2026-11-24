package kit

import (
	"slices"
	"testing"

	"github.com/migueel15/dory/internal/harness"
)

func TestThreadEnvKeepsDefaultEnvironment(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	env := ThreadEnv(nil, "t1")
	if !slices.Contains(env, "PATH=/usr/bin") || !slices.Contains(env, "DORY_THREAD_ID=t1") {
		t.Fatalf("env = %v", env)
	}
}

func TestPromptWithInstructions(t *testing.T) {
	turn := harness.Turn{Text: "hola", Instructions: "guía"}
	if got := PromptWithInstructions(turn); got != "<dory_instructions>\nguía\n</dory_instructions>\n\nhola" {
		t.Fatalf("new session: %q", got)
	}
	turn.Session = "s1"
	if got := PromptWithInstructions(turn); got != "hola" {
		t.Fatalf("resumed session repeats instructions: %q", got)
	}
}
