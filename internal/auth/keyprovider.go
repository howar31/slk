package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

// Backend identifiers and the env var that selects them.
const (
	backendFile    = "file"
	backendKeyring = "keyring"
	backendAuto    = "auto"

	keyEnvVar = "SLK_KEYRING_BACKEND"

	keyringService = "slk"
	keyringUser    = "encryption-key"
	keyFileName    = ".encryption_key"
)

// Sentinel errors used by the keyring layer (and the test fake).
var (
	errKeyNotFound        = errors.New("encryption key not found")
	errKeyringUnavailable = errors.New("keyring unavailable")
)

// keyringStore abstracts the OS keyring so tests can substitute a fake.
type keyringStore interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

// osKeyring is the production keyring backed by github.com/zalando/go-keyring.
type osKeyring struct{}

func (osKeyring) Get(s, u string) (string, error) { return keyring.Get(s, u) }
func (osKeyring) Set(s, u, p string) error        { return keyring.Set(s, u, p) }
func (osKeyring) Delete(s, u string) error        { return keyring.Delete(s, u) }

// activeKeyring is the keyring in use; overridden in tests.
var activeKeyring keyringStore = osKeyring{}

// keyFilePath returns the encryption-key file located beside the config file.
func keyFilePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), keyFileName)
}

// resolveBackend decides where the key lives. Precedence: a backend already
// recorded in the config (that is where an existing key actually is) wins; then
// the SLK_KEYRING_BACKEND env value; then auto-probe (keyring if usable, else
// file).
func resolveBackend(cfg *Config) string {
	if cfg != nil && (cfg.KeyBackend == backendFile || cfg.KeyBackend == backendKeyring) {
		return cfg.KeyBackend
	}
	switch os.Getenv(keyEnvVar) {
	case backendFile:
		return backendFile
	case backendKeyring:
		return backendKeyring
	}
	if keyringAvailable() {
		return backendKeyring
	}
	return backendFile
}

// keyringAvailable probes the OS keyring with a throwaway item. A headless or
// locked keyring fails the Set and we fall back to the file backend.
func keyringAvailable() bool {
	const probe = "slk-probe"
	if err := activeKeyring.Set(keyringService, probe, "1"); err != nil {
		return false
	}
	_, getErr := activeKeyring.Get(keyringService, probe)
	_ = activeKeyring.Delete(keyringService, probe)
	return getErr == nil
}

// loadKey returns the 32-byte AES key for backend, creating it if create is
// true and none exists yet. configPath locates the file-backend key.
func loadKey(backend, configPath string, create bool) ([]byte, error) {
	if backend == backendKeyring {
		return keyFromKeyring(create)
	}
	return keyFromFile(keyFilePath(configPath), create)
}

func keyFromFile(path string, create bool) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if !create {
			return nil, &AuthError{Reason: "encryption key not found"}
		}
		key, err := newKey()
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		enc := base64.StdEncoding.EncodeToString(key)
		if err := os.WriteFile(path, []byte(enc), 0o600); err != nil {
			return nil, err
		}
		return key, nil
	}
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(string(data))
}

func keyFromKeyring(create bool) ([]byte, error) {
	enc, err := activeKeyring.Get(keyringService, keyringUser)
	if err == nil {
		return base64.StdEncoding.DecodeString(enc)
	}
	if !create {
		return nil, &AuthError{Reason: "encryption key not found in keyring"}
	}
	key, err := newKey()
	if err != nil {
		return nil, err
	}
	if err := activeKeyring.Set(keyringService, keyringUser,
		base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, err
	}
	return key, nil
}

// newKey returns 32 cryptographically random bytes for AES-256.
func newKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}
