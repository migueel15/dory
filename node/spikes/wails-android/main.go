// Dory spike: Wails v3 on Android. See README.md.
package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/migueel15/dory/node/spikes/wails-android/internal/checks"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	static, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	env := newEnv()
	checks.TraceDir = env.DataDir
	app := checks.NewApp(env)
	// The mTLS listener must be up from launch so the desktop can reach it
	// before anyone presses a button in the UI.
	if err := app.MTLS.Start(); err != nil {
		log.Printf("mtls start: %v", err)
	}
	app.HB.Start()

	wapp := application.New(application.Options{
		Name:        "dory-spike",
		Description: "Wails v3 Android spike",
		Assets:      application.AssetOptions{Handler: app.Handler(static)},
	})
	wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Dory spike", Width: 420, Height: 800, URL: "/",
	})
	if err := wapp.Run(); err != nil {
		log.Fatal(err)
	}
}
