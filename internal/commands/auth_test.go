package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/howar31/slk/internal/auth"
)

func TestAuthLogin_DefaultScopesIncludeExpanded(t *testing.T) {
	cmd := newAuthCommand(&GlobalFlags{})
	login, _, err := cmd.Find([]string{"login"})
	if err != nil {
		t.Fatalf("find login: %v", err)
	}
	def := login.Flags().Lookup("scopes").DefValue
	for _, s := range []string{
		"reactions:read", "files:write", "users.profile:write",
		"pins:read", "pins:write", "bookmarks:read", "bookmarks:write",
		"team:read", "emoji:read", "users:write", "dnd:read", "dnd:write",
		"usergroups:read", "usergroups:write",
	} {
		if !strings.Contains(def, s) {
			t.Errorf("default scopes missing %q", s)
		}
	}
}

func TestFormatAuthIdentity(t *testing.T) {
	raw := []byte(`{"ok":true,"url":"https://acme.slack.com/","team":"acme","user":"alice","team_id":"T0123456789","user_id":"U0123456789"}`)
	line, err := formatAuthIdentity(raw)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	for _, want := range []string{"acme", "T0123456789", "alice", "U0123456789", "https://acme.slack.com/"} {
		if !strings.Contains(line, want) {
			t.Errorf("identity line missing %q: %q", want, line)
		}
	}
}

func TestAuthRevoke_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newAuthCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"revoke"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("revoke dry-run: %v", err)
	}
	if !strings.Contains(out.String(), "auth.revoke") {
		t.Fatalf("dry-run did not name the method: %q", out.String())
	}
}

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

func TestSetToken_NonInteractiveNoTokenErrors(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	set := newAuthCommand(&GlobalFlags{})
	set.SetArgs([]string{"set-token", "--non-interactive", "--profile", "work"})
	if err := set.Execute(); err == nil {
		t.Fatal("expected error when no token in non-interactive mode")
	}

	cfg, err := auth.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := cfg.Profiles["work"]; ok {
		t.Fatal("a token-less profile must not be saved")
	}
}

func TestSetToken_UserTokenFromStdin(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	set := newAuthCommand(&GlobalFlags{})
	set.SetIn(strings.NewReader("xoxp-fromstdin\n"))
	set.SetArgs([]string{"set-token", "--non-interactive", "--profile", "work", "--user", "-"})
	if err := set.Execute(); err != nil {
		t.Fatalf("set-token --user -: %v", err)
	}
	cfg, err := auth.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.Profiles["work"].UserToken != "xoxp-fromstdin" {
		t.Fatalf("token not read from stdin: %+v", cfg.Profiles["work"])
	}
}

func TestSetToken_InteractiveFillsMissing(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	origTerm, origSecret := isTerminal, readSecret
	t.Cleanup(func() { isTerminal, readSecret = origTerm, origSecret })
	isTerminal = func(int) bool { return true }
	secrets := []string{"xoxp-interactive", ""} // user token, then bot (skipped)
	readSecret = func(int) ([]byte, error) {
		v := secrets[0]
		secrets = secrets[1:]
		return []byte(v), nil
	}

	set := newAuthCommand(&GlobalFlags{})
	set.SetIn(strings.NewReader("work\nacme\n")) // profile, workspace
	var out, errb bytes.Buffer
	set.SetOut(&out)
	set.SetErr(&errb)
	set.SetArgs([]string{"set-token"})
	if err := set.Execute(); err != nil {
		t.Fatalf("interactive set-token: %v", err)
	}

	cfg, err := auth.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	p := cfg.Profiles["work"]
	if p.UserToken != "xoxp-interactive" || p.Workspace != "acme" {
		t.Fatalf("profile not filled from prompts: %+v", p)
	}
	if p.BotToken != "" {
		t.Fatalf("empty bot prompt should skip: %+v", p)
	}
}

func TestSetToken_InteractiveKeepsExistingToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	seed := newAuthCommand(&GlobalFlags{})
	seed.SetArgs([]string{"set-token", "--non-interactive", "--profile", "work", "--user", "xoxp-orig"})
	if err := seed.Execute(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	origTerm, origSecret := isTerminal, readSecret
	t.Cleanup(func() { isTerminal, readSecret = origTerm, origSecret })
	isTerminal = func(int) bool { return true }
	readSecret = func(int) ([]byte, error) { return []byte(""), nil } // keep user, skip bot

	upd := newAuthCommand(&GlobalFlags{})
	upd.SetIn(strings.NewReader("work\nnewlabel\n"))
	upd.SetArgs([]string{"set-token"})
	if err := upd.Execute(); err != nil {
		t.Fatalf("update: %v", err)
	}

	cfg, err := auth.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	p := cfg.Profiles["work"]
	if p.UserToken != "xoxp-orig" {
		t.Fatalf("existing token should be kept: %+v", p)
	}
	if p.Workspace != "newlabel" {
		t.Fatalf("workspace should update: %+v", p)
	}
}

func TestSetToken_NonInteractiveFlagRegistered(t *testing.T) {
	cmd := newAuthCommand(&GlobalFlags{})
	st, _, err := cmd.Find([]string{"set-token"})
	if err != nil {
		t.Fatalf("find set-token: %v", err)
	}
	if st.Flags().Lookup("non-interactive") == nil {
		t.Fatal("--non-interactive flag not registered")
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
