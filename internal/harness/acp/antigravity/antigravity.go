// Package antigravity is the ACP profile of Google's Antigravity CLI (agy):
// everything that is specific to it, around the generic adapter of package acp.
package antigravity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/acp"
	"github.com/migueel15/dory/internal/update"
)

// ID is the provider id of Antigravity.
const ID = "antigravity"

func installDir(goos, home, localAppData string) string {
	if goos == "windows" {
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "agy", "bin")
	}
	return filepath.Join(home, ".local", "bin")
}

// Profile returns the ACP profile of Antigravity. bin, when not empty, is the
// executable to run instead of the one found in PATH or its install directory.
func Profile(bin string) acp.Profile {
	bins := []string{"agy"}
	if bin != "" {
		bins = []string{bin}
	} else {
		home, _ := os.UserHomeDir()
		bins = append(bins, filepath.Join(installDir(runtime.GOOS, home, os.Getenv("LOCALAPPDATA")), "agy"))
	}
	return acp.Profile{
		ID: ID, Name: "Antigravity", Order: 3,
		Bins:         bins,
		Args:         []string{"acp"},
		SandboxArgs:  []string{"--sandbox"},
		Agents:       []string{"antigravity-acp", "Antigravity ACP Server"},
		RequireLoad:  true,
		Login:        []string{"acp", "--login"},
		Update:       []string{"update"},
		Install:      install(runtime.GOOS),
		DefaultModel: harness.Model{Slug: "antigravity-default", Name: "Predeterminado de Antigravity"},
		Catalog:      true,
		Sessions:     true,
		Account:      "Google",
		Capabilities: harness.Capabilities{Resume: true, MidTurnRequests: true, UsageReport: true, UsageSnapshot: true, LoginCode: true, SystemPrompt: true},
		ProcessNames: []string{"agy"},
		SystemPrompt: systemPrompt,
		LatestVersion: func(ctx context.Context) string {
			return update.ManifestLatest(ctx, "https://antigravity-cli-auto-updater-974169037036.us-central1.run.app/manifests/")
		},
		Usage:     quota,
		History:   history,
		TurnUsage: turnUsage,
		Commands:  commands,
		SkillDir:  skillDir,
	}
}

func install(goos string) []string {
	if goos == "windows" {
		return []string{"powershell", "-NoProfile", "-Command", `$installer = Join-Path ([System.IO.Path]::GetTempPath()) ("agy-" + [guid]::NewGuid() + ".cmd"); try { Invoke-WebRequest -ErrorAction Stop -UseBasicParsing https://antigravity.google/cli/install.cmd -OutFile $installer; & $installer; exit $LASTEXITCODE } finally { Remove-Item -LiteralPath $installer -ErrorAction SilentlyContinue }`}
	}
	return []string{"sh", "-c", `installer=$(mktemp) || exit; trap 'rm -f "$installer"' EXIT; curl -fsSL https://antigravity.google/cli/install.sh -o "$installer" && bash "$installer"`}
}

// systemPrompt appends the session instructions through the _meta extension
// agy advertises, so they stay out of the visible prompt.
func systemPrompt(meta []byte, instructions string) (map[string]any, bool) {
	var m struct {
		Antigravity struct {
			SystemPrompt struct{ Append bool }
		}
	}
	if json.Unmarshal(meta, &m) != nil || !m.Antigravity.SystemPrompt.Append {
		return nil, false
	}
	return map[string]any{"antigravity": map[string]any{"systemPrompt": map[string]any{"append": instructions}}}, true
}

func history(p harness.Paths, from time.Time) []harness.UsageEvent {
	return usageEvents(p.Home, from)
}

func turnUsage(session string, start, end time.Time) []harness.StepUsage {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return sessionUsage(home, session, start, end)
}
