# slk Auth 解耦 + Bot Flow + Scope 產生 實作計畫

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 slk profile 解耦成「一 profile 一 token」、scope 由前綴推導不儲存、`auth login` 能鑄 bot token,並讓 user/bot scope 由命令樹動態產生。

**Architecture:** `Profile` 改存單一 `Token`;`TokenScope` 從前綴推導 scope;`--as` 從「選 token」改為「斷言/鑄造意圖」;每個命令以 `userScopes`/`botScopes`/`botCapable` 註記為 SSOT,`auth login` 預設與 README manifest 都由命令樹 union 產生,並由 CI 守門。

**Tech Stack:** Go、cobra、BurntSushi/toml、AES-256-GCM(既有 crypto)、`httptest` 單元測試。

**設計依據:** `docs/superpowers/specs/2026-05-24-slk-auth-redesign-design.md`;scope 原始資料來自 `docs/superpowers/specs/2026-05-24-slk-bot-scope-design.md` §9。

**約定:**
- prose 用繁中;程式碼、檔案路徑、指令、測試名稱、commit 訊息一律英文。
- 每個 commit 訊息結尾加一行:`Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`(下方步驟省略不重複)。
- 測試預設快取,跑前若要乾淨結果加 `go clean -testcache`。
- push / 開 PR / merge 不在本計畫內,需使用者另外明確指示。

---

## Phase A — Auth 核心(資料模型 / scope / 解析)

### Task A1: `TokenScope` 前綴推導 helper

**Files:**
- Create: `internal/auth/scope.go`
- Test: `internal/auth/scope_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import "testing"

func TestTokenScope(t *testing.T) {
	cases := map[string]string{
		"xoxp-abc":   "user",
		"xoxb-abc":   "bot",
		"xoxc-abc":   "",
		"xapp-abc":   "",
		"":           "",
		"enc:v1:zzz": "",
	}
	for tok, want := range cases {
		if got := TokenScope(tok); got != want {
			t.Errorf("TokenScope(%q) = %q, want %q", tok, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestTokenScope -v`
Expected: FAIL（`undefined: TokenScope`,編譯不過）

- [ ] **Step 3: Write minimal implementation**

```go
package auth

import "strings"

// TokenScope derives the Slack identity scope from a token's prefix: "user" for
// xoxp-, "bot" for xoxb-, "" for any other prefix (slk supports only these two).
// The token must be decrypted plaintext; a still-encrypted value (IsEncrypted)
// has no readable prefix and yields "".
func TokenScope(token string) string {
	switch {
	case strings.HasPrefix(token, "xoxp-"):
		return "user"
	case strings.HasPrefix(token, "xoxb-"):
		return "bot"
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth/ -run TestTokenScope -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/auth/scope.go internal/auth/scope_test.go
git commit -m "feat(auth): add TokenScope prefix-to-scope helper"
```

---

### Task A2: `Profile` 改單一 `Token`,加解密改單欄位

**Files:**
- Modify: `internal/auth/store.go`（`Profile` 結構、`decryptInPlace`、`encryptProfile`）
- Test: `internal/auth/store_test.go`

- [ ] **Step 1: Update the round-trip test to the single-token model**

把 `internal/auth/store_test.go` 中所有 `Profile{UserToken: ..., BotToken: ...}` 改成 `Profile{Token: ...}`。新增/替換一個明確的單欄位 round-trip 測試:

```go
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
	// On disk the token must be encrypted, not plaintext.
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
```

確認 test 檔 import 了 `os`、`strings`、`path/filepath`。

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestSaveLoad_SingleTokenRoundTrip -v`
Expected: FAIL（編譯不過:`Profile` 仍有 `UserToken`/`BotToken`,且無 `Token`）

- [ ] **Step 3: Edit `store.go` — struct 與加解密改單欄位**

把 `Profile` 改為:

```go
// Profile holds the credential for one Slack identity. The token is stored
// encrypted at rest (see crypto.go); it is plaintext in memory after Load and
// re-encrypted by Save. Scope (user/bot) is derived from the prefix (TokenScope),
// never stored. slk does not persist the OAuth client id/secret.
type Profile struct {
	Token string `toml:"token,omitempty"`
}
```

把 `decryptInPlace` 的 per-profile 迴圈本體改為單欄位:

```go
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
```

把 `encryptProfile` 改為:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth/ -run 'TestSaveLoad|TestTokenScope' -v`
Expected: PASS（auth 套件其餘 `UserToken`/`BotToken` 參照會在 A3 一起修;此步若其他 test 檔仍編譯不過,先只跑本 run 名稱會因套件編譯失敗而 fail——故同時在本步把 `precedence_test.go`、`token_test.go`、`crypto_test.go` 內殘存的 `UserToken`/`BotToken` 字面改成 `Token`，使套件可編譯。其中 precedence 的語意改寫留待 A3。）

- [ ] **Step 5: Commit**

```bash
git add internal/auth/store.go internal/auth/store_test.go
git commit -m "refactor(auth): store a single token per profile"
```

---

### Task A3: `ResolveToken` 改成斷言語意

**Files:**
- Modify: `internal/auth/token.go`
- Test: `internal/auth/precedence_test.go`（改寫）

- [ ] **Step 1: Rewrite the precedence test for single-token + assertion**

把 `precedence_test.go` 的 `TestResolveToken_PrecedenceMatrix` 改寫為:

