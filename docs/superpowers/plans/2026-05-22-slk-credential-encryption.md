# slk Credential Encryption-at-Rest Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop writing Slack credentials to disk in plaintext — encrypt the sensitive fields at rest with AES-256-GCM, holding the encryption key in the OS keyring or a key file. Tokens set via `set-token`/`login` are encrypted on write; any pre-existing plaintext is read transparently and encrypted on the next write (no re-auth).

**Architecture:** A key-provider layer resolves a 32-byte AES key from one of two backends — a key file (`~/.config/slk/.encryption_key`, default for headless/agent use) or the OS keyring (opt-in, real OS protection). `auth.Save` encrypts the sensitive profile fields before writing TOML; `auth.Load` decrypts `enc:v1:` fields back into memory (best-effort) and passes any plaintext through unchanged, so a stray plaintext token still resolves and is encrypted on the next `Save`. `Load` never rewrites the file. Token-resolution precedence and the `SLK_TOKEN` bypass are untouched because `ResolveToken` already returns before reading the config.

**Tech Stack:** Go 1.25, standard-library `crypto/aes` + `crypto/cipher` (GCM) + `crypto/rand`, `github.com/zalando/go-keyring` for the opt-in keyring backend, `github.com/BurntSushi/toml` (existing) for config persistence.

---

## Project rules that override the writing-plans skill defaults

These come from `/opt/projects/slk/CLAUDE.md` and the user; they win over the skill template:

- **One commit per feature, after explicit approval.** Do NOT commit per task. Tasks 1–6 implement and test with NO commit. Task 7 presents a summary, waits for the user's explicit approval, then makes a single `feat:` commit via the `/commit` skill. Never commit automatically.
- **Public-repo language constraint:** describe the design in slk's own terms only. Keep code comments, SPEC.md, README.md, CLAUDE.md, test names, and the commit message free of references or comparisons to other tools. The commit message stays neutral.
- **If executed by a dispatched subagent:** the subagent must NOT run any commit-helper or documentation skill, and must NOT create or edit SPEC.md / CLAUDE.md / top-level README.md. Those (Task 7 docs + commit) are done by the main session only.
- **Never print or log token strings, decrypted credentials, or the raw encryption key.** `auth status` shows presence booleans only.
- **Scrubbed fixtures only:** `Alice` / `Bob` / `C0123456789` / `U0123456789` / `xoxp-…` dummy tokens. No real names, IDs, or tokens.
- Code comments and Go test names in English. Test names use `Test<Subject>_<Behavior>`.

---

## Design decisions (locked with the user)

- **On-disk layout:** keep the existing multi-profile `config.toml`; encrypt the sensitive fields *in place*. An encrypted value is the string `enc:v1:<base64(nonce‖ciphertext‖tag)>`. The `enc:v1:` prefix both marks ciphertext and gives a versioning hook. (slk is multi-profile, so a single opaque blob file is not used.)
- **Sensitive fields:** `user_token`, `bot_token`, `client_secret`. `client_id` and `workspace` stay plaintext (not secret).
- **Backend selection env var:** `SLK_KEYRING_BACKEND` with values `file` | `keyring` | `auto`.
- **Default = `auto`:** at key-creation time, probe the OS keyring; if usable, use keyring (desktop gets real OS protection); otherwise fall back to the file backend (headless agents / CI run with zero interaction). The resolved backend is recorded in `config.toml` as `key_backend` so later reads are deterministic.
- **Backend precedence (where the key is):** recorded `key_backend` in the config wins (that is where an existing key actually lives); else the `SLK_KEYRING_BACKEND` env value; else auto-probe. Changing backend on an existing key is a future `auth rekey` (out of scope).
- **Key file location:** `<dir of config.toml>/.encryption_key` (so it is `~/.config/slk/.encryption_key` in production and a temp dir under `SLK_CONFIG` in tests), 32 random bytes base64-encoded, mode `0600`.
- **Plaintext handling (no eager migration):** `Load` decrypts only `enc:v1:` fields and passes any plaintext through unchanged — a stray or hand-written plaintext token still resolves and works, and is encrypted on the next `Save` (any `set-token` / `login` / `switch` / `logout`). `Load` never rewrites the file. Normal flow never produces plaintext on disk because `set-token` / `login` always go through `Save`. No re-auth is ever required. slk does not actively reject plaintext: that would be dead code in normal flow and would only break an otherwise-valid token.
- **Threat model (accepted):** the file backend defends against *accidental* disclosure (dotfile sync, screen-share, casual `cat`, backup leakage), not a local attacker who can already read `~/.config/slk/`. The keyring backend provides real local protection. This nuance goes in README in slk's own words.
- **Out of scope:** a token-export/print command (deliberately omitted — slk uses tokens, it does not hand them out); eager auto-migration of existing plaintext configs (we tolerate-on-read and encrypt-on-next-write instead); key rotation / `auth rekey`; AAD binding of ciphertext to field names.

