package runtime

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
)

// Usage asks the official agy binary for the quota of the signed-in account.
// Dory never reads Antigravity credentials nor calls Google directly: agy
// answers "/quota" without a model and prints one TSV row per window.
func (a Antigravity) Usage(ctx context.Context) ([]Window, error) {
	bin, err := exec.LookPath(a.bin())
	if err != nil {
		return nil, ErrNoUsage
	}
	ctx, cancel := context.WithTimeout(ctx, usageTimeout)
	defer cancel()
	out, err := cmdexec.CommandContext(ctx, bin, "-p", "/quota").Output()
	if err != nil {
		return nil, ErrNoUsage
	}
	windows := parseAgyQuota(out)
	if len(windows) == 0 {
		return nil, ErrNoUsage
	}
	return windows, nil
}

// parseAgyQuota reads rows "group\tlabel\tNN%\treset". The format is not a
// documented contract, so unknown rows are skipped instead of failing.
func parseAgyQuota(raw []byte) []Window {
	seen := map[string]bool{}
	var out []Window
	for line := range bytes.Lines(raw) {
		fields := strings.Split(strings.TrimSpace(string(line)), "\t")
		if len(fields) != 4 {
			continue
		}
		group, label := strings.TrimSpace(fields[0]), strings.ToLower(fields[1])
		pct, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(fields[2]), "%"), 64)
		if group == "" || err != nil || pct < 0 || pct > 100 {
			continue
		}
		reset, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(fields[3]))
		if err != nil {
			continue
		}
		suffix, name, minutes := "", "", 0
		switch {
		case strings.Contains(label, "five hour") || strings.Contains(label, "5 hour"):
			suffix, name, minutes = "5h", "5 horas", 5*60
		case strings.Contains(label, "week"):
			suffix, name, minutes = "weekly", "Semanal", 7*24*60
		default:
			// Translated labels: guess the window from the time left to reset.
			if time.Until(reset) > 6*time.Hour {
				suffix, name, minutes = "weekly", "Semanal", 7*24*60
			} else {
				suffix, name, minutes = "5h", "5 horas", 5*60
			}
		}
		kind := strings.Join(strings.Fields(strings.ToLower(group)), "-") + "-" + suffix
		if seen[kind] {
			continue
		}
		seen[kind] = true
		out = append(out, Window{Kind: kind, Label: group + " · " + name, Minutes: minutes, Utilization: 1 - pct/100, ResetsAt: reset.UTC()})
	}
	return sortWindows(out)
}
