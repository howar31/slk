package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStore_SaveLoadRoundTrip(t *testing.T) {
	t.Setenv(keyEnvVar, backendFile)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := &Config{
		Active: "work",
		Profiles: map[string]Profile{
			"work": {Workspace: "acme", UserToken: "xoxp-1", BotToken: "xoxb-1"},
		},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config perms = %o, want 600", info.Mode().Perm())
	}

	// On-disk tokens must be ciphertext, not the raw values.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if strings.Contains(string(raw), "xoxp-1") || strings.Contains(string(raw), "xoxb-1") {
		t.Fatalf("config file leaked a plaintext token:\n%s", raw)
	}

	// The key file is created beside the config, mode 0600.
	keyInfo, err := os.Stat(filepath.Join(dir, ".encryption_key"))
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("key file perms = %o, want 600", keyInfo.Mode().Perm())
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Active != "work" || loaded.Profiles["work"].UserToken != "xoxp-1" {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}
	if loaded.Profiles["work"].BotToken != "xoxb-1" {
		t.Fatalf("bot token round trip mismatch: %+v", loaded)
	}
}

func TestStore_LoadMissingFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
}

func TestStore_PlaintextReadableEncryptedOnNextSave(t *testing.T) {
	t.Setenv(keyEnvVar, backendFile)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Hand-written plaintext config (e.g. a config slk has never saved).
	legacy := `active = "work"

[profiles.work]
workspace = "acme"
user_token = "xoxp-legacy"
bot_token = "xoxb-legacy"
`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("seed plaintext: %v", err)
	}

	// Load tolerates plaintext: the token resolves and Load does NOT rewrite.
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Profiles["work"].UserToken != "xoxp-legacy" {
		t.Fatalf("plaintext token not readable: %+v", loaded)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw after load: %v", err)
	}
	if !strings.Contains(string(raw), "xoxp-legacy") {
		t.Fatalf("Load must not rewrite the file (B: no eager migration):\n%s", raw)
	}

	// The next Save (e.g. any auth write command) encrypts the plaintext.
	if err := Save(path, loaded); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw after save: %v", err)
	}
	if strings.Contains(string(raw), "xoxp-legacy") || strings.Contains(string(raw), "xoxb-legacy") {
		t.Fatalf("Save did not encrypt the plaintext fields:\n%s", raw)
	}
	if !strings.Contains(string(raw), `key_backend = "file"`) {
		t.Fatalf("Save did not record key_backend:\n%s", raw)
	}

	// The encrypted form still resolves to the original values.
	again, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if again.Profiles["work"].UserToken != "xoxp-legacy" {
		t.Fatalf("post-encrypt decrypt mismatch: %+v", again)
	}
}
