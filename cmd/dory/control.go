package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/config"
)

// The core runs as a background service of the user: closing the interface
// does not stop it, and there is one per machine (requirements 4.4 and 4.5).
const unitName = "dory.service"

const unit = `[Unit]
Description=Dory, nodo de agentes
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s serve
Restart=on-failure
RestartSec=3
# Agents run with the user's environment: keep PATH so harnesses are found.
Environment=PATH=%s

[Install]
WantedBy=default.target
`

const launchAgent = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>net.dorlab.dory</string>
  <key>ProgramArguments</key><array><string>%s</string><string>serve</string></array>
  <key>EnvironmentVariables</key><dict><key>PATH</key><string>%s</string></dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
</dict></plist>
`

const desktopEntry = `[Desktop Entry]
Type=Application
Name=Dory
Comment=Tu entorno de trabajo con agentes
Exec=%s open
Icon=dory
Terminal=false
Categories=Development;
`

// control handles the commands that manage the core of this machine. It
// returns false for the other commands.
func control(args []string) (bool, error) {
	cmd := args[0]
	switch cmd {
	case "status", "start", "stop", "restart", "open", "service", "update", "url":
	default:
		return false, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return true, err
	}
	base := "http://" + loopbackAddr(cfg.Listen)
	switch cmd {
	case "url":
		fmt.Println(base)
		return true, nil
	case "status":
		h, err := health(base)
		if err != nil {
			fmt.Println("parado")
			return true, nil
		}
		fmt.Printf("en marcha: %s (%s), versión %s, en %s\n", h["name"], h["node_id"], h["version"], base)
		return true, nil
	case "start":
		return true, start(base, cfg.DataDir)
	case "stop":
		return true, stop(base)
	case "restart":
		if err := stop(base); err != nil {
			return true, err
		}
		return true, start(base, cfg.DataDir)
	case "open":
		if err := start(base, cfg.DataDir); err != nil {
			return true, err
		}
		return true, openWindow(base)
	case "service":
		if len(args) < 2 {
			return true, errors.New("usage: dory service install|uninstall")
		}
		return true, service(args[1])
	case "update":
		fs := flag.NewFlagSet("update", flag.ContinueOnError)
		channel := fs.String("channel", "stable", "stable or nightly")
		if err := fs.Parse(args[1:]); err != nil {
			return true, err
		}
		return true, update(*channel)
	}
	return true, nil
}

func health(base string) (map[string]any, error) {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(base + "/api/health")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var h map[string]any
	if err = json.NewDecoder(resp.Body).Decode(&h); err != nil || h["app"] != "dory" {
		return nil, errors.New("another program answers on the Dory port")
	}
	return h, nil
}

// start runs the core if it is not running yet: through the service when it
// is installed, otherwise as a detached process.
func start(base, dataDir string) error {
	if _, err := health(base); err == nil {
		return nil
	}
	if err := serviceCtl("start"); err != nil {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		if err = os.MkdirAll(dataDir, 0o700); err != nil {
			return err
		}
		logFile, err := os.OpenFile(filepath.Join(dataDir, "dory.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		defer logFile.Close()
		cmd := exec.Command(self, "serve")
		cmd.Stdout, cmd.Stderr = logFile, logFile
		detach(cmd)
		if err = cmd.Start(); err != nil {
			return err
		}
		cmd.Process.Release()
	}
	for range 50 {
		if _, err := health(base); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("el núcleo no arrancó; mira %s o `journalctl --user -u dory`", filepath.Join(dataDir, "dory.log"))
}

// stop ends the core: through the service when it is installed, otherwise
// by signalling the process that answers on the Dory port.
func stop(base string) error {
	serviceCtl("stop")
	// A core started outside the service may still hold the port.
	h, err := health(base)
	if err != nil {
		return nil // already stopped
	}
	pid, ok := h["pid"].(float64)
	if !ok {
		return errors.New("el núcleo en marcha es de una versión anterior: páralo a mano")
	}
	p, err := os.FindProcess(int(pid))
	if err != nil {
		return err
	}
	if err = terminate(p); err != nil {
		return err
	}
	for range 50 {
		if _, err := health(base); err != nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("el núcleo no se detuvo")
}

func serviceCtl(action string) error {
	switch runtime.GOOS {
	case "linux":
		if exec.Command("systemctl", "--user", "cat", unitName).Run() != nil {
			return errors.New("el servicio no está instalado: dory service install")
		}
		return exec.Command("systemctl", "--user", action, unitName).Run()
	case "darwin":
		plist := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "net.dorlab.dory.plist")
		verb := map[string]string{"start": "load", "stop": "unload", "restart": "load"}[action]
		if action == "restart" {
			exec.Command("launchctl", "unload", plist).Run()
		}
		return exec.Command("launchctl", verb, "-w", plist).Run()
	}
	return errors.New("en este sistema el núcleo se arranca con `dory serve`")
}

// service installs or removes the user service and the desktop launcher.
func service(action string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, _ = filepath.EvalSymlinks(self)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "linux":
		unitPath := filepath.Join(home, ".config", "systemd", "user", unitName)
		desktop := filepath.Join(home, ".local", "share", "applications", "dory.desktop")
		if action == "uninstall" {
			exec.Command("systemctl", "--user", "disable", "--now", unitName).Run()
			os.Remove(unitPath)
			os.Remove(desktop)
			return exec.Command("systemctl", "--user", "daemon-reload").Run()
		}
		for _, p := range []string{unitPath, desktop} {
			if err = os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
		}
		if err = os.WriteFile(unitPath, []byte(fmt.Sprintf(unit, self, os.Getenv("PATH"))), 0o644); err != nil {
			return err
		}
		// The Electron app has its own launcher; this one opens the interface in a browser app window.
		if _, err := os.Stat(desktop); errors.Is(err, os.ErrNotExist) {
			os.WriteFile(desktop, []byte(fmt.Sprintf(desktopEntry, self)), 0o644)
		}
		for _, args := range [][]string{{"daemon-reload"}, {"enable", "--now", unitName}} {
			if out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput(); err != nil {
				return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), out)
			}
		}
		fmt.Println("servicio instalado y en marcha: arranca con tu sesión")
		return nil
	case "darwin":
		plist := filepath.Join(home, "Library", "LaunchAgents", "net.dorlab.dory.plist")
		if action == "uninstall" {
			exec.Command("launchctl", "unload", plist).Run()
			return os.Remove(plist)
		}
		os.MkdirAll(filepath.Dir(plist), 0o755)
		if err = os.WriteFile(plist, []byte(fmt.Sprintf(launchAgent, self, os.Getenv("PATH"))), 0o644); err != nil {
			return err
		}
		return exec.Command("launchctl", "load", "-w", plist).Run()
	}
	return errors.New("en Windows, crea una tarea programada al iniciar sesión que ejecute `dory serve`")
}

// openWindow shows the interface in an app window of a Chromium browser, or
// in the default browser.
func openWindow(base string) error {
	for _, b := range []string{"chromium", "google-chrome-stable", "google-chrome", "brave", "microsoft-edge"} {
		if path, err := exec.LookPath(b); err == nil {
			cmd := exec.Command(path, "--app="+base)
			detach(cmd)
			return cmd.Start()
		}
	}
	opener := map[string][]string{"linux": {"xdg-open"}, "darwin": {"open"}, "windows": {"cmd", "/c", "start"}}[runtime.GOOS]
	return exec.Command(opener[0], append(opener[1:], base)...).Start()
}

// update replaces this binary with the latest release of a channel, using
// the GitHub CLI so private repositories work too.
func update(channel string) error {
	tag := map[string]string{"stable": "", "nightly": "nightly"}[channel]
	if channel != "stable" && channel != "nightly" {
		return errors.New("--channel must be stable or nightly")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "dory-update")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	asset := fmt.Sprintf("dory-%s-%s", runtime.GOOS, runtime.GOARCH)
	args := []string{"release", "download", "--repo", "migueel15/dory", "--pattern", asset, "--dir", dir}
	if tag != "" {
		args = append(args[:2], append([]string{tag}, args[2:]...)...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "gh", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("gh release download: %s", strings.TrimSpace(string(out)))
	}
	next := filepath.Join(dir, asset)
	if err = os.Chmod(next, 0o755); err != nil {
		return err
	}
	// Replace atomically next to the running binary, then restart the core.
	tmp := self + ".new"
	raw, err := os.ReadFile(next)
	if err != nil {
		return err
	}
	if err = os.WriteFile(tmp, raw, 0o755); err != nil {
		return err
	}
	if err = os.Rename(tmp, self); err != nil {
		return err
	}
	fmt.Printf("actualizado al canal %s\n", channel)
	if serviceCtl("restart") == nil {
		fmt.Println("núcleo reiniciado")
	}
	return nil
}
