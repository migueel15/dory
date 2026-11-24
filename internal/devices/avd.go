package devices

import (
	"bytes"
	"context"
	"errors"
	"github.com/migueel15/dory/internal/i18n"
	"os/exec"
	"regexp"
	"strings"
	"sync"
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

var (
	avdName  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]*$`)
	hostPort = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-]*:[0-9]{1,5}$`)
	pairCode = regexp.MustCompile(`^[0-9]{4,10}$`)
)

// AVDState is the progress of an emulator this node started.
type AVDState struct {
	Status string `json:"status"` // starting, booting, ready, failed
	// Code names a known failure the web explains (avd.exited, avd.timeout);
	// Error is the technical detail.
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

type avdProc struct {
	mu    sync.Mutex
	state AVDState
	cmd   *exec.Cmd
}

func (p *avdProc) set(s AVDState) { p.mu.Lock(); p.state = s; p.mu.Unlock() }

// AVDStates returns the state of the emulators this node started.
func (m *Manager) AVDStates() map[string]AVDState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]AVDState{}
	for name, p := range m.avds {
		p.mu.Lock()
		out[name] = p.state
		p.mu.Unlock()
	}
	return out
}

// StartAVD launches an emulator without a window and watches it until Android booted.
func (m *Manager) StartAVD(name string) error {
	if !avdName.MatchString(name) {
		return errors.New(i18n.Text("devices.nombre_emulador_no_valido"))
	}
	if m.Tools.Emulator == "" {
		return errors.New(i18n.Text("devices.emulador_android_no_esta"))
	}
	if !m.Tools.KVM {
		return errors.New(i18n.Text("devices.emulador_necesita_kvm_dev"))
	}
	m.mu.Lock()
	if p := m.avds[name]; p != nil && (p.state.Status == "starting" || p.state.Status == "booting") {
		m.mu.Unlock()
		return nil
	}
	p := &avdProc{state: AVDState{Status: "starting"}}
	m.avds[name] = p
	m.mu.Unlock()
	cmd := exec.Command(m.Tools.Emulator, "-avd", name, "-no-window", "-no-audio", "-no-boot-anim", "-no-snapshot-save")
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &lockedWriter{b: &log}, &lockedWriter{b: &log}
	if err := cmd.Start(); err != nil {
		p.set(AVDState{Status: "failed", Error: err.Error()})
		return err
	}
	p.cmd = cmd
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	go func() {
		p.set(AVDState{Status: "booting"})
		deadline := time.After(4 * time.Minute)
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case err := <-exited:
				p.set(AVDState{Status: "failed", Code: "avd.exited", Error: tail(log.String(), "the emulator exited")})
				_ = err
				return
			case <-deadline:
				_ = cmd.Process.Kill()
				p.set(AVDState{Status: "failed", Code: "avd.timeout", Error: "the emulator did not boot in 4 minutes"})
				return
			case <-tick.C:
				if m.avdBooted(name) {
					p.set(AVDState{Status: "ready"})
					<-exited
					// Stopped on purpose or by the user: it is just off again.
					m.mu.Lock()
					if m.avds[name] == p {
						delete(m.avds, name)
					}
					m.mu.Unlock()
					return
				}
			}
		}
	}()
	return nil
}

func (m *Manager) avdBooted(name string) bool {
	ctx := context.Background()
	list, err := m.ADB.Devices(ctx)
	if err != nil {
		return false
	}
	for _, d := range list {
		if d.Transport == "emulator" && d.State == "device" && m.ADB.AVDName(ctx, d.Serial) == name {
			v, _ := m.ADB.Shell(ctx, d.Serial, 4*time.Second, "getprop", "sys.boot_completed")
			return v == "1"
		}
	}
	return false
}

func tail(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	lines := strings.Split(s, "\n")
	return strings.Join(lines[max(0, len(lines)-3):], "\n")
}

type lockedWriter struct {
	mu sync.Mutex
	b  *bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.b.Len() > 64<<10 {
		w.b.Reset()
	}
	return w.b.Write(p)
}

// Stop turns a device off: emulators are killed, physical devices powered off (the caller confirms).
func (m *Manager) Stop(ctx context.Context, serial string) error {
	m.Close(serial)
	if strings.HasPrefix(serial, "emulator-") {
		_, err := m.ADB.Run(ctx, 10*time.Second, "-s", serial, "emu", "kill")
		return err
	}
	_, err := m.ADB.Shell(ctx, serial, 10*time.Second, "reboot", "-p")
	return err
}

// Pair pairs with a device that shows a wireless debugging pairing code.
func (m *Manager) Pair(ctx context.Context, hostPortStr, code string) (string, error) {
	if !hostPort.MatchString(hostPortStr) || !pairCode.MatchString(code) {
		return "", errors.New(i18n.Text("devices.direccion_codigo_emparejamiento_no"))
	}
	out, err := m.ADB.Run(ctx, 20*time.Second, "pair", hostPortStr, code)
	res := strings.TrimSpace(string(out))
	if err == nil && !strings.Contains(strings.ToLower(res), "successfully") {
		err = errors.New(res)
	}
	return res, err
}

// Connect connects to a device over Wi-Fi.
func (m *Manager) Connect(ctx context.Context, hostPortStr string) (string, error) {
	if !hostPort.MatchString(hostPortStr) {
		return "", errors.New(i18n.Text("devices.direccion_no_valida"))
	}
	out, err := m.ADB.Run(ctx, 15*time.Second, "connect", hostPortStr)
	res := strings.TrimSpace(string(out))
	if err == nil && !strings.Contains(res, "connected") {
		err = errors.New(res)
	}
	return res, err
}

// Disconnect drops a Wi-Fi device.
func (m *Manager) Disconnect(ctx context.Context, serial string) error {
	m.Close(serial)
	_, err := m.ADB.Run(ctx, 10*time.Second, "disconnect", serial)
	return err
}