## Files

| File | Action | Responsibility |
|---|---|---|
| `internal/auth/crypto.go` | Create | AES-256-GCM encrypt/decrypt of a single string field; `enc:v1:` prefix detection. |
| `internal/auth/crypto_test.go` | Create | Round-trip, tamper-detection, prefix detection unit tests. |
| `internal/auth/keyprovider.go` | Create | Backend resolution, keyring abstraction, key load/create for file + keyring backends, auto-probe. |
| `internal/auth/keyprovider_test.go` | Create | File-backend create/read, backend-resolution precedence, auto-probe, keyring backend (against fake), plus the shared `fakeKeyring`. |
| `internal/auth/main_test.go` | Create | `TestMain` that swaps in the in-memory `fakeKeyring` so no auth-package test ever touches the real OS keyring. |
| `internal/auth/store.go` | Modify | Add `Config.KeyBackend`; encrypt on `Save`; decrypt `enc:` fields on `Load` (plaintext passes through, no rewrite); factor the atomic write into `writeConfig`. |
| `internal/auth/store_test.go` | Modify | Force file backend; assert no plaintext token on disk; add a plaintext-tolerance test (Load reads it without rewriting; next Save encrypts). |
| `internal/auth/token.go` | Modify | Guard: a still-encrypted (undecryptable) selected token becomes an `*AuthError` (exit 3). |
| `internal/auth/precedence_test.go` | Modify | Force file backend so Save/Load round-trips through encryption. |
| `internal/commands/auth.go` | Modify | `auth status` prints a one-line encryption indicator (no secrets). |
| `internal/commands/auth_test.go` | Modify | Force file backend; assert no plaintext on disk and that the encryption line appears. |
| `go.mod` / `go.sum` | Modify | Add `github.com/zalando/go-keyring`. |
| `SPEC.md` / `README.md` | Modify (Task 7) | Document the at-rest encryption model in slk's own terms. |

---

### Task 1: Add the keyring dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add the dependency**

Run:
```bash
cd /opt/projects/slk && go get github.com/zalando/go-keyring@latest && go mod tidy
```
Expected: `go.mod` gains a `github.com/zalando/go-keyring` require line; `go.sum` updated; command exits 0.

- [ ] **Step 2: Confirm the library's public API matches what later tasks assume**

Run:
```bash
cd /opt/projects/slk && go doc github.com/zalando/go-keyring | grep -E 'func (Set|Get|Delete)|ErrNotFound'
```
Expected: shows `func Set(service, user, password string) error`, `func Get(service, user string) (string, error)`, `func Delete(service, user string) error`, and `var ErrNotFound`. If the signatures differ, adjust the `osKeyring` wrapper in Task 3 Step 3 accordingly (this is the only place that touches the library directly).

- [ ] **Step 3: Verify the module still builds**

Run:
```bash
cd /opt/projects/slk && go build ./...
```
Expected: exits 0, no output.

---

### Task 2: AES-256-GCM field crypto

