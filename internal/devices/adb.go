package devices

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrNoADB means adb is not installed on this node.
var ErrNoADB = errors.New("adb no está instalado en este nodo")

// Device is one entry of `adb devices -l`.
type Device struct {
	Serial    string `json:"serial"`
	State     string `json:"state"` // device, offline, unauthorized...
	Model     string `json:"model"`
	Product   string `json:"product"`
	Transport string `json:"transport"` // usb, wifi or emulator
	AVD       string `json:"avd,omitempty"`
}

// ParseDevices reads the output of `adb devices -l`.
func ParseDevices(out string) []Device {
	var list []Device
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") || strings.HasPrefix(line, "* daemon") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		d := Device{Serial: f[0], State: f[1]}
		for _, kv := range f[2:] {
			k, v, ok := strings.Cut(kv, ":")
			if !ok {
				continue
			}
			switch k {
			case "model":
				d.Model = strings.ReplaceAll(v, "_", " ")
			case "product":
				d.Product = v
			}
		}
		switch {
		case strings.HasPrefix(d.Serial, "emulator-"):
			d.Transport = "emulator"
		case strings.Contains(d.Serial, "._adb-tls-connect.") || strings.Contains(d.Serial, ":"):
			d.Transport = "wifi"
		default:
			d.Transport = "usb"
		}
		list = append(list, d)
	}
	return list
}

// ADB runs adb commands with a timeout.
type ADB struct{ Path string }

// Run runs adb with args and returns stdout; stderr goes in the error.
func (a ADB) Run(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	if a.Path == "" {
		return nil, ErrNoADB
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.Path, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		if ctx.Err() != nil {
			msg = "tiempo agotado"
		}
		return out.Bytes(), fmt.Errorf("adb %s: %s", strings.Join(args, " "), msg)
	}
	return out.Bytes(), nil
}

// Shell runs a command on the device.
func (a ADB) Shell(ctx context.Context, serial string, timeout time.Duration, args ...string) (string, error) {
	out, err := a.Run(ctx, timeout, append([]string{"-s", serial, "shell"}, args...)...)
	return strings.TrimSpace(string(out)), err
}

// Devices lists the devices adb sees.
func (a ADB) Devices(ctx context.Context) ([]Device, error) {
	out, err := a.Run(ctx, 8*time.Second, "devices", "-l")
	if err != nil {
		return nil, err
	}
	return ParseDevices(string(out)), nil
}
