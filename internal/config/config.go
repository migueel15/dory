// Package config loads defaults, TOML and then DORY_* environment overrides.
package config

import (
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Listen  string `toml:"listen"`
	DataDir string `toml:"data_dir"`
	// Name shown for this node in the mesh; defaults to the host name.
	NodeName string `toml:"node_name"`
	// MeshPort is where other nodes reach this one; 0 turns the mesh off.
	MeshPort       int           `toml:"mesh_port"`
	CookieSecure   bool          `toml:"cookie_secure"`
	PublicOrigin   string        `toml:"public_origin"`
	SessionTTL     time.Duration `toml:"-"`
	SessionTTLText string        `toml:"session_ttl"`
	// Tasks and Threads hold the defaults of the settings the interface can
	// change on a node.
	Tasks   TasksConfig   `toml:"tasks"`
	Threads ThreadsConfig `toml:"threads"`
}

type TasksConfig struct {
	// ArchiveThreadsOnFinish archives the threads of a task that becomes done or cancelled.
	ArchiveThreadsOnFinish bool `toml:"archive_threads_on_finish"`
}

type ThreadsConfig struct {
	// RemoveWorktreeOnArchive deletes the worktree of an archived thread.
	RemoveWorktreeOnArchive bool `toml:"remove_worktree_on_archive"`
}

// Load uses DORY_CONFIG, or ~/.config/dory/config.toml. An explicitly selected
// missing file is an error; the implicit default file is optional.
func Load() (Config, error) {
	// A desktop node serves plain HTTP on loopback; a server behind TLS sets
	// cookie_secure.
	c := Config{Listen: "127.0.0.1:7700", SessionTTLText: "168h", MeshPort: 7710,
		Threads: ThreadsConfig{RemoveWorktreeOnArchive: true}}
	path := os.Getenv("DORY_CONFIG")
	explicit := path != ""
	if !explicit {
		dir, err := os.UserConfigDir()
		if err != nil {
			return c, err
		}
		path = filepath.Join(dir, "dory", "config.toml")
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if err = toml.Unmarshal(data, &c); err != nil {
			return c, err
		}
	} else if explicit || !os.IsNotExist(err) {
		return c, err
	}
	for key, target := range map[string]*string{"DORY_LISTEN": &c.Listen, "DORY_DATA_DIR": &c.DataDir, "DORY_NODE_NAME": &c.NodeName, "DORY_SESSION_TTL": &c.SessionTTLText, "DORY_PUBLIC_ORIGIN": &c.PublicOrigin} {
		if v, ok := os.LookupEnv(key); ok {
			*target = v
		}
	}
	if v, ok := os.LookupEnv("DORY_MESH_PORT"); ok {
		if _, err := fmt.Sscan(v, &c.MeshPort); err != nil {
			return c, fmt.Errorf("DORY_MESH_PORT must be a number")
		}
	}
	if v, ok := os.LookupEnv("DORY_COOKIE_SECURE"); ok {
		switch v {
		case "true":
			c.CookieSecure = true
		case "false":
			c.CookieSecure = false
		default:
			return c, fmt.Errorf("DORY_COOKIE_SECURE must be true or false")
		}
	}
	if c.DataDir == "" {
		c.DataDir, err = defaultDataDir()
		if err != nil {
			return c, err
		}
	}
	if c.NodeName == "" {
		c.NodeName, _ = os.Hostname()
	}
	if _, _, err = net.SplitHostPort(c.Listen); err != nil {
		return c, fmt.Errorf("listen: %w", err)
	}
	c.SessionTTL, err = time.ParseDuration(c.SessionTTLText)
	if err != nil || c.SessionTTL <= 0 {
		return c, fmt.Errorf("session_ttl must be a positive duration")
	}
	if c.PublicOrigin != "" {
		if err = validateOrigin(c.PublicOrigin); err != nil {
			return c, fmt.Errorf("public_origin: %w", err)
		}
	}
	return c, nil
}

func defaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".dory", "data")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		return dir, nil
	}
	legacy := os.Getenv("XDG_DATA_HOME")
	if legacy == "" {
		legacy = filepath.Join(home, ".local", "share")
	}
	legacy = filepath.Join(legacy, "dory")
	if _, err := os.Stat(filepath.Join(legacy, "dory.db")); !os.IsNotExist(err) {
		return legacy, nil
	}
	return dir, nil
}

// validateOrigin accepts scheme://host[:port] only, the form browsers send in Origin.
func validateOrigin(origin string) error {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("must be an http or https URL")
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("must contain only scheme and host, without path, query or credentials")
	}
	return nil
}
