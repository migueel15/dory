//go:build !android

package main

import (
	"os"
	"path/filepath"
)

// dataDir lets the app run on a desktop for development.
func dataDir() string {
	if d := os.Getenv("DORY_ANDROID_DATA_DIR"); d != "" {
		return d
	}
	d := filepath.Join(os.TempDir(), "dory-android-dev")
	_ = os.MkdirAll(d, 0o700)
	return d
}

func keepAlive() {}
