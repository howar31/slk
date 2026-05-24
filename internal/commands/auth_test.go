package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/auth"
)

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
	set.SetArgs([]string{"set-token", "--profile", "work", "--token", "xoxp-x", "--non-interactive"})
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
	set.SetArgs([]string{"set-token", "--profile", "dummy", "--token", "xoxp-d", "--non-interactive"})
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

func TestSetToken_TokenFromStdin(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	set := newAuthCommand(&GlobalFlags{})
	set.SetIn(strings.NewReader("xoxp-fromstdin\n"))
	set.SetArgs([]string{"set-token", "--non-interactive", "--profile", "work", "--token", "-"})
	if err := set.Execute(); err != nil {
		t.Fatalf("set-token --token -: %v", err)
	}
	cfg, err := auth.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.Profiles["work"].Token != "xoxp-fromstdin" {
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
	readSecret = func(int) ([]byte, error) { return []byte("xoxp-interactive"), nil }

	set := newAuthCommand(&GlobalFlags{})
	set.SetIn(strings.NewReader("work\n")) // profile
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
	if p.Token != "xoxp-interactive" {
		t.Fatalf("profile not filled from prompts: %+v", p)
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

// B3 Step-1: single --token flag stores token on disk.
func TestSetToken_SingleTokenFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	root := NewRootCommand("test")
	root.SetArgs([]string{"auth", "set-token", "--profile", "work", "--token", "xoxb-abc", "--non-interactive"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	cfg, err := auth.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.Profiles["work"].Token; got != "xoxb-abc" {
		t.Fatalf("Token = %q, want xoxb-abc", got)
	}
}

// B3 Step-1: unsupported token prefix is rejected.
func TestSetToken_RejectsUnsupportedPrefix(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	root := NewRootCommand("test")
	root.SetArgs([]string{"auth", "set-token", "--token", "xoxc-nope", "--non-interactive"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unsupported token prefix")
	}
}

func TestAuthSetTokenAndStatus(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	set := newAuthCommand(&GlobalFlags{})
	set.SetArgs([]string{"set-token", "--profile", "work", "--token", "xoxp-x", "--non-interactive"})
	if err := set.Execute(); err != nil {
		t.Fatalf("set-token: %v", err)
	}

	cfg, err := auth.Load(cfgPath)
	if err != nil || cfg.Profiles["work"].Token != "xoxp-x" {
		t.Fatalf("token not persisted: %+v err=%v", cfg, err)
	}

	status := newAuthCommand(&GlobalFlags{})
	var out bytes.Buffer
	status.SetOut(&out)
	status.SetArgs([]string{"status", "--offline"})
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

// seedProfile stores a token for name via set-token. The first profile
// seeded becomes the active one (set-token sets Active when it is empty).
func seedProfile(t *testing.T, name, token string) {
	t.Helper()
	set := newAuthCommand(&GlobalFlags{})
	set.SetArgs([]string{"set-token", "--profile", name, "--token", token, "--non-interactive"})
	if err := set.Execute(); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
}

// runStatus runs `auth status` with args and returns combined stdout.
func runStatus(t *testing.T, args ...string) string {
	t.Helper()
	status := newAuthCommand(&GlobalFlags{})
	var out bytes.Buffer
	status.SetOut(&out)
	status.SetArgs(args)
	if err := status.Execute(); err != nil {
		t.Fatalf("status %v: %v", args, err)
	}
	return out.String()
}

func TestAuthStatus_FlagsRegistered(t *testing.T) {
	cmd := newAuthCommand(&GlobalFlags{})
	st, _, err := cmd.Find([]string{"status"})
	if err != nil {
		t.Fatalf("find status: %v", err)
	}
	for _, f := range []string{"all", "offline"} {
		if st.Flags().Lookup(f) == nil {
			t.Errorf("--%s flag not registered on status", f)
		}
	}
}

func TestAuthStatus_ChecksActiveByDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	seedProfile(t, "work", "xoxp-work")     // active
	seedProfile(t, "personal", "xoxp-pers") // non-active

	orig := liveIdentity
	t.Cleanup(func() { liveIdentity = orig })
	liveIdentity = func(token string) (*authIdentity, error) {
		return &authIdentity{Team: "LIVE(" + token + ")"}, nil
	}

	out := runStatus(t, "status")
	if !strings.Contains(out, "LIVE(xoxp-work)") {
		t.Fatalf("active profile should be live-checked: %q", out)
	}
	if strings.Contains(out, "LIVE(xoxp-pers)") {
		t.Fatalf("non-active profile must not be checked by default: %q", out)
	}
}

func TestAuthStatus_AllChecksEveryProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	seedProfile(t, "work", "xoxp-work")
	seedProfile(t, "personal", "xoxp-pers")

	orig := liveIdentity
	t.Cleanup(func() { liveIdentity = orig })
	liveIdentity = func(token string) (*authIdentity, error) {
		return &authIdentity{Team: "LIVE(" + token + ")"}, nil
	}

	out := runStatus(t, "status", "--all")
	if !strings.Contains(out, "LIVE(xoxp-work)") || !strings.Contains(out, "LIVE(xoxp-pers)") {
		t.Fatalf("--all should live-check every profile: %q", out)
	}
}

func TestAuthStatus_OfflineSkipsCheck(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	seedProfile(t, "work", "xoxp-work")

	orig := liveIdentity
	t.Cleanup(func() { liveIdentity = orig })
	calls := 0
	liveIdentity = func(token string) (*authIdentity, error) {
		calls++
		return &authIdentity{Team: "LIVE"}, nil
	}

	out := runStatus(t, "status", "--offline")
	if calls != 0 {
		t.Fatalf("--offline must not call the live check, got %d calls", calls)
	}
	if strings.Contains(out, "LIVE") {
		t.Fatalf("--offline output must stay local-only: %q", out)
	}
	if !strings.Contains(out, "work") {
		t.Fatalf("--offline must still list profiles: %q", out)
	}
}

func TestAuthStatus_NetworkDownNote(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	seedProfile(t, "work", "xoxp-work")

	orig := liveIdentity
	t.Cleanup(func() { liveIdentity = orig })
	liveIdentity = func(token string) (*authIdentity, error) {
		return nil, errors.New("dial tcp: connection refused")
	}

	out := runStatus(t, "status")
	if !strings.Contains(out, "offline") {
		t.Fatalf("transport failure should render an offline note: %q", out)
	}
	if strings.Contains(out, "invalid") {
		t.Fatalf("a network failure must not be reported as an invalid token: %q", out)
	}
}

func TestAuthStatus_InvalidTokenNote(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	seedProfile(t, "work", "xoxp-work")

	orig := liveIdentity
	t.Cleanup(func() { liveIdentity = orig })
	liveIdentity = func(token string) (*authIdentity, error) {
		return nil, &api.APIError{Method: "auth.test", SlackError: "invalid_auth"}
	}

	out := runStatus(t, "status")
	if !strings.Contains(out, "invalid") || !strings.Contains(out, "invalid_auth") {
		t.Fatalf("a rejected token should render an invalid-token note: %q", out)
	}
	if strings.Contains(out, "offline") {
		t.Fatalf("a rejected token must not be reported as offline: %q", out)
	}
}

func TestAuthStatus_UndecryptableTokenSkipsNetwork(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	// A config whose token is ciphertext but with no key file present: Load
	// leaves it encrypted, so status cannot use it and must skip the network.
	cfgData := `active = "work"
key_backend = "file"

[profiles.work]
token = "enc:v1:unreadableciphertext"
`
	if err := os.WriteFile(cfgPath, []byte(cfgData), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	orig := liveIdentity
	t.Cleanup(func() { liveIdentity = orig })
	calls := 0
	liveIdentity = func(token string) (*authIdentity, error) {
		calls++
		return &authIdentity{Team: "LIVE"}, nil
	}

	out := runStatus(t, "status")
	if calls != 0 {
		t.Fatalf("an undecryptable token must not reach the network, got %d calls", calls)
	}
	if !strings.Contains(out, "encrypted") {
		t.Fatalf("an undecryptable token should render an encrypted scope: %q", out)
	}
}

func TestAuthStatus_SortedByName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	seedProfile(t, "zebra", "xoxp-z")
	seedProfile(t, "alpha", "xoxp-a")
	seedProfile(t, "mango", "xoxp-m")

	out := runStatus(t, "status", "--offline")
	ia, im, iz := strings.Index(out, "alpha"), strings.Index(out, "mango"), strings.Index(out, "zebra")
	if !(ia >= 0 && ia < im && im < iz) {
		t.Fatalf("profiles must be listed in sorted (config.toml) order: %q", out)
	}
}

func TestAuthStatus_EncryptionLineFirst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	seedProfile(t, "work", "xoxp-work")

	out := runStatus(t, "status", "--offline")
	if ie, iw := strings.Index(out, "encryption:"), strings.Index(out, "work"); !(ie >= 0 && ie < iw) {
		t.Fatalf("encryption status should print before the profile list: %q", out)
	}
}

func TestAuthStatus_HintsAllWhenProfilesUnchecked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	seedProfile(t, "work", "xoxp-work")
	seedProfile(t, "personal", "xoxp-pers")

	orig := liveIdentity
	t.Cleanup(func() { liveIdentity = orig })
	liveIdentity = func(token string) (*authIdentity, error) {
		return &authIdentity{Team: "LIVE"}, nil
	}

	out := runStatus(t, "status")
	if !strings.Contains(out, "--all") {
		t.Fatalf("with unchecked profiles, status should hint --all: %q", out)
	}
	// Blank line above, and the hint is not indented (it is not a list item).
	if !strings.Contains(out, "\n\nRun with --all") {
		t.Fatalf("the --all hint should be a blank-line-separated, unindented line: %q", out)
	}
}

func TestAuthStatus_NoHintWhenNothingUnchecked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	seedProfile(t, "work", "xoxp-work")     // single profile
	seedProfile(t, "personal", "xoxp-pers") // second, for the --all case

	orig := liveIdentity
	t.Cleanup(func() { liveIdentity = orig })
	liveIdentity = func(token string) (*authIdentity, error) {
		return &authIdentity{Team: "LIVE"}, nil
	}

	if out := runStatus(t, "status", "--all"); strings.Contains(out, "--all") {
		t.Fatalf("--all verifies everything; no hint expected: %q", out)
	}
	if out := runStatus(t, "status", "--offline"); strings.Contains(out, "--all") {
		t.Fatalf("--offline opted out of checks; no hint expected: %q", out)
	}
}

func TestAuthStatus_AllRunsConcurrently(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	seedProfile(t, "p1", "xoxp-1")
	seedProfile(t, "p2", "xoxp-2")
	seedProfile(t, "p3", "xoxp-3")

	orig := liveIdentity
	t.Cleanup(func() { liveIdentity = orig })
	var inFlight, maxInFlight int32
	var mu sync.Mutex
	liveIdentity = func(token string) (*authIdentity, error) {
		cur := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if cur > maxInFlight {
			maxInFlight = cur
		}
		mu.Unlock()
		time.Sleep(50 * time.Millisecond) // hold so concurrent calls overlap
		atomic.AddInt32(&inFlight, -1)
		return &authIdentity{Team: "LIVE"}, nil
	}

	runStatus(t, "status", "--all")
	if maxInFlight < 2 {
		t.Fatalf("--all should verify profiles concurrently; max in flight = %d", maxInFlight)
	}
}

// B4 Step-1: scope labels appear in text output.
func TestAuthStatus_ScopeLabelOffline(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	cfg := &auth.Config{Active: "work", Profiles: map[string]auth.Profile{
		"work": {Token: "xoxb-abc"}, "me": {Token: "xoxp-def"},
	}}
	if err := auth.Save(filepath.Join(dir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"auth", "status", "--offline"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "[bot]") || !strings.Contains(s, "[user]") {
		t.Fatalf("missing scope labels in:\n%s", s)
	}
}

// B4 Step-1: --format json emits correct shape.
func TestAuthStatus_JSONOffline(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")
	cfg := &auth.Config{Active: "work", Profiles: map[string]auth.Profile{"work": {Token: "xoxb-abc"}}}
	if err := auth.Save(filepath.Join(dir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"auth", "status", "--offline", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got struct {
		Active   string `json:"active"`
		Profiles []struct {
			Name    string `json:"name"`
			Scope   string `json:"scope"`
			Checked bool   `json:"checked"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if got.Active != "work" || len(got.Profiles) != 1 || got.Profiles[0].Scope != "bot" || got.Profiles[0].Checked {
		t.Fatalf("unexpected json: %+v", got)
	}
}

func TestAuthLogin_NonInteractiveFlagRegistered(t *testing.T) {
	cmd := newAuthCommand(&GlobalFlags{})
	login, _, err := cmd.Find([]string{"login"})
	if err != nil {
		t.Fatalf("find login: %v", err)
	}
	if login.Flags().Lookup("non-interactive") == nil {
		t.Fatal("--non-interactive flag not registered on login")
	}
}

func TestAuthLogin_InteractivePromptsAndMints(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	origTerm, origSecret := isTerminal, readSecret
	origWait, origExch := waitForCode, exchangeCode
	t.Cleanup(func() {
		isTerminal, readSecret = origTerm, origSecret
		waitForCode, exchangeCode = origWait, origExch
	})
	isTerminal = func(int) bool { return true }
	readSecret = func(int) ([]byte, error) { return []byte("csecret"), nil } // client secret prompt
	waitForCode = func(addr, path string) (string, error) { return "fakecode", nil }
	var gotID, gotSecret string
	exchangeCode = func(base, id, secret, code, redirect string) (auth.TokenPair, error) {
		gotID, gotSecret = id, secret
		return auth.TokenPair{UserToken: "xoxp-minted"}, nil
	}

	login := newAuthCommand(&GlobalFlags{})
	login.SetIn(strings.NewReader("work\nCID123\n")) // profile, client-id
	var out, errb bytes.Buffer
	login.SetOut(&out)
	login.SetErr(&errb)
	login.SetArgs([]string{"login"})
	if err := login.Execute(); err != nil {
		t.Fatalf("interactive login: %v", err)
	}

	cfg, err := auth.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	p := cfg.Profiles["work"]
	if p.Token != "xoxp-minted" {
		t.Fatalf("minted user token not stored: %+v", p)
	}
	if gotID != "CID123" || gotSecret != "csecret" {
		t.Fatalf("exchange got id=%q secret=%q (want prompted values)", gotID, gotSecret)
	}
}

func TestAuthLogin_NonInteractiveDefaultsProfileAndHint(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	origWait, origExch := waitForCode, exchangeCode
	t.Cleanup(func() { waitForCode, exchangeCode = origWait, origExch })
	waitForCode = func(addr, path string) (string, error) { return "fakecode", nil }
	exchangeCode = func(base, id, secret, code, redirect string) (auth.TokenPair, error) {
		return auth.TokenPair{UserToken: "xoxp-flagminted"}, nil
	}

	login := newAuthCommand(&GlobalFlags{})
	var out, errb bytes.Buffer
	login.SetOut(&out)
	login.SetErr(&errb)
	login.SetArgs([]string{"login", "--non-interactive", "--client-id", "CID", "--client-secret", "SEC"})
	if err := login.Execute(); err != nil {
		t.Fatalf("non-interactive login: %v", err)
	}

	cfg, err := auth.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.Profiles["default"].Token != "xoxp-flagminted" {
		t.Fatalf("default profile not saved: %+v", cfg.Profiles)
	}
	if combined := out.String() + errb.String(); !strings.Contains(combined, "--profile") {
		t.Fatalf("expected a hint mentioning --profile; out=%q err=%q", out.String(), errb.String())
	}
}

func TestAuthLogin_NonInteractiveMissingCredsErrors(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	login := newAuthCommand(&GlobalFlags{})
	login.SetArgs([]string{"login", "--non-interactive"})
	if err := login.Execute(); err == nil {
		t.Fatal("expected error when client-id/secret missing in non-interactive login")
	}
}

func TestAuthLogin_DoesNotStoreClientCredentials(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	origWait, origExch := waitForCode, exchangeCode
	t.Cleanup(func() { waitForCode, exchangeCode = origWait, origExch })
	waitForCode = func(addr, path string) (string, error) { return "fakecode", nil }
	exchangeCode = func(base, id, secret, code, redirect string) (auth.TokenPair, error) {
		return auth.TokenPair{UserToken: "xoxp-minted"}, nil
	}

	login := newAuthCommand(&GlobalFlags{})
	login.SetArgs([]string{"login", "--non-interactive", "--client-id", "CID9", "--client-secret", "SEC9"})
	if err := login.Execute(); err != nil {
		t.Fatalf("login: %v", err)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	s := string(raw)
	if strings.Contains(s, "client_id") || strings.Contains(s, "client_secret") {
		t.Fatalf("login must not persist client credentials; config:\n%s", s)
	}
	if strings.Contains(s, "CID9") || strings.Contains(s, "SEC9") {
		t.Fatalf("client credentials leaked into config:\n%s", s)
	}
}

// B5 Step-1: --as bot stores the bot token.
func TestLogin_BotMintStoresBotToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	origWait, origExch := waitForCode, exchangeCode
	defer func() { waitForCode, exchangeCode = origWait, origExch }()
	waitForCode = func(addr, path string) (string, error) { return "code123", nil }
	exchangeCode = func(base, id, secret, code, redirect string) (auth.TokenPair, error) {
		return auth.TokenPair{UserToken: "xoxp-u", BotToken: "xoxb-b"}, nil
	}

	root := NewRootCommand("test")
	root.SetArgs([]string{"auth", "login", "--as", "bot",
		"--client-id", "x", "--client-secret", "y",
		"--scopes", "chat:write", "--non-interactive", "--profile", "work"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	cfg, _ := auth.Load(filepath.Join(dir, "config.toml"))
	if got := cfg.Profiles["work"].Token; got != "xoxb-b" {
		t.Fatalf("stored token = %q, want xoxb-b", got)
	}
}

// C3: default scopes come from the runtime union, not a static literal.
func TestLogin_DefaultScopesFromUnion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	origWait, origExch := waitForCode, exchangeCode
	defer func() { waitForCode, exchangeCode = origWait, origExch }()
	waitForCode = func(addr, path string) (string, error) { return "c", nil }
	exchangeCode = func(base, id, secret, code, redirect string) (auth.TokenPair, error) {
		return auth.TokenPair{UserToken: "xoxp-u"}, nil
	}

	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	// No --scopes: the authorize URL must carry the generated user union.
	root.SetArgs([]string{"auth", "login",
		"--client-id", "x", "--client-secret", "y", "--non-interactive"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	// users:read.email is only present if the default came from the union, not
	// the removed legacy literal.
	if !strings.Contains(out.String(), "users%3Aread.email") {
		t.Fatalf("authorize URL missing generated scope; got:\n%s", out.String())
	}
}

// B5 Step-1: default (no --as) stores the user token.
func TestLogin_UserMintStoresUserToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	origWait, origExch := waitForCode, exchangeCode
	defer func() { waitForCode, exchangeCode = origWait, origExch }()
	waitForCode = func(addr, path string) (string, error) { return "code123", nil }
	exchangeCode = func(base, id, secret, code, redirect string) (auth.TokenPair, error) {
		return auth.TokenPair{UserToken: "xoxp-u", BotToken: "xoxb-b"}, nil
	}

	root := NewRootCommand("test")
	root.SetArgs([]string{"auth", "login",
		"--client-id", "x", "--client-secret", "y",
		"--scopes", "chat:write", "--non-interactive", "--profile", "me"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	cfg, _ := auth.Load(filepath.Join(dir, "config.toml"))
	if got := cfg.Profiles["me"].Token; got != "xoxp-u" {
		t.Fatalf("stored token = %q, want xoxp-u", got)
	}
}
