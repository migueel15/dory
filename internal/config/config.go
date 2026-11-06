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
	Listen         string        `toml:"listen"`
	DatabaseURL    string        `toml:"database_url"`
	CookieSecure   bool          `toml:"cookie_secure"`
	PublicOrigin   string        `toml:"public_origin"`
	SessionTTL     time.Duration `toml:"-"`
	SessionTTLText string        `toml:"session_ttl"`
	// Title generation: an empty TitleModel turns it off.
	TitleProvider    string        `toml:"title_provider"`
	TitleModel       string        `toml:"title_model"`
	TitleEffort      string        `toml:"title_effort"`
	TitleProject     string        `toml:"title_project"`
	TitleTimeout     time.Duration `toml:"-"`
	TitleTimeoutText string        `toml:"title_timeout"`
}

// Load uses DORY_CONFIG, or ~/.config/dory/config.toml. An explicitly selected
// missing file is an error; the implicit default file is optional.
func Load() (Config, error) {
	c := Config{Listen: "127.0.0.1:8080", CookieSecure: true, SessionTTLText: "168h", TitleTimeoutText: "2m"}
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
	for key, target := range map[string]*string{"DORY_LISTEN": &c.Listen, "DORY_DATABASE_URL": &c.DatabaseURL, "DORY_SESSION_TTL": &c.SessionTTLText, "DORY_PUBLIC_ORIGIN": &c.PublicOrigin, "DORY_TITLE_PROVIDER": &c.TitleProvider, "DORY_TITLE_MODEL": &c.TitleModel, "DORY_TITLE_EFFORT": &c.TitleEffort, "DORY_TITLE_PROJECT": &c.TitleProject, "DORY_TITLE_TIMEOUT": &c.TitleTimeoutText} {
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
	c.TitleTimeout, err = time.ParseDuration(c.TitleTimeoutText)
	if err != nil || c.TitleTimeout <= 0 {
		return c, fmt.Errorf("title_timeout must be a positive duration")
	}
	if c.PublicOrigin != "" {
		if err = validateOrigin(c.PublicOrigin); err != nil {
			return c, fmt.Errorf("public_origin: %w", err)
		}
	}
	return c, nil
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
