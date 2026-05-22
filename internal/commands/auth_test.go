package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/howar31/slk/internal/auth"
)

func TestAuthLogout_MissingProfileErrors(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	// Seed an unrelated profile so the config file exists but does not
	// contain the profile we attempt to remove.
	set := newAuthCommand(&GlobalFlags{})
	set.SetArgs([]string{"set-token", "--profile", "work", "--user", "xoxp-x", "--workspace", "acme"})
	if err := set.Execute(); err != nil {
		t.Fatalf("seed set-token: %v", err)
	}

	logout := newAuthCommand(&GlobalFlags{})
	logout.SetArgs([]string{"logout", "ghost"})
	err := logout.Execute()
	if err == nil {
		t.Fatal("logout on missing profile should error")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("error %q does not mention the missing profile", err)
	}

	cfg, err := auth.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if _, ok := cfg.Profiles["work"]; !ok {
		t.Fatal("logout error path should not mutate other profiles")
	}
}

func TestAuthLogout_RemovesExisting(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	set := newAuthCommand(&GlobalFlags{})
	set.SetArgs([]string{"set-token", "--profile", "dummy", "--user", "xoxp-d", "--workspace", "test"})
	if err := set.Execute(); err != nil {
		t.Fatalf("seed set-token: %v", err)
	}

	logout := newAuthCommand(&GlobalFlags{})
	var out bytes.Buffer
	logout.SetOut(&out)
	logout.SetArgs([]string{"logout", "dummy"})
	if err := logout.Execute(); err != nil {
		t.Fatalf("logout existing: %v", err)
	}
	if !strings.Contains(out.String(), "removed profile") {
		t.Fatalf("missing success line: %q", out.String())
	}

	cfg, err := auth.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if _, ok := cfg.Profiles["dummy"]; ok {
		t.Fatal("profile still present after logout")
	}
}

func TestAuthSetTokenAndStatus(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	set := newAuthCommand(&GlobalFlags{})
	set.SetArgs([]string{"set-token", "--profile", "work", "--user", "xoxp-x", "--workspace", "acme"})
	if err := set.Execute(); err != nil {
		t.Fatalf("set-token: %v", err)
	}

	cfg, err := auth.Load(cfgPath)
	if err != nil || cfg.Profiles["work"].UserToken != "xoxp-x" {
		t.Fatalf("token not persisted: %+v err=%v", cfg, err)
	}

	status := newAuthCommand(&GlobalFlags{})
	var out bytes.Buffer
	status.SetOut(&out)
	status.SetArgs([]string{"status"})
	if err := status.Execute(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "work") {
		t.Fatalf("status missing profile: %q", out.String())
	}
	if strings.Contains(out.String(), "xoxp-x") {
		t.Fatalf("status output leaked the raw token: %q", out.String())
	}

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
}
