package claude

import (
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

// LineParser returns a function that turns the stream-json lines of one Claude
// Code turn into events. Tests and recorded-session players use it.
func LineParser(run string) func(line []byte, emit func(harness.Event)) {
	p := &claudeParser{run: run, texts: map[string]*kit.Text{}, tools: map[string]*harness.Message{}}
	return p.parse
}
