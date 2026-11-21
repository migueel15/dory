package devices

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// ListAVDs returns the names of the emulators created on this node.
func ListAVDs(ctx context.Context, t Tools) []string {
	if t.Emulator == "" {
		return []string{}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, t.Emulator, "-list-avds")
	cmd.Stdout = &out
	_ = cmd.Run()
	names := []string{}
	for _, l := range strings.Split(out.String(), "\n") {
		l = strings.TrimSpace(l)
		// The emulator prints log lines (INFO |...) next to the names.
		if l != "" && !strings.Contains(l, " | ") && !strings.Contains(l, " ") {
			names = append(names, l)
		}
	}
	return names
}

// AVDName asks a running emulator which AVD it runs.
func (a ADB) AVDName(ctx context.Context, serial string) string {
	out, err := a.Run(ctx, 4*time.Second, "-s", serial, "emu", "avd", "name")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && l != "OK" {
			return l
		}
	}
	return ""
}

type avdProc struct{}
