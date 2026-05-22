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