**Files:**
- Create: `internal/auth/crypto.go`
- Test: `internal/auth/crypto_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/auth/crypto_test.go`:
```go
package auth

import (
	"crypto/rand"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return k
}

func TestCrypto_EncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	const plain = "xoxp-secret-value"
	enc, err := encryptValue(key, plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, encPrefix) {
		t.Fatalf("ciphertext missing %q prefix: %q", encPrefix, enc)
	}
	if strings.Contains(enc, plain) {
		t.Fatalf("ciphertext leaked plaintext: %q", enc)
	}
	got, err := decryptValue(key, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("round trip = %q, want %q", got, plain)
	}
}

func TestCrypto_DecryptWithWrongKeyFails(t *testing.T) {
	enc, err := encryptValue(testKey(t), "xoxp-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := decryptValue(testKey(t), enc); err == nil {
		t.Fatal("decrypt with a different key should fail (GCM auth)")
	}
}

func TestCrypto_NonceIsUniquePerEncryption(t *testing.T) {
	key := testKey(t)
	a, _ := encryptValue(key, "same")
	b, _ := encryptValue(key, "same")
	if a == b {
		t.Fatal("two encryptions of the same plaintext must differ (random nonce)")
	}
}

func TestCrypto_IsEncrypted(t *testing.T) {
	if !isEncrypted(encPrefix + "abc") {
		t.Fatal("prefixed value should be detected as encrypted")
	}
	if isEncrypted("xoxp-plain") {
		t.Fatal("plaintext token must not be detected as encrypted")
	}
	if isEncrypted("") {
		t.Fatal("empty string is not encrypted")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /opt/projects/slk && go test ./internal/auth/ -run TestCrypto -v`
Expected: FAIL — `undefined: encryptValue` / `decryptValue` / `encPrefix` / `isEncrypted`.

- [ ] **Step 3: Implement the crypto helpers**

Create `internal/auth/crypto.go`:
```go
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

// encPrefix marks an encrypted field value and versions the scheme.
const encPrefix = "enc:v1:"

// isEncrypted reports whether v is an encrypted field value.
func isEncrypted(v string) bool { return strings.HasPrefix(v, encPrefix) }

// encryptValue seals plaintext with AES-256-GCM and returns
// enc:v1:<base64(nonce||ciphertext||tag)>. key must be 32 bytes.
func encryptValue(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptValue reverses encryptValue. It returns an error if the value is not
// valid ciphertext or fails GCM authentication (wrong key / tampering).
func decryptValue(key []byte, value string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /opt/projects/slk && go test ./internal/auth/ -run TestCrypto -v`
Expected: PASS (4 tests).

---

### Task 3: Key-provider layer (backends + resolution)

**Files:**
- Create: `internal/auth/keyprovider.go`
- Create: `internal/auth/keyprovider_test.go`
- Create: `internal/auth/main_test.go`

- [ ] **Step 1: Write the test scaffolding and failing tests**