```go
func TestResolveToken_PrecedenceAndAssertion(t *testing.T) {
	t.Setenv(keyEnvVar, backendFile)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	cfg := &Config{
		Active: "work",
		Profiles: map[string]Profile{
			"work":     {Token: "xoxb-work-bot"},
			"personal": {Token: "xoxp-personal-user"},
		},
	}
	if err := Save(cfgPath, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	for _, tc := range []struct {
		name        string
		profileName string
		assertScope string
		envToken    string
		wantToken   string
		wantErr     bool
	}{
		{"active profile, no assertion", "", "", "", "xoxb-work-bot", false},
		{"active profile, matching assertion", "", "bot", "", "xoxb-work-bot", false},
		{"active profile, mismatched assertion", "", "user", "", "", true},
		{"explicit profile overrides active", "personal", "", "", "xoxp-personal-user", false},
		{"explicit profile, matching assertion", "personal", "user", "", "xoxp-personal-user", false},
		{"env token wins over everything", "personal", "", "xoxp-env", "xoxp-env", false},
		{"env token still asserted", "personal", "bot", "xoxp-env", "", true},
		{"missing profile is an error", "ghost", "", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveToken(loaded, tc.profileName, tc.assertScope, tc.envToken)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if got != tc.wantToken {
				t.Errorf("token = %q, want %q", got, tc.wantToken)
			}
		})
	}

	// AuthError type preserved for the mismatch case (exit 3, not 1).
	_, err = ResolveToken(loaded, "", "user", "")
	if _, ok := err.(*AuthError); !ok {
		t.Errorf("expected *AuthError for assertion mismatch, got %T", err)
	}
}
```

`TestResolveToken_UndecryptableTokenErrors` 改成單欄位:`Profiles{"work": {Token: encPrefix + "..."}}`,呼叫 `ResolveToken(cfg, "", "user", "")` 與 env 覆寫照舊。

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestResolveToken -v`
Expected: FAIL（`ResolveToken` 仍是舊的 identity-selector 簽名/語意)

- [ ] **Step 3: Rewrite `token.go`**

```go
package auth

import "fmt"

