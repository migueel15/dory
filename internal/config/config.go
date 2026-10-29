// Package config loads defaults, TOML and then DORY_* environment overrides.
package config

import (
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"net"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Listen         string        `toml:"listen"`
	DatabaseURL    string        `toml:"database_url"`
	CookieSecure   bool          `toml:"cookie_secure"`
	SessionTTL     time.Duration `toml:"-"`
	SessionTTLText string        `toml:"session_ttl"`
}

// Load uses DORY_CONFIG, or ~/.config/dory/config.toml. An explicitly selected
// missing file is an error; the implicit default file is optional.
func Load() (Config, error) {
	c := Config{Listen: "127.0.0.1:8080", CookieSecure: true, SessionTTLText: "168h"}
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
	for key, target := range map[string]*string{"DORY_LISTEN": &c.Listen, "DORY_DATABASE_URL": &c.DatabaseURL, "DORY_SESSION_TTL": &c.SessionTTLText} {
		if v, ok := os.LookupEnv(key); ok {
			*target = v
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
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("database_url or DORY_DATABASE_URL is required")
	}
	if _, _, err = net.SplitHostPort(c.Listen); err != nil {
		return c, fmt.Errorf("listen: %w", err)
	}
	c.SessionTTL, err = time.ParseDuration(c.SessionTTLText)
	if err != nil || c.SessionTTL <= 0 {
		return c, fmt.Errorf("session_ttl must be a positive duration")
	}
	return c, nil
}
