package skills

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

const linkPrefix = "link:"

func antigravitySkillDir(home string) string {
	return filepath.Join(home, ".gemini", "config", "skills")
}

// antigravityTarget links a skill to Dory's managed copy. Windows can fall
// back to the normal file reconciliation when symlinks are unavailable.
func (r *Reconciler) antigravityTarget(dir, name string, m manifest, want map[string]bool) (string, error) {
	target, err := filepath.Abs(filepath.Join(filepath.Dir(r.Manifest), "antigravity-skills", name))
	if err != nil {
		return "", err
	}
	if actual, err := os.Readlink(dir); err == nil {
		if actual != target || m[dir] != linkPrefix+target {
			return "", errors.New("conflicto: " + dir + " es un enlace ajeno")
		}
		want[dir] = true
		return target, nil
	}
	if _, err := os.Lstat(dir); err == nil {
		return dir, nil // Existing skill or a previously installed Windows copy.
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}
	if err := os.Symlink(target, dir); err != nil {
		if runtime.GOOS == "windows" {
			_ = os.Remove(target)
			return dir, nil
		}
		return "", err
	}
	m[dir] = linkPrefix + target
	want[dir] = true
	return target, nil
}
