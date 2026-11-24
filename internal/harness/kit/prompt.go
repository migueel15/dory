package kit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
)

const AttachmentFooter = "Ficheros adjuntos (léelos desde su ruta):"

// promptWithInstructions is the fallback for a harness that cannot take
// session instructions: they precede only the first prompt of a new session,
// which the harness then keeps in its own history.
func PromptWithInstructions(t harness.Turn) string {
	if t.Instructions == "" || t.Session != "" {
		return t.Text
	}
	return "<dory_instructions>\n" + t.Instructions + "\n</dory_instructions>\n\n" + t.Text
}

// threadEnv marks a provider and its inherited descendants for the owning thread.
// A nil env is the default account's: it inherits Dory's own environment.
func ThreadEnv(env []string, id string) []string {
	if env == nil {
		env = os.Environ()
	}
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, "DORY_THREAD_ID=") {
			out = append(out, entry)
		}
	}
	if id != "" {
		out = append(out, "DORY_THREAD_ID="+id)
	}
	return out
}

// providerCommand places a turn outside the Dory service's systemd cgroup
// when a user manager is available. Detached children can then outlive it.
func ProviderCommand(ctx context.Context, threadID, bin string, args ...string) *exec.Cmd {
	if scope := UserScope(); threadID != "" && scope != "" {
		return cmdexec.CommandContext(ctx, scope, append([]string{"--user", "--scope", "--quiet", "--", bin}, args...)...)
	}
	return cmdexec.CommandContext(ctx, bin, args...)
}

func WithAttachments(text string, files []harness.Attachment) string {
	if len(files) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n\n" + AttachmentFooter)
	for _, f := range files {
		fmt.Fprintf(&b, "\n- %s (%s)", f.Path, f.Name)
	}
	return b.String()
}
