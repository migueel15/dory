//go:build !android

package main

import (
	"os"
	"path/filepath"

	"github.com/migueel15/dory/node/spikes/wails-android/internal/checks"
)

func newEnv() *checks.Env {
	dir := filepath.Join(os.TempDir(), "dory-wails-spike")
	_ = os.MkdirAll(dir, 0o700)
	return &checks.Env{DataDir: dir}
}
