//go:build android

package main

import (
	"github.com/migueel15/dory/node/spikes/wails-android/internal/checks"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func newEnv() *checks.Env {
	return &checks.Env{
		DataDir: application.Android.StoragePath(), // activity.getFilesDir()
		SetForeground: func(on bool, title, text string) {
			if on {
				application.Android.StartForegroundService(`{"title":"` + title + `","text":"` + text + `"}`)
			} else {
				application.Android.StopForegroundService()
			}
		},
	}
}
