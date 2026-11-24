package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/update"

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
	case "status", "start", "stop", "restart", "open", "service", "update", "url", "pair":
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
		// The desktop app starts its core again by itself.
		for range 25 {
			if _, err := health(base); err == nil {
				return true, nil
			}
			time.Sleep(200 * time.Millisecond)
		}
		return true, start(base, cfg.DataDir)
	case "open":
		if err := start(base, cfg.DataDir); err != nil {
			return true, err
		}
		return true, openWindow(base)
	case "pair":
		return true, pair(base, cfg.DataDir, args[1:])
	case "service":
		if len(args) < 2 {
			return true, errors.New("usage: dory service install|uninstall")
		}
		return true, service(args[1])
	case "update":
		// The same update the interface runs, for this node only: the
		// channel is the one the binary was built for.
		u := &update.Updater{DataDir: cfg.DataDir, Active: func() int { return 0 },
			Restart: func() { _ = serviceCtl("restart") }}
		if err := u.Apply(context.Background(), update.Options{}); err != nil {
			return true, err
		}
		fmt.Println("actualizado")
		return true, nil
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
		cmd := cmdexec.Command(self, "serve")
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
		// Another program, such as an older Dory, may own a unit of the same name.
		out, err := cmdexec.Command("systemctl", "--user", "cat", unitName).Output()
		self, _ := os.Executable()
		if err != nil || !strings.Contains(string(out), "ExecStart="+self+" serve") {
			return errors.New("el servicio no está instalado: dory service install")
		}
		return cmdexec.Command("systemctl", "--user", action, unitName).Run()
	case "darwin":
		plist := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "net.dorlab.dory.plist")
		verb := map[string]string{"start": "load", "stop": "unload", "restart": "load"}[action]
		if action == "restart" {
			cmdexec.Command("launchctl", "unload", plist).Run()
		}
		return cmdexec.Command("launchctl", verb, "-w", plist).Run()
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
			cmdexec.Command("systemctl", "--user", "disable", "--now", unitName).Run()
			os.Remove(unitPath)
			os.Remove(desktop)
			return cmdexec.Command("systemctl", "--user", "daemon-reload").Run()
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
			if out, err := cmdexec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput(); err != nil {
				return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), out)
			}
		}
		fmt.Println("servicio instalado y en marcha: arranca con tu sesión")
		return nil
	case "darwin":
		plist := filepath.Join(home, "Library", "LaunchAgents", "net.dorlab.dory.plist")
		if action == "uninstall" {
			cmdexec.Command("launchctl", "unload", plist).Run()
			return os.Remove(plist)
		}
		os.MkdirAll(filepath.Dir(plist), 0o755)
		if err = os.WriteFile(plist, []byte(fmt.Sprintf(launchAgent, self, os.Getenv("PATH"))), 0o644); err != nil {
			return err
		}
		return cmdexec.Command("launchctl", "load", "-w", plist).Run()
	case "windows":
		// A Run entry of the user starts the core at sign-in, without a
		// console window.
		key := `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
		if action == "uninstall" {
			return cmdexec.Command("reg", "delete", key, "/v", "Dory", "/f").Run()
		}
		value := fmt.Sprintf(`conhost.exe --headless "%s" serve`, self)
		if out, err := cmdexec.Command("reg", "add", key, "/v", "Dory", "/t", "REG_SZ", "/d", value, "/f").CombinedOutput(); err != nil {
			return fmt.Errorf("reg add: %s", out)
		}
		fmt.Println("servicio instalado: el núcleo arranca con tu sesión")
		return nil
	}
	return errors.New("este sistema no tiene servicio de usuario: ejecuta `dory serve`")
}

// desktopApp returns the command that opens the installed desktop app of
// this system, or nil when there is none: the app bundles this binary in
// resources/bin, or the system installer put it in its usual place.
func desktopApp() *exec.Cmd {
	home, _ := os.UserHomeDir()
	var paths []string
	if self, err := os.Executable(); err == nil {
		if resources := filepath.Dir(filepath.Dir(self)); filepath.Base(resources) == "resources" || filepath.Base(resources) == "Resources" {
			root := filepath.Dir(resources)
			paths = append(paths, filepath.Join(root, "Dory.exe"), filepath.Join(root, "dory-desktop"), filepath.Join(filepath.Dir(root), "MacOS", "Dory"))
		}
	}
	switch runtime.GOOS {
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			local = filepath.Join(home, "AppData", "Local")
		}
		paths = append(paths, filepath.Join(local, "Programs", "Dory", "Dory.exe"), filepath.Join(os.Getenv("ProgramFiles"), "Dory", "Dory.exe"))
	case "darwin":
		for _, app := range []string{"/Applications/Dory.app", filepath.Join(home, "Applications", "Dory.app")} {
			if _, err := os.Stat(app); err == nil {
				return exec.Command("open", "-a", app)
			}
		}
	default:
		if path, err := exec.LookPath("dory-desktop"); err == nil {
			paths = append(paths, path)
		}
		paths = append(paths, "/opt/Dory/dory-desktop")
		appimages, _ := filepath.Glob(filepath.Join(home, ".local", "bin", "Dory*.AppImage"))
		paths = append(paths, appimages...)
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return exec.Command(path)
		}
	}
	return nil
}

// openWindow shows the interface in the desktop app, in an app window of a
// Chromium browser, or in the default browser. The desktop app comes first
// because browsers add their own interface, such as the link status bubble.
func openWindow(base string) error {
	if app := desktopApp(); app != nil {
		detach(app)
		return app.Start()
	}
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

// pair shows a pairing code for another node to use, or, given the address
// of another node and its code, pairs with it. It needs no interface.
func pair(base, dataDir string, args []string) error {
	if len(args) != 0 && len(args) != 2 {
		return errors.New("usage: dory pair            muestra un código para emparejar este nodo\n       dory pair DIRECCIÓN CÓDIGO   empareja con otro nodo")
	}
	h, err := health(base)
	if err != nil {
		return errors.New("el núcleo no está en marcha: ejecuta `dory start`")
	}
	// The running core may use another data folder than this shell's config.
	if d, ok := h["data_dir"].(string); ok && d != "" {
		dataDir = d
	}
	token, err := os.ReadFile(filepath.Join(dataDir, "control.token"))
	if err != nil {
		return errors.New("no encuentro el token de control: reinicia el núcleo con `dory restart`")
	}
	path, body := "/api/mesh/pair/code", "{}"
	if len(args) == 2 {
		b, _ := json.Marshal(map[string]string{"address": args[0], "code": args[1]})
		path, body = "/api/mesh/pair", string(b)
	}
	req, _ := http.NewRequest("POST", base+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		if e, ok := out["error"].(map[string]any); ok {
			if p, ok := e["params"].(map[string]any); ok && p["detail"] != nil {
				return fmt.Errorf("no se pudo emparejar: %v", p["detail"])
			}
			return fmt.Errorf("no se pudo emparejar: %v", e["message"])
		}
		return fmt.Errorf("no se pudo emparejar: %s", resp.Status)
	}
	if len(args) == 0 {
		fmt.Printf("Código: %v (caduca %v)\nHuella de este nodo: %v\nPuerto de la malla: %v\n\nEn el otro nodo: Entornos → Emparejar, o `dory pair <dirección de este nodo> %v`\n",
			out["code"], out["expires_at"], out["fingerprint"], out["port"], out["code"])
		return nil
	}
	fmt.Printf("Emparejado con %v. Huella: %v. Comprueba que coincide con la del otro nodo.\n", out["name"], out["fingerprint"])
	return nil
}

// restartSelf runs the new binary after an update. Under the service the core
// exits with an error and systemd or launchd start it again; otherwise a
// detached `dory restart` stops this process and starts the new one.
func restartSelf() {
	if serviceCtl("status") == nil || runtime.GOOS == "darwin" {
		os.Exit(3)
	}
	self, err := os.Executable()
	if err != nil {
		return
	}
	cmd := cmdexec.Command(self, "restart")
	detach(cmd)
	_ = cmd.Start()
}
