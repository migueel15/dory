package runtime

import (
	"context"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
)

// OpenCode runs each turn on its own "opencode serve" (opencode_server.go);
// a later turn continues the same session.
type OpenCode struct{ Bin string }

func (OpenCode) ID() string { return "opencode" }

func (o OpenCode) bin() string {
	if o.Bin != "" {
		return o.Bin
	}
	return resolveBin("opencode")
}

func (o OpenCode) Detect(ctx context.Context) Status {
	version, ok := detect(ctx, o.bin())
	st := Status{ID: "opencode", Name: "OpenCode", Installed: ok, Version: version, Capabilities: []string{"interrupt", "resume"}}
	if !ok {
		return st
	}
	// Models depend on the providers configured in OpenCode: ask it.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, _ := cmdexec.CommandContext(ctx, o.bin(), "models").Output()
	st.Models = append(st.Models, Model{Slug: "", Name: "Por defecto"})
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			st.Models = append(st.Models, Model{Slug: line, Name: line})
		}
	}
	return st
}
