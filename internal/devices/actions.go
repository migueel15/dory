package devices

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Action is a typed adb change: the interface and the agents never run free commands.
type Action struct {
	Name    string  `json:"action"`
	Value   string  `json:"value,omitempty"`
	Package string  `json:"package,omitempty"`
	Perm    string  `json:"permission,omitempty"`
	Lat     float64 `json:"lat,omitempty"`
	Lon     float64 `json:"lon,omitempty"`
}

var (
	ident     = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)
	resumedRe = regexp.MustCompile(`ActivityRecord\{[0-9a-f]+ u\d+ ([A-Za-z0-9_.]+)/([A-Za-z0-9_.$]+)`)
)

// ParseForeground finds the resumed activity in `dumpsys activity activities`.
func ParseForeground(dump string) (pkg, activity string) {
	for _, l := range strings.Split(dump, "\n") {
		if strings.Contains(l, "ResumedActivity") {
			if m := resumedRe.FindStringSubmatch(l); m != nil {
				return m[1], m[2]
			}
		}
	}
	return "", ""
}

func onOff(v string) (bool, error) {
	switch v {
	case "on", "true", "1":
		return true, nil
	case "off", "false", "0":
		return false, nil
	}
	return false, errors.New("el valor debe ser on u off")
}

// State reads the settings the tools drawer shows. Each key is read from the device.
func (m *Manager) State(ctx context.Context, serial string) map[string]string {
	sh := func(args ...string) string {
		v, _ := m.ADB.Shell(ctx, serial, 6*time.Second, args...)
		return v
	}
	st := map[string]string{}
	pkg, act := ParseForeground(sh("dumpsys", "activity", "activities"))
	if pkg != "" {
		st["foreground"] = pkg + "/" + act
	}
	st["dark_mode"] = strings.TrimSpace(strings.TrimPrefix(sh("cmd", "uimode", "night"), "Night mode:"))
	st["font_scale"] = sh("settings", "get", "system", "font_scale")
	auto, rot := sh("settings", "get", "system", "accelerometer_rotation"), sh("settings", "get", "system", "user_rotation")
	st["orientation"] = orientationName(auto, rot)
	st["wifi"] = flag(sh("settings", "get", "global", "wifi_on"))
	st["data"] = flag(sh("settings", "get", "global", "mobile_data"))
	if strings.HasPrefix(serial, "emulator-") {
		st["location"] = "available"
	}
	return st
}

func flag(v string) string {
	if v == "1" {
		return "on"
	}
	if v == "0" {
		return "off"
	}
	return v
}

func orientationName(auto, rot string) string {
	if auto == "1" {
		return "auto"
	}
	return map[string]string{"0": "portrait", "1": "landscape", "2": "reverse_portrait", "3": "reverse_landscape"}[rot]
}

// Apply runs one typed action and returns the value read back from the device.
func (m *Manager) Apply(ctx context.Context, serial string, a Action) (map[string]string, error) {
	sh := func(args ...string) (string, error) { return m.ADB.Shell(ctx, serial, 8*time.Second, args...) }
	var key string
	switch a.Name {
	case "state":
		return m.State(ctx, serial), nil
	case "dark_mode":
		if a.Value != "yes" && a.Value != "no" && a.Value != "auto" {
			return nil, errors.New("el modo oscuro admite yes, no o auto")
		}
		key = "dark_mode"
		if _, err := sh("cmd", "uimode", "night", a.Value); err != nil {
			return nil, err
		}
	case "font_scale":
		f, err := strconv.ParseFloat(a.Value, 64)
		if err != nil || f < 0.5 || f > 2.0 {
			return nil, errors.New("el tamaño de texto va de 0.5 a 2.0")
		}
		key = "font_scale"
		if _, err := sh("settings", "put", "system", "font_scale", strconv.FormatFloat(f, 'f', 2, 64)); err != nil {
			return nil, err
		}
	case "orientation":
		codes := map[string]string{"portrait": "0", "landscape": "1", "reverse_portrait": "2", "reverse_landscape": "3"}
		key = "orientation"
		if a.Value == "auto" {
			if _, err := sh("settings", "put", "system", "accelerometer_rotation", "1"); err != nil {
				return nil, err
			}
		} else if c, ok := codes[a.Value]; ok {
			if _, err := sh("settings", "put", "system", "accelerometer_rotation", "0"); err != nil {
				return nil, err
			}
			if _, err := sh("settings", "put", "system", "user_rotation", c); err != nil {
				return nil, err
			}
		} else {
			return nil, errors.New("orientación no válida")
		}
	case "wifi", "data":
		on, err := onOff(a.Value)
		if err != nil {
			return nil, err
		}
		// Turning Wi-Fi off would drop the adb connection this very device uses.
		if a.Name == "wifi" && !on && !strings.HasPrefix(serial, "emulator-") && (strings.Contains(serial, "._adb-tls-") || strings.Contains(serial, ":")) {
			return nil, errors.New("no se puede apagar el Wi-Fi de un dispositivo conectado por Wi-Fi: se perdería la conexión")
		}
		verb := map[bool]string{true: "enable", false: "disable"}[on]
		key = a.Name
		if _, err := sh("svc", a.Name, verb); err != nil {
			return nil, err
		}
		time.Sleep(700 * time.Millisecond) // the setting flips a moment after the service call
	case "permission":
		if !ident.MatchString(a.Package) || !ident.MatchString(a.Perm) {
			return nil, errors.New("paquete o permiso no válidos")
		}
		verb := "revoke"
		if a.Value == "grant" {
			verb = "grant"
		} else if a.Value != "revoke" {
			return nil, errors.New("el valor debe ser grant o revoke")
		}
		if _, err := sh("pm", verb, a.Package, a.Perm); err != nil {
			return nil, err
		}
		dump, _ := sh("dumpsys", "package", a.Package)
		state := "revoked"
		if strings.Contains(dump, a.Perm+": granted=true") {
			state = "granted"
		}
		return map[string]string{"permission": a.Perm, "state": state}, nil
	case "location":
		if !strings.HasPrefix(serial, "emulator-") {
			return nil, errors.New("la ubicación simulada solo funciona en emuladores")
		}
		if a.Lat < -90 || a.Lat > 90 || a.Lon < -180 || a.Lon > 180 {
			return nil, errors.New("coordenadas fuera de rango")
		}
		if _, err := m.ADB.Run(ctx, 6*time.Second, "-s", serial, "emu", "geo", "fix", fmt.Sprint(a.Lon), fmt.Sprint(a.Lat)); err != nil {
			return nil, err
		}
		return map[string]string{"location": fmt.Sprintf("%.5f,%.5f", a.Lat, a.Lon)}, nil
	default:
		return nil, errors.New("acción no admitida")
	}
	return map[string]string{key: m.State(ctx, serial)[key]}, nil
}
