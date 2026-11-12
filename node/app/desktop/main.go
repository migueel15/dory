// Command dory-desktop is the Dory local-first node as a Linux desktop app.
//
// The window serves the dory-node handler in-process (Wails AssetServer): no
// local HTTP port for the UI. Set DORY_DESKTOP_LOOPBACK=1 to serve the same
// handler on 127.0.0.1 and point the window there instead (fallback; see
// README.md).
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/migueel15/dory/node/nodeapp"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

func dataDir() string {
	if d := os.Getenv("DORY_DESKTOP_DATA_DIR"); d != "" {
		return d
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "dory-desktop")
}

func main() {
	opts := nodeapp.Options{DataDir: dataDir(), PeerPort: 7710, ConfirmViaUI: true}
	if v := os.Getenv("DORY_DESKTOP_PEER_PORT"); v != "" {
		opts.PeerPort, _ = strconv.Atoi(v)
	}
	loopback := os.Getenv("DORY_DESKTOP_LOOPBACK") == "1"
	if loopback {
		opts.Listen = "127.0.0.1:0"
	}
	node, err := nodeapp.Start(context.Background(), opts)
	if err != nil {
		log.Fatal(err)
	}
	defer node.Stop()
	log.Printf("device %s (%s), data in %s, peer port %s", node.Identity.DeviceID(), node.Identity.Name, opts.DataDir, node.PeerAddr)

	h := node.Handler()
	if os.Getenv("DORY_DESKTOP_TRACE") == "1" {
		h = trace(h)
	}
	app := application.New(application.Options{
		Name:        "Dory",
		Description: "Dory local-first node",
		Assets:      application.AssetOptions{Handler: h},
	})
	root := "/"
	nodeURL := "/node/"
	if loopback {
		root = "http://" + node.HTTPAddr + "/"
		nodeURL = root + "node/"
		log.Printf("loopback UI at %s", root)
	}
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Dory", Width: 1280, Height: 800, URL: root,
	})
	// Closing the window quits the app (and stops the node).
	win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { app.Quit() })

	menu := app.NewMenu()
	dory := menu.AddSubmenu("Dory")
	dory.Add("Inicio").OnClick(func(*application.Context) { win.SetURL(root) })
	dory.Add("Nodo y dispositivos").OnClick(func(*application.Context) { win.SetURL(nodeURL) })
	dory.AddSeparator()
	dory.Add("Salir").OnClick(func(*application.Context) { app.Quit() })
	app.Menu.Set(menu)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
