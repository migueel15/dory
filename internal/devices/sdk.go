// Package devices bridges Android devices and emulators to Dory: it lists
// them with adb, streams their screen with scrcpy-server and injects input.
package devices

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Tools tells where the Android tools are and what this node lacks.
type Tools struct {
	ADB      string `json:"adb"`
	Emulator string `json:"emulator"`
	KVM      bool   `json:"kvm"`
	SDK      string `json:"sdk"`
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// FindTools looks for adb and the emulator in ANDROID_HOME, ANDROID_SDK_ROOT,
// the default SDK folder and PATH.
func FindTools() Tools {
	var roots []string
	for _, k := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(k); v != "" {
			roots = append(roots, v)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "Android", "Sdk"), filepath.Join(home, "Library", "Android", "sdk"), filepath.Join(home, "AppData", "Local", "Android", "Sdk"))
	}
	t := Tools{}
	find := func(rel ...string) string {
		for _, r := range roots {
			p := filepath.Join(append([]string{r}, rel...)...)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				t.SDK = r
				return p
			}
		}
		return ""
	}
	t.ADB = find("platform-tools", exe("adb"))
	if t.ADB == "" {
		t.ADB, _ = exec.LookPath("adb")
	}
	t.Emulator = find("emulator", exe("emulator"))
	if t.Emulator == "" {
		t.Emulator, _ = exec.LookPath("emulator")
	}
	if _, err := os.Stat("/dev/kvm"); err == nil {
		t.KVM = true
	}
	return t
}
