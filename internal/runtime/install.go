package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// shellInstall runs the official installer script of a harness.
func shellInstall(unix, windows string) []string {
	if runtime.GOOS == "windows" {
		return []string{"powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "irm " + windows + " | iex"}
	}
	return []string{"sh", "-c", "curl -fsSL " + unix + " | bash"}
}

func (Claude) InstallCommand() []string {
	return shellInstall("https://claude.ai/install.sh", "https://claude.ai/install.ps1")
}

func (Codex) InstallCommand() []string {
	return shellInstall("https://chatgpt.com/codex/install.sh", "https://chatgpt.com/codex/install.ps1")
}

// OpenCode publishes no installer for Windows; npm is its documented route.
func (OpenCode) InstallCommand() []string {
	if runtime.GOOS == "windows" {
		return []string{"npm", "install", "-g", "@opencode/cli@latest"}
	}
	return shellInstall("https://opencode.ai/v2/install", "")
}

// installDirs are where the official installers leave the harnesses. The
// node may have started with a PATH that lacks them, as on Windows, where it
// inherits the environment of whoever launched it.
func installDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(home, ".local", "bin"), filepath.Join(home, ".opencode", "bin")}
	if runtime.GOOS == "windows" {
		dirs = append(dirs,
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "OpenAI", "Codex", "bin"),
			filepath.Join(os.Getenv("APPDATA"), "npm"))
	}
	return dirs
}

// resolveBin returns name when it is on PATH, else its path in a known
// install folder, else name so the caller's error names the harness.
func resolveBin(name string) string {
	if _, err := exec.LookPath(name); err == nil {
		return name
	}
	for _, dir := range installDirs() {
		if path, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return path
		}
	}
	return name
}