Create `internal/auth/keyprovider_test.go`:
```go
package auth

import (
	"path/filepath"
	"testing"
)

// fakeKeyring is an in-memory keyringStore for tests. It can simulate an
// unavailable keyring via the unavailable flag.
type fakeKeyring struct {
	items       map[string]string
	unavailable bool
}

func newFakeKeyring() *fakeKeyring { return &fakeKeyring{items: map[string]string{}} }

func (f *fakeKeyring) key(s, u string) string { return s + "\x00" + u }

func (f *fakeKeyring) Get(s, u string) (string, error) {
	if f.unavailable {
		return "", errKeyringUnavailable
	}
	v, ok := f.items[f.key(s, u)]
	if !ok {
		return "", errKeyNotFound
	}
	return v, nil
}

func (f *fakeKeyring) Set(s, u, p string) error {
	if f.unavailable {
		return errKeyringUnavailable
	}
	f.items[f.key(s, u)] = p
	return nil
}

func (f *fakeKeyring) Delete(s, u string) error {
	if f.unavailable {
		return errKeyringUnavailable
	}
	delete(f.items, f.key(s, u))
	return nil
}

// withFakeKeyring swaps activeKeyring for the duration of a test.
func withFakeKeyring(t *testing.T, f *fakeKeyring) {
	t.Helper()
	prev := activeKeyring
	activeKeyring = f
	t.Cleanup(func() { activeKeyring = prev })
}

func TestKeyProvider_FileBackendCreateThenRead(t *testing.T) {
	t.Setenv(keyEnvVar, backendFile)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")

	created, err := loadKey(backendFile, cfgPath, true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(created) != 32 {
		t.Fatalf("key length = %d, want 32", len(created))
	}
	read, err := loadKey(backendFile, cfgPath, false)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(read) != string(created) {
		t.Fatal("re-read key differs from created key")
	}
}

func TestKeyProvider_FileBackendMissingKeyErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if _, err := loadKey(backendFile, cfgPath, false); err == nil {
		t.Fatal("reading a missing key without create should error")
	}
}

func TestKeyProvider_KeyringBackendRoundTrip(t *testing.T) {
	withFakeKeyring(t, newFakeKeyring())
	created, err := loadKey(backendKeyring, "", true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	read, err := loadKey(backendKeyring, "", false)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(read) != string(created) {
		t.Fatal("keyring re-read differs")
	}
}

func TestResolveBackend_RecordedWinsOverEnv(t *testing.T) {
	t.Setenv(keyEnvVar, backendKeyring)
	cfg := &Config{KeyBackend: backendFile}
	if got := resolveBackend(cfg); got != backendFile {
		t.Fatalf("resolveBackend = %q, want recorded %q", got, backendFile)
	}
}

func TestResolveBackend_EnvWinsWhenNoRecord(t *testing.T) {
	t.Setenv(keyEnvVar, backendKeyring)
	if got := resolveBackend(&Config{}); got != backendKeyring {
		t.Fatalf("resolveBackend = %q, want env %q", got, backendKeyring)
	}
}

func TestResolveBackend_AutoPrefersKeyringWhenAvailable(t *testing.T) {
	t.Setenv(keyEnvVar, backendAuto)
	withFakeKeyring(t, newFakeKeyring())
	if got := resolveBackend(&Config{}); got != backendKeyring {
		t.Fatalf("auto with usable keyring = %q, want %q", got, backendKeyring)
	}
}

func TestResolveBackend_AutoFallsBackToFile(t *testing.T) {
	t.Setenv(keyEnvVar, backendAuto)
	withFakeKeyring(t, &fakeKeyring{items: map[string]string{}, unavailable: true})
	if got := resolveBackend(&Config{}); got != backendFile {
		t.Fatalf("auto with unusable keyring = %q, want %q", got, backendFile)
	}
}
```

Create `internal/auth/main_test.go`:
```go
package auth

import (
	"os"
	"testing"
)

// TestMain guarantees no auth-package test ever touches the real OS keyring:
// the default keyring is replaced with an in-memory fake. Individual tests may
// still override activeKeyring via withFakeKeyring.
func TestMain(m *testing.M) {
	activeKeyring = newFakeKeyring()
	os.Exit(m.Run())
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /opt/projects/slk && go test ./internal/auth/ -run 'TestKeyProvider|TestResolveBackend' -v`
Expected: FAIL — `undefined: loadKey` / `resolveBackend` / `activeKeyring` / `keyEnvVar` / `backendFile` / `backendKeyring` / `backendAuto` / `errKeyNotFound` / `errKeyringUnavailable`.

- [ ] **Step 3: Implement the key provider**

Create `internal/auth/keyprovider.go`:
```go
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
```

