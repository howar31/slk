package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoad_LegacyDualTokenIgnored locks the clean-break migration (design §8):
// a pre-decouple config with the old user_token/bot_token keys loads without
// error, those keys are ignored, and the profile resolves to a clean "no token"
// error rather than crashing or resurrecting a stale credential.
func TestLoad_LegacyDualTokenIgnored(t *testing.T) {
	t.Setenv(keyEnvVar, backendFile)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	legacy := "active = \"work\"\nkey_backend = \"file\"\n\n[profiles.work]\nuser_token = \"xoxp-old\"\nbot_token = \"xoxb-old\"\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.Profiles["work"].Token; got != "" {
		t.Fatalf("legacy user_token/bot_token must be ignored, got Token=%q", got)
	}
	if _, err := ResolveToken(cfg, "work", "", ""); err == nil {
		t.Fatal("expected a 'no token' error for a legacy-only profile")
	}
}

func TestStore_SaveLoadRoundTrip(t *testing.T) {
	t.Setenv(keyEnvVar, backendFile)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := &Config{
		Active: "work",
		Profiles: map[string]Profile{
			"work": {Token: "xoxb-1"},
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

	// On-disk token must be ciphertext, not the raw value.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if strings.Contains(string(raw), "xoxb-1") {
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
	if loaded.Active != "work" || loaded.Profiles["work"].Token != "xoxb-1" {
		t.Fatalf("round trip mismatch: %+v", loaded)
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
token = "xoxp-legacy"
`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("seed plaintext: %v", err)
	}

	// Load tolerates plaintext: the token resolves and Load does NOT rewrite.
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Profiles["work"].Token != "xoxp-legacy" {
		t.Fatalf("plaintext token not readable: %+v", loaded)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw after load: %v", err)
	}
	if !strings.Contains(string(raw), "xoxp-legacy") {
		t.Fatalf("Load must not rewrite the file (no eager migration):\n%s", raw)
	}

	// The next Save (e.g. any auth write command) encrypts the plaintext.
	if err := Save(path, loaded); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw after save: %v", err)
	}
	if strings.Contains(string(raw), "xoxp-legacy") {
		t.Fatalf("Save did not encrypt the plaintext token:\n%s", raw)
	}
	if !strings.Contains(string(raw), `key_backend = "file"`) {
		t.Fatalf("Save did not record key_backend:\n%s", raw)
	}

	// The encrypted form still resolves to the original value.
	again, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if again.Profiles["work"].Token != "xoxp-legacy" {
		t.Fatalf("post-encrypt decrypt mismatch: %+v", again)
	}
}

func TestSaveLoad_SingleTokenRoundTrip(t *testing.T) {
	t.Setenv(keyEnvVar, backendFile)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := &Config{
		Active:   "work",
		Profiles: map[string]Profile{"work": {Token: "xoxb-work-bot"}},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "xoxb-work-bot") {
		t.Fatal("token stored in plaintext")
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := loaded.Profiles["work"].Token; got != "xoxb-work-bot" {
		t.Fatalf("Token = %q, want xoxb-work-bot", got)
	}
}
