// Package auth manages slk credentials: profiles, token resolution, OAuth.
package auth

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Profile holds the credential for one Slack identity. The token is stored
// encrypted at rest (see crypto.go); it is plaintext in memory after Load and
// re-encrypted by Save. Scope (user/bot) is derived from the prefix (TokenScope),
// never stored. slk does not persist the OAuth client id/secret.
type Profile struct {
	Token string `toml:"token,omitempty"`
}

// Config is the on-disk slk configuration. KeyBackend records which backend
// holds the encryption key ("file" or "keyring") so reads are deterministic.
type Config struct {
	Active     string             `toml:"active"`
	KeyBackend string             `toml:"key_backend,omitempty"`
	Profiles   map[string]Profile `toml:"profiles"`
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
// Encrypted (enc:v1:) fields are decrypted into memory best-effort: a field that
// cannot be decrypted (key unavailable / bad ciphertext) is left as ciphertext
// and ResolveToken reports it. Plaintext fields are passed through unchanged so
// they still resolve; they are encrypted only when something next calls Save.
// Load never writes to disk.
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

	decryptInPlace(cfg, path)
	return cfg, nil
}

// decryptInPlace decrypts each enc:v1: sensitive field in cfg, best-effort: a
// field whose key is unavailable or whose ciphertext is invalid is left as-is
// (ResolveToken then reports it). Plaintext and empty fields are left untouched.
func decryptInPlace(cfg *Config, path string) {
	var key []byte
	var keyErr error
	keyTried := false
	ensureKey := func() bool {
		if !keyTried {
			key, keyErr = loadKey(resolveBackend(cfg), path, false)
			keyTried = true
		}
		return keyErr == nil
	}

	for name, p := range cfg.Profiles {
		if p.Token == "" || !isEncrypted(p.Token) {
			continue
		}
		if !ensureKey() {
			continue
		}
		if pt, err := decryptValue(key, p.Token); err == nil {
			p.Token = pt
			cfg.Profiles[name] = p
		}
	}
}

// Save encrypts the sensitive fields of every profile and writes the config
// atomically with 0600 permissions. The caller's cfg is not mutated.
func Save(path string, cfg *Config) error {
	backend := resolveBackend(cfg)
	key, err := loadKey(backend, path, true)
	if err != nil {
		return err
	}

	out := *cfg
	out.KeyBackend = backend
	out.Profiles = make(map[string]Profile, len(cfg.Profiles))
	for name, p := range cfg.Profiles {
		ep, err := encryptProfile(key, p)
		if err != nil {
			return err
		}
		out.Profiles[name] = ep
	}
	return writeConfig(path, &out)
}

// encryptProfile returns a copy of p with its token encrypted when it is
// non-empty and not already encrypted.
func encryptProfile(key []byte, p Profile) (Profile, error) {
	if p.Token == "" || isEncrypted(p.Token) {
		return p, nil
	}
	v, err := encryptValue(key, p.Token)
	if err != nil {
		return p, err
	}
	p.Token = v
	return p, nil
}

// writeConfig atomically writes cfg to path (temp file + rename), 0600.
func writeConfig(path string, cfg *Config) error {
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