Note: the test fake returns `errKeyNotFound` from `Get`, which `keyFromKeyring` treats like any non-nil error (falls through to create/error) — it does not need to equal `keyring.ErrNotFound`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /opt/projects/slk && go test ./internal/auth/ -run 'TestKeyProvider|TestResolveBackend' -v`
Expected: PASS (7 tests).

---

### Task 4: Encrypt on Save, decrypt on Load (tolerate plaintext, no eager migration)

**Files:**
- Modify: `internal/auth/store.go`
- Modify: `internal/auth/store_test.go`

- [ ] **Step 1: Update existing store tests and add a plaintext-tolerance test (these fail first)**

Replace the entire contents of `internal/auth/store_test.go` with:
```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /opt/projects/slk && go test ./internal/auth/ -run TestStore -v`
Expected: FAIL — `TestStore_SaveLoadRoundTrip` finds the plaintext token still on disk (Save does not yet encrypt) and the `.encryption_key` stat fails; `TestStore_PlaintextReadableEncryptedOnNextSave` fails at the post-Save assertion because the current `Save` does not encrypt.

- [ ] **Step 3: Wire encryption into store.go**

Replace the entire contents of `internal/auth/store.go` with:
```go
// Package auth manages slk credentials: profiles, token resolution, OAuth.
package auth

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Profile holds credentials for one Slack workspace. The token and secret
// fields are stored encrypted at rest (see crypto.go); they are plaintext in
// memory after Load and re-encrypted by Save.
type Profile struct {
	Workspace    string `toml:"workspace"`
	UserToken    string `toml:"user_token,omitempty"`
	BotToken     string `toml:"bot_token,omitempty"`
	ClientID     string `toml:"client_id,omitempty"`
	ClientSecret string `toml:"client_secret,omitempty"`
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
		changed := false
		for _, f := range []*string{&p.UserToken, &p.BotToken, &p.ClientSecret} {
			if *f == "" || !isEncrypted(*f) {
				continue
			}
			if !ensureKey() {
				continue
			}
			if pt, err := decryptValue(key, *f); err == nil {
				*f = pt
				changed = true
			}
		}
		if changed {
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

// encryptProfile returns a copy of p with each non-empty, not-already-encrypted
// sensitive field encrypted.
func encryptProfile(key []byte, p Profile) (Profile, error) {
	for _, f := range []*string{&p.UserToken, &p.BotToken, &p.ClientSecret} {
		if *f == "" || isEncrypted(*f) {
			continue
		}
		v, err := encryptValue(key, *f)
		if err != nil {
			return p, err
		}
		*f = v
	}
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
```

- [ ] **Step 4: Run the store tests to verify they pass**

Run: `cd /opt/projects/slk && go test ./internal/auth/ -run TestStore -v`
Expected: PASS (3 tests).

---

### Task 5: Reject undecryptable tokens in ResolveToken

**Files:**
- Modify: `internal/auth/token.go`
- Modify: `internal/auth/precedence_test.go`

- [ ] **Step 1: Add the failing test and force the file backend in the matrix test**

In `internal/auth/precedence_test.go`, add `t.Setenv(keyEnvVar, backendFile)` as the first line inside `TestResolveToken_PrecedenceMatrix` (immediately after the `func ... {` line, before `dir := t.TempDir()`), so Save/Load round-trip through encryption deterministically.

Then append this new test to the same file:
```go
func TestResolveToken_UndecryptableTokenErrors(t *testing.T) {
	// A profile whose token is ciphertext that no key can open (decryption was
	// left to fail at Load) must surface as an *AuthError, not a leaked blob.
	cfg := &Config{
		Active: "work",
		Profiles: map[string]Profile{
			"work": {UserToken: encPrefix + "bm90LXJlYWwtY2lwaGVydGV4dA=="},
		},
	}
	_, err := ResolveToken(cfg, "", "user", "")
	if err == nil {
		t.Fatal("expected an error for an undecryptable token")
	}
	if _, ok := err.(*AuthError); !ok {
		t.Fatalf("expected *AuthError (exit 3), got %T", err)
	}

	// SLK_TOKEN must still bypass everything, even with a broken stored token.
	tok, err := ResolveToken(cfg, "", "user", "xoxp-env")
	if err != nil {
		t.Fatalf("env override should bypass the broken token: %v", err)
	}
	if tok != "xoxp-env" {
		t.Fatalf("env token = %q, want xoxp-env", tok)
	}
}
```

- [ ] **Step 2: Run the tests to verify the new one fails**

Run: `cd /opt/projects/slk && go test ./internal/auth/ -run 'TestResolveToken_Undecryptable|TestResolveToken_PrecedenceMatrix' -v`
Expected: `TestResolveToken_UndecryptableTokenErrors` FAILS (it currently returns the raw `enc:v1:…` string with no error); the precedence matrix still passes.

- [ ] **Step 3: Add the ciphertext guard to ResolveToken**

Replace the entire contents of `internal/auth/token.go` with:
```go
package auth

import "fmt"

// ResolveToken picks the active token. Precedence: envToken, then the named
// profile (or cfg.Active if profileName is empty). identity is "user" or "bot".
func ResolveToken(cfg *Config, profileName, identity, envToken string) (string, error) {
	if envToken != "" {
		return envToken, nil
	}
	name := profileName
	if name == "" {
		name = cfg.Active
	}
	if name == "" {
		return "", &AuthError{Reason: "no profile selected; run 'slk auth set-token' or set SLK_TOKEN"}
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return "", &AuthError{Reason: fmt.Sprintf("profile %q not found", name)}
	}

	var tok string
	switch identity {
	case "bot":
		if p.BotToken == "" {
			return "", &AuthError{Reason: fmt.Sprintf("profile %q has no bot token", name)}
		}
		tok = p.BotToken
	default:
		if p.UserToken == "" {
			return "", &AuthError{Reason: fmt.Sprintf("profile %q has no user token", name)}
		}
		tok = p.UserToken
	}

	// A still-encrypted value means Load could not decrypt it (the encryption
	// key was unavailable or wrong). Fail as an auth error rather than handing
	// back ciphertext.
	if isEncrypted(tok) {
		return "", &AuthError{Reason: fmt.Sprintf("profile %q token could not be decrypted (encryption key unavailable)", name)}
	}
	return tok, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /opt/projects/slk && go test ./internal/auth/ -run TestResolveToken -v`
Expected: PASS (all `TestResolveToken_*` including the new one and the matrix).

---

### Task 6: Encryption indicator in `auth status`

**Files:**
- Modify: `internal/commands/auth.go`
- Modify: `internal/commands/auth_test.go`

- [ ] **Step 1: Update the status test (fails first) and force the file backend in command tests**

In `internal/commands/auth_test.go`, add `t.Setenv("SLK_KEYRING_BACKEND", "file")` immediately after each existing `t.Setenv("SLK_CONFIG", cfgPath)` line (there are three: in `TestAuthLogout_MissingProfileErrors`, `TestAuthLogout_RemovesExisting`, `TestAuthSetTokenAndStatus`).

Then, in `TestAuthSetTokenAndStatus`, after the existing block that asserts the status output does not contain `xoxp-x` (the final `if strings.Contains(...)` check), add:
```go
	if !strings.Contains(out.String(), "encryption:") {
		t.Fatalf("status missing encryption indicator: %q", out.String())
	}
	if !strings.Contains(out.String(), "file") {
		t.Fatalf("status should report the file backend: %q", out.String())
	}

	// The encrypted token must not be on disk in plaintext either.
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read raw config: %v", err)
	}
	if strings.Contains(string(raw), "xoxp-x") {
		t.Fatalf("config file leaked the plaintext token:\n%s", raw)
	}
```

Add `"os"` to the import block of `internal/commands/auth_test.go` (it currently imports `bytes`, `path/filepath`, `strings`, `testing`, and `github.com/howar31/slk/internal/auth`).

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /opt/projects/slk && go test ./internal/commands/ -run TestAuthSetTokenAndStatus -v`
Expected: FAIL — status output has no `encryption:` line yet.

- [ ] **Step 3: Add the indicator to the status command**

In `internal/commands/auth.go`, in `newAuthStatusCommand`, replace the `RunE` body's profile loop tail so the function prints the encryption summary after listing profiles. Replace this existing block:
```go
				for name, p := range cfg.Profiles {
					marker := " "
					if name == cfg.Active {
						marker = "*"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s (workspace=%s user=%v bot=%v)\n",
						marker, name, p.Workspace, p.UserToken != "", p.BotToken != "")
				}
				return nil
```
with:
```go
				for name, p := range cfg.Profiles {
					marker := " "
					if name == cfg.Active {
						marker = "*"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s (workspace=%s user=%v bot=%v)\n",
						marker, name, p.Workspace, p.UserToken != "", p.BotToken != "")
				}
				fmt.Fprintln(cmd.OutOrStdout(), auth.EncryptionStatus(cfg))
				return nil
```

- [ ] **Step 4: Add the EncryptionStatus helper to the auth package**

`cfg.UserToken`/etc. are decrypted in memory after `Load`, so a remaining `enc:v1:` value signals a decryption failure. Add this exported helper. Create `internal/auth/status.go`:
```go
package auth

import "fmt"

// EncryptionStatus returns a single non-secret line describing at-rest
// encryption for display by `slk auth status`. It never includes any token,
// secret, or key material.
func EncryptionStatus(cfg *Config) string {
	if cfg.KeyBackend == "" {
		return "encryption: none (legacy plaintext)"
	}
	for _, p := range cfg.Profiles {
		if isEncrypted(p.UserToken) || isEncrypted(p.BotToken) || isEncrypted(p.ClientSecret) {
			return fmt.Sprintf("encryption: %s (error: key unavailable)", cfg.KeyBackend)
		}
	}
	return fmt.Sprintf("encryption: %s (ok)", cfg.KeyBackend)
}
```

- [ ] **Step 5: Run the command tests to verify they pass**

Run: `cd /opt/projects/slk && go test ./internal/commands/ -run TestAuth -v`
Expected: PASS (all `TestAuth*` tests).

---

### Task 7: Documentation, full verification, manual smoke test, single commit

**Files:**
- Modify: `SPEC.md`
- Modify: `README.md`
- (Verification only) the live config at `~/.config/slk/config.toml`

> Main session only. If this plan is being executed by a dispatched subagent, STOP after Task 6 and hand back to the main session for this task — per project rules a subagent must not edit SPEC.md / README.md or run the commit skill.

- [ ] **Step 1: Run the full test suite, uncached**

Run:
```bash
cd /opt/projects/slk && go clean -testcache && go test ./...
```
Expected: all packages PASS. If a non-auth command test now fails because it persists a config and hits the auto-probe, add `t.Setenv("SLK_KEYRING_BACKEND", "file")` to that test (same fix as Task 6 Step 1) and re-run. Empty/missing-config tests are unaffected (Load returns before any key work).

- [ ] **Step 2: Confirm `go vet` and build are clean**

Run:
```bash
cd /opt/projects/slk && go vet ./... && go build -ldflags "-X main.version=dev" -o slk ./cmd/slk
```
Expected: no vet output; `slk` binary produced.

- [ ] **Step 3: Update SPEC.md**

In `SPEC.md`, under `## Architecture` add the encryption to the external-dependency list (`github.com/zalando/go-keyring` for the opt-in keyring backend) and under "Runtime state" replace the line describing token storage. The new runtime-state wording (no reference to any other tool):
> - OAuth tokens live in `~/.config/slk/config.toml` (mode `0600`, TOML-encoded, multi-profile). The `user_token`, `bot_token`, and `client_secret` fields are encrypted at rest with AES-256-GCM (`enc:v1:` prefix). The 32-byte key is held in the OS keyring or, for headless/agent use, a key file at `~/.config/slk/.encryption_key` (mode `0600`); the backend is chosen by `SLK_KEYRING_BACKEND` (`auto` default — keyring if available, else file) and recorded as `key_backend` in the config. Tokens set via `set-token`/`login` are encrypted on write; a pre-existing plaintext value is still read and is encrypted on the next write (no re-auth).

Also add `internal/auth/crypto.go`, `internal/auth/keyprovider.go`, and `internal/auth/status.go` to the layout tree under `internal/auth/`, and add a `## Key Decisions` bullet:
> - **Credentials are encrypted at rest, not just file-permissioned.** Sensitive fields use AES-256-GCM with the key in the OS keyring (opt-in, real local protection) or a key file (default; defends against accidental disclosure such as dotfile sync or screen-share, but not a local attacker who can already read `~/.config/slk/`). Default backend is `auto` because slk runs headless-first and must never block on an interactive keychain unlock.

- [ ] **Step 4: Update README.md**

In `README.md`, document (in human-facing language, no reference to any other CLI) that tokens are encrypted at rest, the `SLK_KEYRING_BACKEND` env var and its `auto`/`file`/`keyring` values, the `.encryption_key` file, and the honest threat-model note (protects against accidental disclosure; the keyring backend adds OS-level protection; the file backend's key sits beside the config so it is not a defense against a local attacker who can read that directory). Note that tokens set via the normal commands are encrypted automatically; any pre-existing plaintext keeps working and is encrypted on the next write (no re-auth).

- [ ] **Step 5: Manual smoke test against the live config**

Back up first. Under option B, `Load` does not rewrite the file, so encrypting the existing (plaintext) config requires one Save-triggering command. Pin the file backend for predictable, prompt-free testing on a desktop (otherwise `auto` may pick the macOS Keychain):
```bash
export SLK_KEYRING_BACKEND=file
cp ~/.config/slk/config.toml /tmp/slk-config.bak.toml

# 1. Reading the existing plaintext config still works; note the active profile.
./slk auth status        # prints profiles + an "encryption:" line; never prints a token

# 2. Trigger a Save to encrypt it in place (re-select the active profile from step 1).
./slk auth switch <active-profile-name-from-step-1>

# 3. Confirm no plaintext token remains and the decrypt path works end to end.
grep -aoE "xox[bp]-" ~/.config/slk/config.toml || echo "OK: no plaintext slack tokens on disk"
./slk auth status                       # now: encryption: file (ok)
./slk search channels --format concise  # requires a valid token; exercises the decrypt path
```
Expected: step 1 status shows `encryption: none (legacy plaintext)` and never prints a token; after step 2 the grep prints the "OK" line and status shows `encryption: file (ok)`; the read command succeeds.
IF the read command fails with an auth error: restore with `cp /tmp/slk-config.bak.toml ~/.config/slk/config.toml` and debug before proceeding.
Caution: once the config is encrypted, the older Homebrew-installed `slk` (0.1.0) can no longer read it (it would send `enc:v1:…` as the token). Use the freshly built binary until the encryption-capable release ships.

- [ ] **Step 6: Present a summary and commit (single feat commit, after approval)**

Show the user the full diff summary and wait for explicit approval. Then, in the main session, run the `/commit` skill (it also syncs SPEC.md). The commit must be a single neutral conventional commit, e.g.:
```
feat(auth): encrypt credentials at rest with AES-256-GCM
```
Do NOT mention the design's lineage or any other tool in the message. Do NOT commit without approval.

---

## Self-Review

- **Spec coverage:** PRIORITY 1 (AES-256-GCM at-rest for the three sensitive fields) → Tasks 2 + 4. PRIORITY 2 (file + keyring backends; plaintext tolerated on read and encrypted on the next write, no re-auth) → Tasks 3 + 4. PRIORITY 3 / threat-model + design confirmation → locked in "Design decisions" and surfaced in docs (Task 7). Invariants: `SLK_TOKEN` bypass + precedence → preserved (token.go change only adds a guard after selection; covered by `TestResolveToken_PrecedenceMatrix` and `TestResolveToken_UndecryptableTokenErrors`). `SLK_CONFIG` → unchanged, still covered by the matrix test. 0600/0700 perms → asserted in `TestStore_SaveLoadRoundTrip` (config and key file). Decryption failure → exit 3 → `TestResolveToken_UndecryptableTokenErrors`. Never-log-tokens → `auth status` asserted not to leak in `TestAuthSetTokenAndStatus`.
- **Placeholder scan:** none — every code step contains full code; every type/function referenced (`encryptValue`, `decryptValue`, `isEncrypted`, `encPrefix`, `loadKey`, `resolveBackend`, `activeKeyring`, `keyringStore`, `errKeyNotFound`, `errKeyringUnavailable`, `decryptInPlace`, `encryptProfile`, `writeConfig`, `EncryptionStatus`, `Config.KeyBackend`, the `backend*`/`keyEnvVar` consts) is defined in Tasks 2–6.
- **Type consistency:** `loadKey(backend, configPath string, create bool) ([]byte, error)`, `resolveBackend(cfg *Config) string`, `encryptValue(key []byte, plaintext string) (string, error)`, `decryptValue(key []byte, value string) (string, error)` are used identically across store.go, token.go, and the tests. The keyring fake implements the same `keyringStore` interface used by `osKeyring`.

## Verification approach (summary)

- **Unit:** crypto round-trip / wrong-key / unique-nonce / prefix (Task 2); key file create+read, missing-key error, keyring round-trip, backend-resolution precedence, auto-probe both ways (Task 3); save→encrypt→load→decrypt, no-plaintext-on-disk, plaintext-tolerated-then-encrypted-on-next-save (Task 4); undecryptable-token→AuthError and SLK_TOKEN-still-bypasses (Task 5); status indicator + no-leak (Task 6).
- **Integration/full suite:** `go clean -testcache && go test ./...` plus `go vet` and build (Task 7 Steps 1–2).
- **Manual smoke:** real config encryption (via an explicit Save trigger) + a live read command, with a backup and a restore fallback (Task 7 Step 5).
