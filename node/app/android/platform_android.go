//go:build android

package main

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// dataDir is activity.getFilesDir(): private to the app, not backed up.
func dataDir() string { return application.Android.StoragePath() }

var fgOnce sync.Once

// keepAlive starts the foreground service so the peer listener and the sync
// connections survive while the app is in the background.
func keepAlive() {
	fgOnce.Do(func() {
		application.Android.StartForegroundService(`{"title":"Dory","text":"Nodo activo, sincronizando"}`)
	})
}
