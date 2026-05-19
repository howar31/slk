// Package auth manages slk credentials: profiles, token resolution, OAuth.
package auth

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Profile holds credentials for one Slack workspace.
type Profile struct {
	Workspace    string `toml:"workspace"`
	UserToken    string `toml:"user_token,omitempty"`
	BotToken     string `toml:"bot_token,omitempty"`
	ClientID     string `toml:"client_id,omitempty"`
	ClientSecret string `toml:"client_secret,omitempty"`
}

// Config is the on-disk slk configuration.
type Config struct {
	Active   string             `toml:"active"`
	Profiles map[string]Profile `toml:"profiles"`
}

// defaultPath returns ~/.config/slk/config.toml.
func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "slk", "config.toml"), nil
}

// ConfigPath returns the config path, honoring the SLK_CONFIG env override.
func ConfigPath() (string, error) {
	if p := os.Getenv("SLK_CONFIG"); p != "" {
		return p, nil
	}
	return defaultPath()
}

// Load reads the config; a missing file yields an empty Config and no error.
func Load(path string) (*Config, error) {
	cfg := &Config{Profiles: map[string]Profile{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return cfg, nil
}

// Save writes the config atomically with 0600 permissions.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