// ResolveToken picks the active token. Precedence: envToken, then the named
// profile (or cfg.Active if profileName is empty). assertScope, when non-empty
// ("user"|"bot"), requires the resolved token's derived scope to match; a known
// mismatch is an *AuthError. An unknown prefix does not fail the assertion.
func ResolveToken(cfg *Config, profileName, assertScope, envToken string) (string, error) {
	if envToken != "" {
		return assertOK(envToken, assertScope, "SLK_TOKEN")
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
	if p.Token == "" {
		return "", &AuthError{Reason: fmt.Sprintf("profile %q has no token", name)}
	}
	// A still-encrypted value means Load could not decrypt it (key unavailable).
	if isEncrypted(p.Token) {
		return "", &AuthError{Reason: fmt.Sprintf("profile %q token could not be decrypted (encryption key unavailable)", name)}
	}
	return assertOK(p.Token, assertScope, fmt.Sprintf("profile %q", name))
}

// assertOK returns tok unless assertScope is set and the token's known scope
// differs from it.
func assertOK(tok, assertScope, where string) (string, error) {
	if assertScope != "" {
		if s := TokenScope(tok); s != "" && s != assertScope {
			return "", &AuthError{Reason: fmt.Sprintf("%s holds a %s token but --as %s was requested", where, s, assertScope)}
		}
	}
	return tok, nil
}
```

- [ ] **Step 4: Run the whole auth package**

Run: `go clean -testcache && go test ./internal/auth/ -v`
Expected: PASS（整個 auth 套件綠燈）

- [ ] **Step 5: Commit**

```bash
git add internal/auth/token.go internal/auth/precedence_test.go
git commit -m "refactor(auth): make ResolveToken assert scope instead of selecting token"
```

---

## Phase B — 命令面接線

### Task B1: `--as` 預設改空字串 + 說明更新

**Files:**
- Modify: `internal/commands/context.go:19`

- [ ] **Step 1: Edit the flag registration**

把該行改為:

```go
	pf.StringVar(&g.Identity, "as", "", "identity user|bot: on a command, assert the active token's scope; on `auth login`, which token to mint (default user)")
```

- [ ] **Step 2: Build to confirm it compiles**

Run: `go build ./cmd/slk`
Expected: 成功（行為改動的測試在 B3–B5）

- [ ] **Step 3: Commit**

```bash
git add internal/commands/context.go
git commit -m "refactor(commands): --as defaults to empty (assertion is opt-in)"
```

---

### Task B2: `buildClient(cmd, g)` + botCapable guardrail + 全站呼叫點更新

**Files:**
- Modify: `internal/commands/clientutil.go`
- Modify: 所有呼叫 `buildClient(g)` 的命令檔(機械式)
- Test: `internal/commands/clientutil_test.go`

- [ ] **Step 1: Write the guardrail test**

Create `internal/commands/clientutil_test.go`:

```go
package commands

import (
	"path/filepath"
	"testing"

	"github.com/howar31/slk/internal/auth"
	"github.com/spf13/cobra"
)

func TestBuildClient_BotGuardrail(t *testing.T) {
	t.Setenv("SLK_CONFIG", filepath.Join(t.TempDir(), "config.toml")) // hermetic; no real config
	t.Setenv("SLK_TOKEN", "xoxb-bot-token")                           // bot token via env
	cmd := &cobra.Command{Use: "messages", Annotations: map[string]string{"botCapable": "false"}}
	_, err := buildClient(cmd, &GlobalFlags{})
	if err == nil {
		t.Fatal("expected guardrail error for user-only verb under a bot token")
	}
	if _, ok := err.(*auth.AuthError); !ok {
		t.Fatalf("expected *auth.AuthError (exit 3), got %T", err)
	}
}

func TestBuildClient_BotAllowedWhenCapable(t *testing.T) {
	t.Setenv("SLK_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("SLK_TOKEN", "xoxb-bot-token")
	cmd := &cobra.Command{Use: "send", Annotations: map[string]string{"botCapable": "true"}}
	if _, err := buildClient(cmd, &GlobalFlags{}); err != nil {
		t.Fatalf("unexpected error for bot-capable verb: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestBuildClient -v`
Expected: FAIL（`buildClient` 仍是單參數 `buildClient(g)`,編譯不過)

- [ ] **Step 3: Rewrite `buildClient` in `clientutil.go`**

```go
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/auth"
	"github.com/spf13/cobra"
)

// profileName returns the effective profile: the --profile flag if set,
// otherwise the SLK_PROFILE env var.
func profileName(g *GlobalFlags) string {
	if g.Profile != "" {
		return g.Profile
	}
	return os.Getenv("SLK_PROFILE")
}

// buildClient resolves the active token (asserting --as scope) and returns a
// ready API client. It refuses a user-only verb (botCapable=false) when the
// resolved token is a bot token, failing fast with an *auth.AuthError (exit 3)
// instead of surfacing Slack's not_allowed_token_type.
func buildClient(cmd *cobra.Command, g *GlobalFlags) (*api.Client, error) {
	path, err := auth.ConfigPath()
	if err != nil {
		return nil, err
	}
	cfg, err := auth.Load(path)
	if err != nil {
		return nil, err
	}
	token, err := auth.ResolveToken(cfg, profileName(g), g.Identity, os.Getenv("SLK_TOKEN"))
	if err != nil {
		return nil, err
	}
	if auth.TokenScope(token) == "bot" && cmd.Annotations["botCapable"] == "false" {
		return nil, &auth.AuthError{Reason: fmt.Sprintf("%q is user-token-only; the active profile holds a bot token", cmd.CommandPath())}
	}
	return api.New(token), nil
}

// cacheDir returns ~/.config/slk/cache.
func cacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	return filepath.Join(home, ".config", "slk", "cache")
}
```

- [ ] **Step 4: Update every call site `buildClient(g)` → `buildClient(cmd, g)`**

每個呼叫都發生在某個命令的 `RunE: func(cmd *cobra.Command, args []string)` 內,`cmd` 必在 scope。找出全部:

Run: `grep -rn 'buildClient(g)' internal/commands/`

逐一把 `buildClient(g)` 改為 `buildClient(cmd, g)`。這是純機械替換,編譯器會抓漏。（執行時可派一個 subagent 做此替換;指令明確、無判斷。）

- [ ] **Step 5: Build + run tests**

Run: `go build ./cmd/slk && go clean -testcache && go test ./internal/commands/ -run TestBuildClient -v`
Expected: build 成功;兩個 guardrail 測試 PASS

- [ ] **Step 6: Commit**

```bash
git add internal/commands/clientutil.go internal/commands/clientutil_test.go internal/commands/
git commit -m "feat(auth): refuse user-only verbs under a bot token (exit 3)"
```

---

### Task B3: `set-token` 改單一 `--token` + 前綴驗證

**Files:**
- Modify: `internal/commands/auth.go`（`newAuthSetTokenCommand`）
- Test: `internal/commands/auth_test.go`

- [ ] **Step 1: Write/replace the set-token tests**

在 `auth_test.go` 把任何 `--user`/`--bot` 的 set-token 測試改寫,新增:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/commands/ -run TestSetToken -v`
Expected: FAIL（`--token` flag 不存在)

- [ ] **Step 3: Rewrite `newAuthSetTokenCommand`**

```go
func newAuthSetTokenCommand() *cobra.Command {
	var profile, token string
	var nonInteractive bool
	cmd := &cobra.Command{
		Use:   "set-token",
		Short: "Store a token for a profile",
		Long: "Store a single token (user xoxp- or bot xoxb-) for a profile. Pass --token for " +
			"scripts/agents (use --token - to read from stdin), or run in a terminal to be " +
			"prompted (token entry is hidden). A profile holds exactly one token; its scope is " +
			"derived from the prefix.",
		RunE: func(cmd *cobra.Command, args []string) error {
			pc := newPromptCtx(cmd, nonInteractive)
			flags := cmd.Flags()

			name, err := pc.line(profile, flags.Changed("profile"), "Profile name [default]: ")
			if err != nil {
				return err
			}
			if name == "" {
				name = "default"
			}

			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			p := cfg.Profiles[name]

			tok, err := pc.secret(token, flags.Changed("token"), true, "Paste token (xoxp- or xoxb-, hidden): ")
			if err != nil {
				return err
			}
			if tok == "" {
				return fmt.Errorf("no token; pass --token <token> (use - to read stdin), or run in a terminal")
			}
			if auth.TokenScope(tok) == "" {
				return fmt.Errorf("unsupported token prefix; slk accepts user (xoxp-) or bot (xoxb-) tokens only")
			}
			p.Token = tok

			cfg.Profiles[name] = p
			if cfg.Active == "" {
				cfg.Active = name
			}
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved profile %q\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "default", "profile name")
	cmd.Flags().StringVar(&token, "token", "", "token (xoxp- or xoxb-, or - to read stdin)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "never prompt; require values via flags")
	return cmd
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/commands/ -run TestSetToken -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/commands/auth.go internal/commands/auth_test.go
git commit -m "feat(auth): set-token takes a single --token with prefix validation"
```

---

### Task B4: `auth status` `[scope]` 標籤 + 結構化 live-check + `--format json`

**Files:**
- Modify: `internal/commands/auth.go`（`newAuthCommand`、`newAuthStatusCommand`、live-check seam）
- Test: `internal/commands/auth_test.go`

- [ ] **Step 1: Write the tests (text label + json shape)**

```go
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
```

確認 import:`bytes`、`encoding/json`、`strings`、`path/filepath`。

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/commands/ -run TestAuthStatus -v`
Expected: FAIL（status 不接 `--format`;無 `[scope]` 標籤;非 json)

- [ ] **Step 3: Refactor status to a structured model**

在 `auth.go` 加結構與 helper:

```go
// authIdentity is the parsed auth.test identity for one profile.
type authIdentity struct {
	Team   string `json:"team"`
	TeamID string `json:"team_id"`
	User   string `json:"user"`
	UserID string `json:"user_id"`
	URL    string `json:"url"`
}

// profileStatus is the resolved status of one profile for `auth status`.
type profileStatus struct {
	Name     string        `json:"name"`
	Active   bool          `json:"active"`
	Scope    string        `json:"scope"` // user|bot|unknown|encrypted|none
	Checked  bool          `json:"checked"`
	Identity *authIdentity `json:"identity,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// profileScope derives the displayed scope of a profile's token.
func profileScope(p auth.Profile) string {
	if p.Token == "" {
		return "none"
	}
	if auth.IsEncrypted(p.Token) {
		return "encrypted"
	}
	if s := auth.TokenScope(p.Token); s != "" {
		return s
	}
	return "unknown"
}

// parseAuthIdentity parses an auth.test response into an authIdentity.
func parseAuthIdentity(raw []byte) (*authIdentity, error) {
	var id authIdentity
	if err := json.Unmarshal(raw, &id); err != nil {
		return nil, err
	}
	return &id, nil
}
```

把 live-check seam 改成回傳結構（取代舊的 `liveIdentity func(string)(string,error)`）:

```go
// liveIdentity calls auth.test for token and returns the parsed identity. Seam:
// overridable in tests so status runs without a live token or network.
var liveIdentity = func(token string) (*authIdentity, error) {
	c := api.New(token)
	c.HTTP.Timeout = 4 * time.Second
	raw, err := c.Call("auth.test", nil, nil)
	if err != nil {
		return nil, err
	}
	return parseAuthIdentity(raw)
}

// resolveProfileStatus builds the status for one profile. When check is false no
// network call is made (Checked stays false). A Slack rejection reads as an
// invalid token; any other error reads as offline.
func resolveProfileStatus(name string, p auth.Profile, active, check bool) profileStatus {
	st := profileStatus{Name: name, Active: active, Scope: profileScope(p)}
	if !check || st.Scope == "none" || st.Scope == "encrypted" {
		return st
	}
	st.Checked = true
	id, err := liveIdentity(p.Token)
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) {
			st.Error = fmt.Sprintf("invalid token: %s", apiErr.SlackError)
		} else {
			st.Error = "offline"
		}
		return st
	}
	st.Identity = id
	return st
}
```

移除舊的 `identitySuffix` 與舊字串版 `liveIdentity`（`formatAuthIdentity` 仍保留給 `auth test`）。

改寫 `newAuthStatusCommand` 接 `g *GlobalFlags`,並在 `newAuthCommand` 改成 `newAuthStatusCommand(g)`:

```go
func newAuthStatusCommand(g *GlobalFlags) *cobra.Command {
	var all, offline bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show configured profiles",
		Long: "Show configured profiles with a derived [user]/[bot] scope label. By default the " +
			"active profile is verified live (auth.test); --all verifies every profile, --offline " +
			"skips the network. --format json emits a structured object.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(cfg.Profiles) == 0 {
				if g.Format == "json" {
					fmt.Fprintln(out, `{"profiles":[]}`)
				} else {
					fmt.Fprintln(out, "no profiles configured")
				}
				return nil
			}

			names := make([]string, 0, len(cfg.Profiles))
			width := 0
			for name := range cfg.Profiles {
				names = append(names, name)
				if len(name) > width {
					width = len(name)
				}
			}
			sort.Strings(names)

			statuses := make(map[string]profileStatus, len(names))
			var mu sync.Mutex
			var wg sync.WaitGroup
			for _, name := range names {
				check := !offline && (all || name == cfg.Active)
				wg.Add(1)
				go func(name string, p auth.Profile) {
					defer wg.Done()
					s := resolveProfileStatus(name, p, name == cfg.Active, check)
					mu.Lock()
					statuses[name] = s
					mu.Unlock()
				}(name, cfg.Profiles[name])
			}
			wg.Wait()

			if g.Format == "json" {
				ordered := make([]profileStatus, 0, len(names))
				for _, name := range names {
					ordered = append(ordered, statuses[name])
				}
				payload := struct {
					Encryption string          `json:"encryption"`
					Active     string          `json:"active"`
					Profiles   []profileStatus `json:"profiles"`
				}{auth.EncryptionStatus(cfg), cfg.Active, ordered}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}

			fmt.Fprintln(out, auth.EncryptionStatus(cfg))
			fmt.Fprintln(out)
			for _, name := range names {
				st := statuses[name]
				marker := " "
				if st.Active {
					marker = "*"
				}
				line := fmt.Sprintf("%s %-*s [%s]", marker, width, name, st.Scope)
				switch {
				case st.Identity != nil:
					line += fmt.Sprintf(" — %s (%s) — %s (%s) @ %s",
						st.Identity.Team, st.Identity.TeamID, st.Identity.User, st.Identity.UserID, st.Identity.URL)
				case st.Error != "":
					line += " — (" + st.Error + ")"
				}
				fmt.Fprintln(out, line)
			}
			if !offline && !all && len(names) > 1 {
				fmt.Fprintln(out)
				fmt.Fprintln(out, "Run with --all to verify every profile, not just the active one.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "verify every profile live, not just the active one")
	cmd.Flags().BoolVar(&offline, "offline", false, "skip the live Slack check; list local info only")
	return cmd
}
```

> 注意:JSON 分支以外的 format(`jsonl`/`table`)落到文字分支(concise)。

更新 `auth_test.go` 內任何覆寫 `liveIdentity` 的舊測試,改用新的 `func(token string) (*authIdentity, error)` 簽名。

- [ ] **Step 4: Run tests to verify they pass**

Run: `go clean -testcache && go test ./internal/commands/ -run 'TestAuthStatus|TestAuth' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/commands/auth.go internal/commands/auth_test.go
git commit -m "feat(auth): status shows derived [scope] label and supports --format json"
```

---

### Task B5: `auth login` 鑄 bot token（鑄造選擇 + 授權 URL 參數 + 存單 token）

> 本任務先做「鑄造選擇 + URL 參數 + 存 token」,scope 預設此步暫用既有字面常數;Phase C 的 C3 再把預設換成 runtime union。

**Files:**
- Modify: `internal/commands/auth.go`（`newAuthLoginCommand` 改接 `g`,`newAuthCommand` 改 `newAuthLoginCommand(g)`）
- Test: `internal/commands/auth_test.go`

- [ ] **Step 1: Write the login mint tests**

利用既有的 `waitForCode` / `exchangeCode` seam（`auth.go` 頂部 var）。新增:

```go
func TestLogin_BotMintStoresBotToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("SLK_KEYRING_BACKEND", "file")

	origWait, origExch := waitForCode, exchangeCode
	defer func() { waitForCode, exchangeCode = origWait, origExch }()
	var gotURLScopeKey string
	waitForCode = func(addr, path string) (string, error) { return "code123", nil }
	exchangeCode = func(base, id, secret, code, redirect string) (auth.TokenPair, error) {
		return auth.TokenPair{UserToken: "xoxp-u", BotToken: "xoxb-b"}, nil
	}
	_ = gotURLScopeKey

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/commands/ -run TestLogin -v`
Expected: FAIL（login 仍只存 UserToken/BotToken 雙欄位、不接 `--as` 鑄造)

- [ ] **Step 3: Rewrite `newAuthLoginCommand(g)` core**

把簽名改成 `func newAuthLoginCommand(g *GlobalFlags) *cobra.Command`,並把 `newAuthCommand` 內改成 `newAuthLoginCommand(g)`。授權 URL 與儲存改為:

```go
			mint := g.Identity
			if mint == "" {
				mint = "user"
			}
			if mint != "user" && mint != "bot" {
				return fmt.Errorf("--as must be user or bot")
			}

			redirectURI := "http://localhost:" + port + "/callback"
			q := url.Values{
				"client_id":    {id},
				"redirect_uri": {redirectURI},
			}
			if mint == "bot" {
				q.Set("scope", scopes)
			} else {
				q.Set("user_scope", scopes)
			}
			authURL := "https://slack.com/oauth/v2/authorize?" + q.Encode()
			fmt.Fprintf(cmd.OutOrStdout(), "Open this URL to authorize:\n%s\n", authURL)

			code, err := waitForCode(":"+port, "/callback")
			if err != nil {
				return err
			}
			pair, err := exchangeCode("https://slack.com/api/oauth.v2.access",
				id, secret, code, redirectURI)
			if err != nil {
				return err
			}

			tok := pair.UserToken
			if mint == "bot" {
				tok = pair.BotToken
			}
			if tok == "" {
				return fmt.Errorf("oauth returned no %s token (for --as bot, your app must have a bot user)", mint)
			}

			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			p := cfg.Profiles[name]
			p.Token = tok
			cfg.Profiles[name] = p
			if cfg.Active == "" {
				cfg.Active = name
			}
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "authorized profile %q (%s)\n", name, mint)
			if !pc.interactive && !flags.Changed("profile") {
				fmt.Fprintln(cmd.ErrOrStderr(), "  (saved to the default profile; pass --profile to choose another)")
			}
			return nil
```

`--scopes` flag 暫時保留既有的 36-scope 字面預設（C3 會移除)。

- [ ] **Step 4: Run tests to verify they pass**

Run: `go clean -testcache && go test ./internal/commands/ -run TestLogin -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/commands/auth.go internal/commands/auth_test.go
git commit -m "feat(auth): auth login can mint a bot token via --as bot"
```

---

## Phase C — Scope 動態產生

### Task C1: 為每個命令加上 scope 註記（SSOT)

**Files:**
- Modify: 每個命令群檔(`internal/commands/msg.go`、`channel.go`、`user.go`、`file.go`、`canvas.go`、`list.go`、`pin.go`、`bookmark.go`、`team.go`、`emoji.go`、`dnd.go`、`usergroup.go`、`search.go`、`thread.go`、`auth.go`）

把 `docs/superpowers/specs/2026-05-24-slk-bot-scope-design.md` §9-A 的每一列,轉成對應命令的 `Annotations`。在既有 `slackMethod` 旁加 `userScopes`、`botScopes`、`botCapable`。範例(務必逐列對照 §9-A,勿憑記憶):

```go
// channel join: user channels:write → bot channels:join
Annotations: map[string]string{
	"slackMethod": "conversations.join",
	"userScopes":  "channels:write",
	"botScopes":   "channels:join",
	"botCapable":  "true",
},
// channel create: user channels:write → bot channels:manage
Annotations: map[string]string{
	"slackMethod": "conversations.create",
	"userScopes":  "channels:write,groups:write,im:write,mpim:write",
	"botScopes":   "channels:manage,groups:write,im:write,mpim:write",
	"botCapable":  "true",
},
// search messages: user-only
Annotations: map[string]string{
	"slackMethod": "search.messages",
	"userScopes":  "search:read",
	"botScopes":   "",
	"botCapable":  "false",
},
// user by-email: users:read.email in BOTH sets (gap fix)
Annotations: map[string]string{
	"slackMethod": "users.lookupByEmail",
	"userScopes":  "users:read.email",
	"botScopes":   "users:read.email",
	"botCapable":  "true",
},
// canvas read: files:read,canvases:read (canvases:read kept defensively)
Annotations: map[string]string{
	"slackMethod": "files.info",
	"userScopes":  "files:read,canvases:read",
	"botScopes":   "files:read,canvases:read",
	"botCapable":  "true",
},
```

規則摘要(完整以 §9-A 為準):
- `slackMethod` 為空的本地命令(set-token/switch/logout)不必加 scope 註記。
- `botCapable=false` 的 12 個 user-only verb(§9-B B2):`botScopes` 留空。
- `channels:write`(user)→ `channels:manage`(bot)套用所有公開頻道寫入/管理 verb;`channel join` → `channels:join`;`channel open`/`close` 兩端都 `im:write,mpim:write`。

> 本任務無獨立測試;**完整性由 C2 的 oracle 測試把關**。執行時適合**按命令群拆給多個平行 subagent**,各自只填一群、附上 §9-A 對應列。

- [ ] **Step 1: Apply annotations group by group per §9-A**
- [ ] **Step 2: Build**

Run: `go build ./cmd/slk`
Expected: 成功

- [ ] **Step 3: Commit**

```bash
git add internal/commands/
git commit -m "feat(scopes): annotate commands with user/bot scopes and bot capability"
```

---

### Task C2: scope union + oracle 測試（鎖定 37/35）

**Files:**
- Create: `internal/commands/scopes.go`
- Test: `internal/commands/scopes_test.go`

- [ ] **Step 1: Write the oracle test**

```go
package commands

import (
	"reflect"
	"testing"
)

func TestScopeUnion_Oracle(t *testing.T) {
	root := NewRootCommand("test")

	wantUser := []string{
		"bookmarks:read", "bookmarks:write", "canvases:read", "canvases:write",
		"channels:history", "channels:read", "channels:write", "chat:write",
		"dnd:read", "dnd:write", "emoji:read", "files:read", "files:write",
		"groups:history", "groups:read", "groups:write", "im:history", "im:read",
		"im:write", "lists:read", "lists:write", "mpim:history", "mpim:read",
		"mpim:write", "pins:read", "pins:write", "reactions:read", "reactions:write",
		"search:read", "team:read", "usergroups:read", "usergroups:write",
		"users.profile:read", "users.profile:write", "users:read", "users:read.email",
		"users:write",
	}
	wantBot := []string{
		"bookmarks:read", "bookmarks:write", "canvases:read", "canvases:write",
		"channels:history", "channels:join", "channels:manage", "channels:read",
		"chat:write", "dnd:read", "emoji:read", "files:read", "files:write",
		"groups:history", "groups:read", "groups:write", "im:history", "im:read",
		"im:write", "lists:read", "lists:write", "mpim:history", "mpim:read",
		"mpim:write", "pins:read", "pins:write", "reactions:read", "reactions:write",
		"team:read", "usergroups:read", "usergroups:write", "users.profile:read",
		"users:read", "users:read.email", "users:write",
	}

	if got := scopeUnion(root, "user"); !reflect.DeepEqual(got, wantUser) {
		t.Errorf("user scopes (%d):\n got  %v\n want %v", len(got), got, wantUser)
	}
	if got := scopeUnion(root, "bot"); !reflect.DeepEqual(got, wantBot) {
		t.Errorf("bot scopes (%d):\n got  %v\n want %v", len(got), got, wantBot)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestScopeUnion_Oracle -v`
Expected: FAIL（`undefined: scopeUnion`)

- [ ] **Step 3: Implement `scopeUnion`**

```go
package commands

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// scopeUnion returns the sorted, de-duplicated set of scopes the command tree
// under root requires for identity ("user" or "bot"). It reads each command's
// "userScopes"/"botScopes" annotation (comma-separated; empty contributes
// nothing). This is the single source consumed by `auth login` (runtime) and
// the README manifest generator.
func scopeUnion(root *cobra.Command, identity string) []string {
	key := "userScopes"
	if identity == "bot" {
		key = "botScopes"
	}
	set := map[string]struct{}{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, s := range strings.Split(c.Annotations[key], ",") {
			if s = strings.TrimSpace(s); s != "" {
				set[s] = struct{}{}
			}
		}
		for _, ch := range c.Commands() {
			walk(ch)
		}
	}
	walk(root)
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go clean -testcache && go test ./internal/commands/ -run TestScopeUnion_Oracle -v`
Expected: PASS（若 fail,代表 C1 註記有漏/錯——依失敗 diff 修 C1 的註記,不要改 oracle 數字)

- [ ] **Step 5: Commit**

```bash
git add internal/commands/scopes.go internal/commands/scopes_test.go
git commit -m "feat(scopes): scopeUnion over the command tree with a 37/35 oracle test"
```

---

### Task C3: `auth login` 預設改 runtime union

**Files:**
- Modify: `internal/commands/auth.go`（`newAuthLoginCommand`）
- Test: `internal/commands/auth_test.go`

- [ ] **Step 1: Write the test asserting the default comes from the union**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestLogin_DefaultScopesFromUnion -v`
Expected: FAIL（仍用舊字面預設,無 `users:read.email`)

- [ ] **Step 3: Replace the static `--scopes` default with a runtime union**

把 `--scopes` flag 預設改為空字串:

```go
	cmd.Flags().StringVar(&scopes, "scopes", "", "comma-separated scopes to request (default: generated from supported commands for the chosen identity)")
```

在 RunE 計算 `scopes`(在決定 `mint` 之後、組 URL 之前):

```go
			scopeList := scopes
			if !flags.Changed("scopes") {
				scopeList = strings.Join(scopeUnion(cmd.Root(), mint), ",")
			}
```

並把組 URL 那段改用 `scopeList`(取代 `scopes`):

```go
			if mint == "bot" {
				q.Set("scope", scopeList)
			} else {
				q.Set("user_scope", scopeList)
			}
```

確認 `auth.go` import 了 `strings`。

- [ ] **Step 4: Run tests to verify they pass**

Run: `go clean -testcache && go test ./internal/commands/ -run TestLogin -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/commands/auth.go internal/commands/auth_test.go
git commit -m "feat(auth): auth login default scopes are generated from the command tree"
```

---

### Task C4: `generate-manifest` 命令重寫 README manifest + CI 守門

**Files:**
- Modify: `README.md`（manifest 區塊加標記、加入 `bot:` 陣列)
- Create: `internal/commands/generatemanifest.go`
- Modify: `internal/commands/root.go`（註冊新命令)
- Test: `internal/commands/generatemanifest_test.go`
- Modify: `.github/workflows/*.yml`(加 staleness 守門 job)

- [ ] **Step 1: Add markers to README around the manifest block**

在 `README.md` 的 manifest fenced block 前後各加一行 HTML 註解(包住 ```json … ``` 整塊):

```
<!-- BEGIN GENERATED MANIFEST -->
```

…(原 json 區塊)…

```
<!-- END GENERATED MANIFEST -->
```

- [ ] **Step 2: Write the test**

```go
package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateManifest_WritesUserAndBot(t *testing.T) {
	dir := t.TempDir()
	readme := filepath.Join(dir, "README.md")
	content := "intro\n<!-- BEGIN GENERATED MANIFEST -->\nOLD\n<!-- END GENERATED MANIFEST -->\nend\n"
	if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand("test")
	root.SetArgs([]string{"generate-manifest", "--readme", readme})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got, _ := os.ReadFile(readme)
	s := string(got)
	if !strings.Contains(s, `"user"`) || !strings.Contains(s, `"bot"`) {
		t.Fatalf("manifest missing user/bot arrays:\n%s", s)
	}
	if !strings.Contains(s, "channels:manage") || !strings.Contains(s, "users:read.email") {
		t.Fatalf("manifest missing generated scopes:\n%s", s)
	}
	if !strings.HasPrefix(s, "intro\n") || !strings.HasSuffix(s, "end\n") {
		t.Fatalf("content outside markers not preserved:\n%s", s)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestGenerateManifest -v`
Expected: FAIL（命令不存在)

- [ ] **Step 4: Implement `generatemanifest.go`**

```go
package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	manifestBegin = "<!-- BEGIN GENERATED MANIFEST -->"
	manifestEnd   = "<!-- END GENERATED MANIFEST -->"
)

// newGenerateManifestCommand builds the hidden `generate-manifest` command,
// which rewrites the README app-manifest region (between the BEGIN/END markers)
// from the command tree's scope annotations. Kept separate from
// generate-skills so each generated artifact has its own clearly-named command
// and CI guard.
func newGenerateManifestCommand() *cobra.Command {
	var readme string
	cmd := &cobra.Command{
		Use:    "generate-manifest",
		Short:  "Regenerate the README app-manifest scopes from the command tree",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			block, err := renderManifestBlock(cmd.Root())
			if err != nil {
				return err
			}
			data, err := os.ReadFile(readme)
			if err != nil {
				return err
			}
			s := string(data)
			i := strings.Index(s, manifestBegin)
			j := strings.Index(s, manifestEnd)
			if i < 0 || j < 0 || j < i {
				return fmt.Errorf("manifest markers not found in %s", readme)
			}
			out := s[:i] + manifestBegin + "\n" + block + "\n" + s[j:]
			return os.WriteFile(readme, []byte(out), 0o644)
		},
	}
	cmd.Flags().StringVar(&readme, "readme", "README.md", "path to README.md")
	return cmd
}

// renderManifestBlock renders the fenced JSON manifest with user + bot scope
// arrays generated from root.
func renderManifestBlock(root *cobra.Command) (string, error) {
	type scopes struct {
		User []string `json:"user"`
		Bot  []string `json:"bot"`
	}
	manifest := map[string]any{
		"display_information": map[string]any{"name": "slk"},
		"oauth_config": map[string]any{
			"scopes": scopes{User: scopeUnion(root, "user"), Bot: scopeUnion(root, "bot")},
		},
		"settings": map[string]any{
			"org_deploy_enabled":    false,
			"socket_mode_enabled":   false,
			"token_rotation_enabled": false,
		},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		return "", err
	}
	return "```json\n" + strings.TrimRight(buf.String(), "\n") + "\n```", nil
}
```

在 `root.go` 的 `AddCommand(...)` 末尾加 `newGenerateManifestCommand(),`。

- [ ] **Step 5: Run test + generate the real README**

```bash
go clean -testcache && go test ./internal/commands/ -run TestGenerateManifest -v
go run ./cmd/slk generate-manifest
git diff --stat README.md
```
Expected: 測試 PASS;`README.md` manifest 區塊更新成 user(37)+bot(35)。人工確認 manifest 段落上方敘述文字(README ~line 181「The manifest's user: list…」)仍正確,必要時順手補一句說明 `bot:` list。

- [ ] **Step 6: Add a CI staleness guard**

在既有 workflow(與 `skill` / `version-sync` job 同檔)新增一個 job,內容:

```yaml
  manifest:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go run ./cmd/slk generate-manifest
      - run: git diff --exit-code README.md
```

（依該 repo 既有 workflow 的 job 命名/結構對齊;`git diff --exit-code` 在 README 過期時讓 CI fail。）

- [ ] **Step 7: Commit**

```bash
git add README.md internal/commands/generatemanifest.go internal/commands/generatemanifest_test.go internal/commands/root.go .github/workflows/
git commit -m "feat(scopes): generate README manifest user/bot scopes with a CI guard"
```

---

### Task C5: README 文件 — bot 模式邊界敘述

**Files:**
- Modify: `README.md`(在既有「Known Slack-side limitations / traps」一節)

- [ ] **Step 1: Add a "Bot mode" subsection**

依 companion §6 / §7 item 2,在 README 既有的 Slack-side 限制段落附近新增一小節(human-facing,繁中/英混用照 README 風格),要點:
- bot 模式是 user 模式的**自動化子集**,不是平行替代。
- 完全不可用(❌):Search(`search messages/files/all`、`canvas list`)、個人狀態寫入(`user set-profile/set-photo/delete-photo`、`dnd snooze/end-snooze/end`)、`file public/revoke-public`、`msg draft`;`user set-presence` 可跑但對 bot 無意義。
- 效果受限(⚠️):`msg read`/`thread read`(只看 bot 已加入的頻道、DM 限 bot 自己)、`msg update`/`msg delete`/`file delete`(只能動自己的)。
- 取得 bot token 的兩條路:`auth login --as bot`,或在 App 後台複製 `xoxb-` 後 `slk auth set-token --token xoxb-…`。

> 不要把 ❌/⚠️ 清單寫成需要手動同步的第二份真相;以「分類說明 + 指向 manifest」為主,逐 verb 細節讓使用者用 `--help` / 試跑得知。

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: document bot-mode capability boundary and token acquisition"
```

---

## Phase D — 收尾驗證

### Task D1: 全套件測試 + skills 重生 + 端到端 sanity

**Files:**
- Modify: `skills/`(若命令說明/旗標有變,需重生)

- [ ] **Step 1: Regenerate skills (command help changed: --as, set-token, login)**

```bash
go run ./cmd/slk generate-skills
git diff --stat skills/
```

- [ ] **Step 2: Full test suite**

Run: `go clean -testcache && go test ./...`
Expected: 全綠

- [ ] **Step 3: Manual dry sanity (no network)**

```bash
go build -o /tmp/slk ./cmd/slk
/tmp/slk auth status --offline            # 應印 [scope] 標籤(或 no profiles)
/tmp/slk auth status --offline --format json   # 應為合法 JSON
/tmp/slk auth login --as bot --help       # 說明應反映鑄造語意
```

- [ ] **Step 4: Commit regenerated skills**

```bash
git add skills/
git commit -m "chore(skills): regenerate after auth surface changes"
```

---

## 收尾備註(非本計畫的程式步驟)

- **真 token 驗證(使用者執行)**:`auth login --as bot` 對一個有 bot user 的真 App;bot token 下跑一個 user-only verb 應得 exit 3;若日後要把 `canvases:read` 砍到 36/34,用「帶 `files:read` 但不帶 `canvases:read`」的 token 對 canvas 跑 `canvas read` 驗證。
- **VERSION / release**:本計畫不動 `VERSION`。合併到 `main` 後再依專案規則由使用者決定是否獨立開 `chore(release)` PR(additive features → minor)。
- **push / PR / merge**:皆需使用者另外明確指示。
