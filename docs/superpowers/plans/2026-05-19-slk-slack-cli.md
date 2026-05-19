# slk — Agent-facing Slack CLI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `slk`, an open-source Go CLI that lets AI agents read/send Slack messages and manage canvases, lists, and channels, replacing the Slack MCP with token-efficient output.

**Architecture:** Single static Go binary. A thin HTTP client (`internal/api`) talks to the Slack Web API and returns raw JSON. Auth (`internal/auth`) manages BYO tokens and OAuth profiles. Output (`internal/output`) renders results in token-efficient formats. Cobra command groups (`internal/commands`) wire these together; a generic `slk api` escape hatch covers any uncurated method.

**Tech Stack:** Go 1.22+, `github.com/spf13/cobra` (CLI), `github.com/BurntSushi/toml` (config), Go stdlib `net/http` + `net/http/httptest` (client + test mocks). No Slack SDK.

**Reference spec:** `docs/superpowers/specs/2026-05-19-slack-cli-design.md`

---

## File Structure

| File | Responsibility |
|------|----------------|
| `go.mod` | Module definition, dependencies |
| `cmd/slk/main.go` | Entry point; builds root command, calls `Execute()` |
| `internal/api/client.go` | HTTP client: `Call(method, params, body)` → raw JSON |
| `internal/api/errors.go` | Slack `error` string → typed error + exit code |
| `internal/api/paginate.go` | Cursor pagination helper |
| `internal/auth/store.go` | `config.toml` load/save, profile struct |
| `internal/auth/token.go` | Resolve active token (env > config), `--as` selection |
| `internal/auth/oauth.go` | OAuth login flow with local redirect server |
| `internal/output/output.go` | `Emit(format, items)`; `Concise` interface; format dispatch |
| `internal/output/render.go` | json / jsonl / table renderers (reflection over json tags) |
| `internal/resolve/resolve.go` | ID→name resolution with on-disk cache |
| `internal/commands/root.go` | Root command, global flags, shared `runAPICommand` helper |
| `internal/commands/api.go` | `slk api` escape hatch |
| `internal/commands/auth.go` | `auth` group |
| `internal/commands/msg.go` | `msg` group |
| `internal/commands/thread.go` | `thread` group |
| `internal/commands/search.go` | `search` group |
| `internal/commands/canvas.go` | `canvas` group |
| `internal/commands/list.go` | `list` group |
| `internal/commands/channel.go` | `channel` group |
| `internal/commands/user.go` | `user` group |
| `.goreleaser.yaml` | Cross-platform release build |
| `README.md` | User documentation |
| `skill/SKILL.md` | Companion Claude Code skill |

Tests live beside the file under test (`client_test.go` next to `client.go`), per Go convention.

---

## Phase 0: Project Scaffold

### Task 0.1: Initialize Go module and entry point

**Files:**
- Create: `go.mod`
- Create: `cmd/slk/main.go`
- Create: `internal/commands/root.go`
- Test: `internal/commands/root_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/root_test.go`:
```go
package commands

import (
	"bytes"
	"testing"
)

func TestRootCommand_Version(t *testing.T) {
	cmd := NewRootCommand("test-version")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := out.String(); got == "" || !bytes.Contains(out.Bytes(), []byte("test-version")) {
		t.Fatalf("expected version output to contain test-version, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestRootCommand_Version -v`
Expected: FAIL — `go.mod` / package does not exist yet (compile error).

- [ ] **Step 3: Create the module and minimal code**

`go.mod`:
```
module github.com/howar31/slk

go 1.22

require github.com/spf13/cobra v1.8.1
```

`internal/commands/root.go`:
```go
// Package commands defines the slk command tree.
package commands

import "github.com/spf13/cobra"

// NewRootCommand builds the slk root command for the given build version.
func NewRootCommand(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "slk",
		Short:         "Agent-facing Slack CLI",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return root
}
```

`cmd/slk/main.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/howar31/slk/internal/commands"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	root := commands.NewRootCommand(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "slk:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Resolve dependencies and run the test**

Run: `go mod tidy && go test ./internal/commands/ -run TestRootCommand_Version -v`
Expected: PASS.

- [ ] **Step 5: Verify the binary builds and runs**

Run: `go build -o /tmp/slk ./cmd/slk && /tmp/slk --version`
Expected: prints `slk version dev`.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum cmd/ internal/commands/root.go internal/commands/root_test.go
git commit -m "feat: scaffold slk module with root command"
```

### Task 0.2: Add .gitignore and commit the existing spec

**Files:**
- Create: `.gitignore`

- [ ] **Step 1: Create `.gitignore`**

```
/slk
/dist/
*.test
/tmp/
```

- [ ] **Step 2: Commit scaffold housekeeping and the design spec**

```bash
git add .gitignore docs/
git commit -m "chore: add gitignore and design spec"
```

---

## Phase 1: API Client

### Task 1.1: Slack error mapping with exit codes

**Files:**
- Create: `internal/api/errors.go`
- Test: `internal/api/errors_test.go`

- [ ] **Step 1: Write the failing test**

`internal/api/errors_test.go`:
```go
package api

import "testing"

func TestExitCodeFor(t *testing.T) {
	cases := map[string]int{
		"invalid_auth":       3,
		"token_expired":      3,
		"not_authed":         3,
		"channel_not_found":  4,
		"user_not_found":     4,
		"ratelimited":        5,
		"something_else":     1,
	}
	for slackErr, want := range cases {
		if got := ExitCodeFor(slackErr); got != want {
			t.Errorf("ExitCodeFor(%q) = %d, want %d", slackErr, got, want)
		}
	}
}

func TestAPIError_Error(t *testing.T) {
	e := &APIError{SlackError: "channel_not_found", Method: "conversations.history"}
	if e.Error() != "conversations.history: channel_not_found" {
		t.Fatalf("unexpected message: %q", e.Error())
	}
	if e.ExitCode() != 4 {
		t.Fatalf("ExitCode = %d, want 4", e.ExitCode())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestExitCode -v`
Expected: FAIL — `api` package does not exist.

- [ ] **Step 3: Write the implementation**

`internal/api/errors.go`:
```go
package api

import "fmt"

// APIError is a structured failure from a Slack Web API call.
type APIError struct {
	Method     string // Slack method, e.g. "conversations.history"
	SlackError string // Slack "error" field, e.g. "channel_not_found"
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Method, e.SlackError)
}

// ExitCode returns the process exit code for this error.
func (e *APIError) ExitCode() int { return ExitCodeFor(e.SlackError) }

// ExitCodeFor maps a Slack error string to a slk exit code.
func ExitCodeFor(slackErr string) int {
	switch slackErr {
	case "invalid_auth", "token_expired", "not_authed", "account_inactive":
		return 3
	case "channel_not_found", "user_not_found", "thread_not_found", "message_not_found":
		return 4
	case "ratelimited", "rate_limited":
		return 5
	default:
		return 1
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestExitCode -v && go test ./internal/api/ -run TestAPIError -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/errors.go internal/api/errors_test.go
git commit -m "feat: add Slack API error mapping with exit codes"
```

### Task 1.2: HTTP client `Call`

**Files:**
- Create: `internal/api/client.go`
- Test: `internal/api/client_test.go`

- [ ] **Step 1: Write the failing test**

`internal/api/client_test.go`:
```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Call_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer xoxp-test" {
			t.Errorf("missing bearer token, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"messages":[{"text":"hi"}]}`))
	}))
	defer srv.Close()

	c := New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("conversations.history", map[string]string{"channel": "C1"}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var got map[string]any
	json.Unmarshal(raw, &got)
	if got["ok"] != true {
		t.Fatalf("expected ok:true, got %v", got)
	}
}

func TestClient_Call_SlackError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
	}))
	defer srv.Close()

	c := New("xoxp-test")
	c.BaseURL = srv.URL

	_, err := c.Call("conversations.history", nil, nil)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T (%v)", err, err)
	}
	if apiErr.SlackError != "channel_not_found" {
		t.Fatalf("got %q", apiErr.SlackError)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestClient_Call -v`
Expected: FAIL — `New` / `Client` undefined.

- [ ] **Step 3: Write the implementation**

`internal/api/client.go`:
```go
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a thin Slack Web API HTTP client.
type Client struct {
	Token      string
	BaseURL    string // overridable for tests; default https://slack.com/api
	HTTP       *http.Client
	MaxRetries int // rate-limit retries
}

// New returns a Client for the given token.
func New(token string) *Client {
	return &Client{
		Token:      token,
		BaseURL:    "https://slack.com/api",
		HTTP:       &http.Client{Timeout: 30 * time.Second},
		MaxRetries: 3,
	}
}

// Call invokes a Slack method. params become POST form fields; if body is
// non-nil it is sent as a JSON body instead. Returns the raw response bytes.
func (c *Client) Call(method string, params map[string]string, body []byte) ([]byte, error) {
	endpoint := c.BaseURL + "/" + method

	for attempt := 0; ; attempt++ {
		req, err := c.buildRequest(endpoint, params, body)
		if err != nil {
			return nil, err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			wait := parseRetryAfter(resp.Header.Get("Retry-After"))
			time.Sleep(wait)
			continue
		}

		var envelope struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, fmt.Errorf("%s: invalid JSON response: %w", method, err)
		}
		if !envelope.OK {
			if envelope.Error == "ratelimited" && attempt < c.MaxRetries {
				time.Sleep(time.Second * time.Duration(attempt+1))
				continue
			}
			return nil, &APIError{Method: method, SlackError: envelope.Error}
		}
		return raw, nil
	}
}

func (c *Client) buildRequest(endpoint string, params map[string]string, body []byte) (*http.Request, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest("POST", endpoint, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
		}
	} else {
		form := url.Values{}
		for k, v := range params {
			form.Set(k, v)
		}
		req, err = http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return req, nil
}

func parseRetryAfter(h string) time.Duration {
	var secs int
	if _, err := fmt.Sscanf(h, "%d", &secs); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return time.Second
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestClient_Call -v`
Expected: PASS (both subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/api/client.go internal/api/client_test.go
git commit -m "feat: add Slack Web API HTTP client with rate-limit retry"
```

### Task 1.3: Cursor pagination helper

**Files:**
- Create: `internal/api/paginate.go`
- Test: `internal/api/paginate_test.go`

- [ ] **Step 1: Write the failing test**

`internal/api/paginate_test.go`:
```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_CallAll_FollowsCursor(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			w.Write([]byte(`{"ok":true,"items":[1],"response_metadata":{"next_cursor":"abc"}}`))
		} else {
			w.Write([]byte(`{"ok":true,"items":[2],"response_metadata":{"next_cursor":""}}`))
		}
	}))
	defer srv.Close()

	c := New("xoxp-test")
	c.BaseURL = srv.URL

	pages, err := c.CallAll("conversations.list", nil, 10)
	if err != nil {
		t.Fatalf("callAll: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	var p2 map[string]any
	json.Unmarshal(pages[1], &p2)
	if p2["ok"] != true {
		t.Fatalf("page 2 malformed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestClient_CallAll -v`
Expected: FAIL — `CallAll` undefined.

- [ ] **Step 3: Write the implementation**

`internal/api/paginate.go`:
```go
package api

import "encoding/json"

// CallAll repeatedly calls method, following response_metadata.next_cursor,
// until the cursor is empty or maxPages is reached. Returns one raw response
// per page.
func (c *Client) CallAll(method string, params map[string]string, maxPages int) ([][]byte, error) {
	if params == nil {
		params = map[string]string{}
	}
	var pages [][]byte
	cursor := ""
	for i := 0; i < maxPages; i++ {
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.Call(method, params, nil)
		if err != nil {
			return pages, err
		}
		pages = append(pages, raw)

		var meta struct {
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		json.Unmarshal(raw, &meta)
		cursor = meta.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}
	return pages, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -v`
Expected: PASS (all api tests).

- [ ] **Step 5: Commit**

```bash
git add internal/api/paginate.go internal/api/paginate_test.go
git commit -m "feat: add cursor pagination helper"
```

---

## Phase 2: Auth

### Task 2.1: Profile store (config.toml load/save)

**Files:**
- Create: `internal/auth/store.go`
- Test: `internal/auth/store_test.go`

- [ ] **Step 1: Write the failing test**

`internal/auth/store_test.go`:
```go
package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_SaveLoadRoundTrip(t *testing.T) {
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
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config perms = %o, want 600", info.Mode().Perm())
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Active != "work" || loaded.Profiles["work"].UserToken != "xoxp-1" {
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestStore -v`
Expected: FAIL — `auth` package does not exist.

- [ ] **Step 3: Write the implementation**

`internal/auth/store.go`:
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

// Profile holds credentials for one Slack workspace.
type Profile struct {
	Workspace    string `toml:"workspace"`
	UserToken    string `toml:"user_token,omitempty"`
	BotToken     string `toml:"bot_token,omitempty"`
	ClientID     string `toml:"client_id,omitempty"`
	ClientSecret string `toml:"client_secret,omitempty"`
}

// Config is the on-disk slk configuration.
type Config struct {
	Active   string             `toml:"active"`
	Profiles map[string]Profile `toml:"profiles"`
}

// DefaultPath returns ~/.config/slk/config.toml.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "slk", "config.toml"), nil
}

// Load reads the config; a missing file yields an empty Config and no error.
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
	return cfg, nil
}

// Save writes the config atomically with 0600 permissions.
func Save(path string, cfg *Config) error {
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
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
```

- [ ] **Step 4: Add dependency and run the test**

Run: `go get github.com/BurntSushi/toml@v1.4.0 && go test ./internal/auth/ -run TestStore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/store.go internal/auth/store_test.go go.mod go.sum
git commit -m "feat: add profile config store"
```

### Task 2.2: Token resolution (env override, identity selection)

**Files:**
- Create: `internal/auth/token.go`
- Test: `internal/auth/token_test.go`

- [ ] **Step 1: Write the failing test**

`internal/auth/token_test.go`:
```go
package auth

import "testing"

func TestResolveToken_EnvWins(t *testing.T) {
	cfg := &Config{Active: "work", Profiles: map[string]Profile{
		"work": {UserToken: "xoxp-config"},
	}}
	tok, err := ResolveToken(cfg, "", "user", "xoxp-env")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tok != "xoxp-env" {
		t.Fatalf("env should win, got %q", tok)
	}
}

func TestResolveToken_IdentitySelection(t *testing.T) {
	cfg := &Config{Active: "work", Profiles: map[string]Profile{
		"work": {UserToken: "xoxp-u", BotToken: "xoxb-b"},
	}}
	u, _ := ResolveToken(cfg, "", "user", "")
	b, _ := ResolveToken(cfg, "", "bot", "")
	if u != "xoxp-u" || b != "xoxb-b" {
		t.Fatalf("identity selection failed: user=%q bot=%q", u, b)
	}
}

func TestResolveToken_MissingProfile(t *testing.T) {
	cfg := &Config{Profiles: map[string]Profile{}}
	if _, err := ResolveToken(cfg, "", "user", ""); err == nil {
		t.Fatal("expected error when no profile and no env token")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestResolveToken -v`
Expected: FAIL — `ResolveToken` undefined.

- [ ] **Step 3: Write the implementation**

`internal/auth/token.go`:
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
		return "", fmt.Errorf("no profile selected; run 'slk auth set-token' or set SLK_TOKEN")
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return "", fmt.Errorf("profile %q not found", name)
	}
	switch identity {
	case "bot":
		if p.BotToken == "" {
			return "", fmt.Errorf("profile %q has no bot token", name)
		}
		return p.BotToken, nil
	default:
		if p.UserToken == "" {
			return "", fmt.Errorf("profile %q has no user token", name)
		}
		return p.UserToken, nil
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth/ -run TestResolveToken -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/token.go internal/auth/token_test.go
git commit -m "feat: add token resolution with env override and identity selection"
```

### Task 2.3: OAuth login flow

**Files:**
- Create: `internal/auth/oauth.go`
- Test: `internal/auth/oauth_test.go`

- [ ] **Step 1: Write the failing test**

`internal/auth/oauth_test.go`:
```go
package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExchangeCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code") != "the-code" {
			t.Errorf("missing code, got %q", r.Form.Get("code"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"access_token": "xoxb-bot",
			"authed_user":  map[string]string{"access_token": "xoxp-user"},
		})
	}))
	defer srv.Close()

	got, err := ExchangeCode(srv.URL, "cid", "secret", "the-code", "http://localhost:0/cb")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if got.UserToken != "xoxp-user" || got.BotToken != "xoxb-bot" {
		t.Fatalf("unexpected tokens: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestExchangeCode -v`
Expected: FAIL — `ExchangeCode` undefined.

- [ ] **Step 3: Write the implementation**

`internal/auth/oauth.go`:
```go
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// TokenPair is the result of an OAuth exchange.
type TokenPair struct {
	UserToken string
	BotToken  string
}

// ExchangeCode trades an OAuth authorization code for tokens via oauth.v2.access.
// baseURL is normally https://slack.com/api (overridable for tests).
func ExchangeCode(baseURL, clientID, clientSecret, code, redirectURI string) (TokenPair, error) {
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}
	resp, err := http.PostForm(baseURL, form)
	if err != nil {
		return TokenPair{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var r struct {
		OK          bool   `json:"ok"`
		Error       string `json:"error"`
		AccessToken string `json:"access_token"`
		AuthedUser  struct {
			AccessToken string `json:"access_token"`
		} `json:"authed_user"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return TokenPair{}, fmt.Errorf("oauth: invalid response: %w", err)
	}
	if !r.OK {
		return TokenPair{}, fmt.Errorf("oauth exchange failed: %s", r.Error)
	}
	return TokenPair{UserToken: r.AuthedUser.AccessToken, BotToken: r.AccessToken}, nil
}

// WaitForCode starts a local HTTP server on addr, returns the first OAuth
// "code" query parameter it receives, then shuts down. Times out after 5 min.
func WaitForCode(addr, callbackPath string) (string, error) {
	codeCh := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "slk: authorization received. You may close this tab.")
		codeCh <- code
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go srv.ListenAndServe()
	defer srv.Shutdown(context.Background())

	select {
	case code := <-codeCh:
		return code, nil
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("oauth: timed out waiting for callback")
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth/ -v`
Expected: PASS (all auth tests).

- [ ] **Step 5: Commit**

```bash
git add internal/auth/oauth.go internal/auth/oauth_test.go
git commit -m "feat: add OAuth code exchange and local callback server"
```

---

## Phase 3: Output

### Task 3.1: Render formats (json, jsonl, table) and Concise interface

**Files:**
- Create: `internal/output/output.go`
- Create: `internal/output/render.go`
- Test: `internal/output/output_test.go`

- [ ] **Step 1: Write the failing test**

`internal/output/output_test.go`:
```go
package output

import (
	"bytes"
	"strings"
	"testing"
)

type sampleMsg struct {
	User string `json:"user"`
	Text string `json:"text"`
}

func (m sampleMsg) Concise() string { return m.User + ": " + m.Text }

func TestEmit_Concise(t *testing.T) {
	var buf bytes.Buffer
	items := []sampleMsg{{"Bob", "hi"}, {"Alice", "yo"}}
	if err := Emit(&buf, "concise", items); err != nil {
		t.Fatalf("emit: %v", err)
	}
	want := "Bob: hi\nAlice: yo\n"
	if buf.String() != want {
		t.Fatalf("concise = %q, want %q", buf.String(), want)
	}
}

func TestEmit_JSON(t *testing.T) {
	var buf bytes.Buffer
	Emit(&buf, "json", []sampleMsg{{"Bob", "hi"}})
	if !strings.Contains(buf.String(), `"user": "Bob"`) {
		t.Fatalf("json missing field: %s", buf.String())
	}
}

func TestEmit_JSONL(t *testing.T) {
	var buf bytes.Buffer
	Emit(&buf, "jsonl", []sampleMsg{{"Bob", "hi"}, {"Alice", "yo"}})
	lines := strings.Count(strings.TrimSpace(buf.String()), "\n") + 1
	if lines != 2 {
		t.Fatalf("jsonl expected 2 lines, got %d", lines)
	}
}

func TestEmit_Table(t *testing.T) {
	var buf bytes.Buffer
	Emit(&buf, "table", []sampleMsg{{"Bob", "hi"}})
	out := buf.String()
	if !strings.Contains(out, "USER") || !strings.Contains(out, "Bob") {
		t.Fatalf("table missing header/row: %s", out)
	}
}

func TestEmit_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := Emit(&buf, "xml", []sampleMsg{{"Bob", "hi"}}); err == nil {
		t.Fatal("expected error for unknown format")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/output/ -v`
Expected: FAIL — `output` package / `Emit` undefined.

- [ ] **Step 3: Write the implementations**

`internal/output/output.go`:
```go
// Package output renders command results in token-efficient formats.
package output

import (
	"fmt"
	"io"
	"reflect"
)

// Concise is implemented by item types that can render a one-line summary.
type Concise interface {
	Concise() string
}

// Emit writes items to w in the given format: concise, json, jsonl, table.
// items must be a slice.
func Emit(w io.Writer, format string, items any) error {
	v := reflect.ValueOf(items)
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("output: items must be a slice, got %T", items)
	}
	switch format {
	case "concise", "":
		return emitConcise(w, v)
	case "json":
		return emitJSON(w, items)
	case "jsonl":
		return emitJSONL(w, v)
	case "table":
		return emitTable(w, v)
	default:
		return fmt.Errorf("output: unknown format %q (want concise|json|jsonl|table)", format)
	}
}

func emitConcise(w io.Writer, v reflect.Value) error {
	for i := 0; i < v.Len(); i++ {
		c, ok := v.Index(i).Interface().(Concise)
		if !ok {
			return fmt.Errorf("output: %s does not implement Concise", v.Index(i).Type())
		}
		if _, err := fmt.Fprintln(w, c.Concise()); err != nil {
			return err
		}
	}
	return nil
}
```

`internal/output/render.go`:
```go
package output

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"text/tabwriter"
)

func emitJSON(w io.Writer, items any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func emitJSONL(w io.Writer, v reflect.Value) error {
	enc := json.NewEncoder(w)
	for i := 0; i < v.Len(); i++ {
		if err := enc.Encode(v.Index(i).Interface()); err != nil {
			return err
		}
	}
	return nil
}

// emitTable reflects over the element struct's json tags for column headers.
func emitTable(w io.Writer, v reflect.Value) error {
	if v.Len() == 0 {
		return nil
	}
	elemType := v.Index(0).Type()
	var cols []string
	for i := 0; i < elemType.NumField(); i++ {
		tag := elemType.Field(i).Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			name = elemType.Field(i).Name
		}
		cols = append(cols, name)
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = strings.ToUpper(c)
	}
	io.WriteString(tw, strings.Join(headers, "\t")+"\n")
	for i := 0; i < v.Len(); i++ {
		row := v.Index(i)
		cells := make([]string, elemType.NumField())
		for j := 0; j < elemType.NumField(); j++ {
			cells[j] = toCell(row.Field(j))
		}
		io.WriteString(tw, strings.Join(cells, "\t")+"\n")
	}
	return tw.Flush()
}

func toCell(fv reflect.Value) string {
	switch fv.Kind() {
	case reflect.String:
		return fv.String()
	default:
		b, _ := json.Marshal(fv.Interface())
		return string(b)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/output/ -v`
Expected: PASS (all five subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/output/
git commit -m "feat: add token-efficient output renderers"
```

---

## Phase 4: ID Resolution

### Task 4.1: Resolver with on-disk cache

**Files:**
- Create: `internal/resolve/resolve.go`
- Test: `internal/resolve/resolve_test.go`

- [ ] **Step 1: Write the failing test**

`internal/resolve/resolve_test.go`:
```go
package resolve

import (
	"path/filepath"
	"testing"
)

type fakeLookup struct{ calls int }

func (f *fakeLookup) Name(id string) (string, error) {
	f.calls++
	return "name-of-" + id, nil
}

func TestResolver_CachesLookups(t *testing.T) {
	fl := &fakeLookup{}
	r := New(filepath.Join(t.TempDir(), "cache.json"), fl)

	if got := r.Resolve("U123"); got != "name-of-U123" {
		t.Fatalf("first resolve = %q", got)
	}
	if got := r.Resolve("U123"); got != "name-of-U123" {
		t.Fatalf("second resolve = %q", got)
	}
	if fl.calls != 1 {
		t.Fatalf("expected 1 lookup (cached), got %d", fl.calls)
	}
}

func TestResolver_DegradesOnError(t *testing.T) {
	r := New(filepath.Join(t.TempDir(), "cache.json"), failLookup{})
	if got := r.Resolve("U999"); got != "U999" {
		t.Fatalf("expected raw ID fallback, got %q", got)
	}
}

type failLookup struct{}

func (failLookup) Name(string) (string, error) { return "", errFail }

var errFail = errTest("lookup failed")

type errTest string

func (e errTest) Error() string { return string(e) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resolve/ -v`
Expected: FAIL — `resolve` package / `New` undefined.

- [ ] **Step 3: Write the implementation**

`internal/resolve/resolve.go`:
```go
// Package resolve maps Slack IDs to human-readable names with a local cache.
package resolve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Lookup fetches the display name for an ID (user or channel).
type Lookup interface {
	Name(id string) (string, error)
}

// Resolver caches ID->name lookups on disk.
type Resolver struct {
	path   string
	lookup Lookup
	mu     sync.Mutex
	cache  map[string]string
}

// New returns a Resolver backed by cachePath and the given Lookup.
func New(cachePath string, lookup Lookup) *Resolver {
	r := &Resolver{path: cachePath, lookup: lookup, cache: map[string]string{}}
	if data, err := os.ReadFile(cachePath); err == nil {
		json.Unmarshal(data, &r.cache)
	}
	return r
}

// Resolve returns the name for id, or id itself if lookup fails.
func (r *Resolver) Resolve(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name, ok := r.cache[id]; ok {
		return name
	}
	name, err := r.lookup.Name(id)
	if err != nil {
		return id // graceful degradation
	}
	r.cache[id] = name
	r.flush()
	return name
}

func (r *Resolver) flush() {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return
	}
	data, err := json.Marshal(r.cache)
	if err != nil {
		return
	}
	os.WriteFile(r.path, data, 0o600)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/resolve/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resolve/
git commit -m "feat: add ID-to-name resolver with on-disk cache"
```

---

## Phase 5: Command Wiring and the `api` Escape Hatch

### Task 5.1: Global flags and command context

**Files:**
- Modify: `internal/commands/root.go`
- Create: `internal/commands/context.go`
- Test: `internal/commands/context_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/context_test.go`:
```go
package commands

import "testing"

func TestGlobalFlags_Defaults(t *testing.T) {
	g := &GlobalFlags{}
	root := NewRootCommand("test")
	bindGlobalFlags(root, g)
	root.SetArgs([]string{"--help"})
	_ = root.Execute()
	if g.Format != "concise" {
		t.Fatalf("default format = %q, want concise", g.Format)
	}
	if g.Identity != "user" {
		t.Fatalf("default identity = %q, want user", g.Identity)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestGlobalFlags -v`
Expected: FAIL — `GlobalFlags` / `bindGlobalFlags` undefined.

- [ ] **Step 3: Write the implementation**

`internal/commands/context.go`:
```go
package commands

import "github.com/spf13/cobra"

// GlobalFlags holds flags shared by every command.
type GlobalFlags struct {
	Format    string // concise|json|jsonl|table
	Identity  string // user|bot
	Profile   string
	Raw       bool
	DryRun    bool
	NoResolve bool
}

// bindGlobalFlags registers persistent flags on cmd, writing into g.
func bindGlobalFlags(cmd *cobra.Command, g *GlobalFlags) {
	pf := cmd.PersistentFlags()
	pf.StringVar(&g.Format, "format", "concise", "output format: concise|json|jsonl|table")
	pf.StringVar(&g.Identity, "as", "user", "identity: user|bot")
	pf.StringVar(&g.Profile, "profile", "", "config profile to use")
	pf.BoolVar(&g.Raw, "raw", false, "return raw Slack API response")
	pf.BoolVar(&g.DryRun, "dry-run", false, "validate without calling the API")
	pf.BoolVar(&g.NoResolve, "no-resolve", false, "do not resolve IDs to names")
}
```

Modify `internal/commands/root.go` — replace `NewRootCommand` body so it binds global flags and attaches subcommands:
```go
// Package commands defines the slk command tree.
package commands

import "github.com/spf13/cobra"

// NewRootCommand builds the slk root command for the given build version.
func NewRootCommand(version string) *cobra.Command {
	g := &GlobalFlags{}
	root := &cobra.Command{
		Use:           "slk",
		Short:         "Agent-facing Slack CLI",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	bindGlobalFlags(root, g)
	root.AddCommand(
		newAPICommand(g),
		newAuthCommand(g),
		newMsgCommand(g),
		newThreadCommand(g),
		newSearchCommand(g),
		newCanvasCommand(g),
		newListCommand(g),
		newChannelCommand(g),
		newUserCommand(g),
	)
	return root
}
```

> Note: this references command constructors created in Tasks 5.2–9.1. Implement Task 5.2 next; until all constructors exist the package will not compile. To keep the build green between tasks, add temporary stubs returning `&cobra.Command{Use: "<name>"}` for not-yet-implemented groups, and replace each stub when its task lands.

- [ ] **Step 4: Add stubs so the package compiles**

Create `internal/commands/stubs.go` with stub constructors for every group except `api` (built next):
```go
package commands

import "github.com/spf13/cobra"

func newAuthCommand(*GlobalFlags) *cobra.Command    { return &cobra.Command{Use: "auth"} }
func newMsgCommand(*GlobalFlags) *cobra.Command     { return &cobra.Command{Use: "msg"} }
func newThreadCommand(*GlobalFlags) *cobra.Command  { return &cobra.Command{Use: "thread"} }
func newSearchCommand(*GlobalFlags) *cobra.Command  { return &cobra.Command{Use: "search"} }
func newCanvasCommand(*GlobalFlags) *cobra.Command  { return &cobra.Command{Use: "canvas"} }
func newListCommand(*GlobalFlags) *cobra.Command    { return &cobra.Command{Use: "list"} }
func newChannelCommand(*GlobalFlags) *cobra.Command { return &cobra.Command{Use: "channel"} }
func newUserCommand(*GlobalFlags) *cobra.Command    { return &cobra.Command{Use: "user"} }
```
Each task below DELETES its stub from `stubs.go` when it adds the real constructor.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/commands/ -run TestGlobalFlags -v`
Expected: PASS. (The `api` command is added in Task 5.2; until then, also temporarily stub `newAPICommand` in `stubs.go`, then remove it in 5.2.)

- [ ] **Step 6: Commit**

```bash
git add internal/commands/
git commit -m "feat: add global flags and command wiring"
```

### Task 5.2: `slk api` escape hatch and shared client builder

**Files:**
- Create: `internal/commands/clientutil.go`
- Create: `internal/commands/api.go`
- Modify: `internal/commands/stubs.go` (remove `newAPICommand` stub)
- Test: `internal/commands/api_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/api_test.go`:
```go
package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestAPICommand_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newAPICommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"conversations.list", "--params", `{"limit":"5"}`})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "conversations.list") {
		t.Fatalf("dry-run should echo the method, got %q", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestAPICommand -v`
Expected: FAIL — `newAPICommand` is currently a stub returning an empty command.

- [ ] **Step 3: Write the implementations**

`internal/commands/clientutil.go`:
```go
package commands

import (
	"os"
	"path/filepath"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/auth"
)

// buildClient resolves the active token and returns a ready API client.
func buildClient(g *GlobalFlags) (*api.Client, error) {
	path, err := auth.DefaultPath()
	if err != nil {
		return nil, err
	}
	cfg, err := auth.Load(path)
	if err != nil {
		return nil, err
	}
	token, err := auth.ResolveToken(cfg, g.Profile, g.Identity, os.Getenv("SLK_TOKEN"))
	if err != nil {
		return nil, err
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

`internal/commands/api.go`:
```go
package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// newAPICommand builds `slk api <method>` — the generic escape hatch.
func newAPICommand(g *GlobalFlags) *cobra.Command {
	var paramsJSON, bodyJSON string
	cmd := &cobra.Command{
		Use:   "api <method>",
		Short: "Call any Slack Web API method directly",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := args[0]
			params := map[string]string{}
			if paramsJSON != "" {
				if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
					return fmt.Errorf("invalid --params JSON: %w", err)
				}
			}
			var body []byte
			if bodyJSON != "" {
				body = []byte(bodyJSON)
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] %s params=%v body=%s\n", method, params, bodyJSON)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call(method, params, body)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return nil
		},
	}
	cmd.Flags().StringVar(&paramsJSON, "params", "", "query/form params as JSON object")
	cmd.Flags().StringVar(&bodyJSON, "json", "", "request body as raw JSON")
	return cmd
}
```

Remove the `newAPICommand` stub from `stubs.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/commands/ -run TestAPICommand -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 5: Set the exit code in main**

Modify `cmd/slk/main.go` — replace the error branch so typed API errors set the right exit code:
```go
func main() {
	root := commands.NewRootCommand(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "slk:", err)
		var apiErr *api.APIError
		if errors.As(err, &apiErr) {
			os.Exit(apiErr.ExitCode())
		}
		os.Exit(1)
	}
}
```
Add `"errors"` and `"github.com/howar31/slk/internal/api"` to the imports.

- [ ] **Step 6: Commit**

```bash
git add internal/commands/ cmd/slk/main.go
git commit -m "feat: add slk api escape hatch and exit-code propagation"
```

---

## Phase 6: Messaging Commands

> Pattern note: every read command below follows the same shape — build client, call method, decode into a typed slice whose elements implement `output.Concise`, then `output.Emit`. Every write command supports `--dry-run`. Each task gives the complete code for its commands; there is no shared "do the call" helper beyond `buildClient`, because each command's decode/format step is genuinely different.

### Task 6.1: `msg read`

**Files:**
- Create: `internal/commands/msg.go`
- Modify: `internal/commands/stubs.go` (remove `newMsgCommand` stub)
- Test: `internal/commands/msg_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/msg_test.go`:
```go
package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestMsgMessage_Concise(t *testing.T) {
	m := msgItem{User: "Bob", Text: "hi", TS: "1779191572.0"}
	if got := m.Concise(); !strings.Contains(got, "Bob") || !strings.Contains(got, "hi") {
		t.Fatalf("concise = %q", got)
	}
}

func TestMsgSend_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newMsgCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"send", "--channel", "C1", "--text", "hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "hello") || !strings.Contains(out.String(), "C1") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestMsg -v`
Expected: FAIL — `msgItem` / real `newMsgCommand` undefined.

- [ ] **Step 3: Write the implementation**

`internal/commands/msg.go`:
```go
package commands

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

// msgItem is one trimmed message.
type msgItem struct {
	User   string `json:"user"`
	Text   string `json:"text"`
	TS     string `json:"ts"`
	Thread string `json:"thread,omitempty"`
}

func (m msgItem) Concise() string {
	return fmt.Sprintf("%s: %s [%s]", m.User, m.Text, shortTS(m.TS))
}

// shortTS renders a Slack ts (e.g. "1779191572.123") as "MM-DD HH:MM".
func shortTS(ts string) string {
	var sec int64
	fmt.Sscanf(ts, "%d", &sec)
	if sec == 0 {
		return ts
	}
	return time.Unix(sec, 0).Format("01-02 15:04")
}

func newMsgCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "msg", Short: "Read and send messages"}
	cmd.AddCommand(newMsgReadCommand(g), newMsgSendCommand(g))
	return cmd
}

func newMsgReadCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	var limit int
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read messages from a channel or DM",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.history", map[string]string{
				"channel": channel,
				"limit":   fmt.Sprintf("%d", limit),
			}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Messages []struct {
					User      string `json:"user"`
					Text      string `json:"text"`
					TS        string `json:"ts"`
					ThreadTS  string `json:"thread_ts"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			items := make([]msgItem, len(resp.Messages))
			for i, m := range resp.Messages {
				items[i] = msgItem{User: m.User, Text: m.Text, TS: m.TS, Thread: m.ThreadTS}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID or user ID for a DM")
	cmd.Flags().IntVar(&limit, "limit", 50, "max messages")
	cmd.MarkFlagRequired("channel")
	return cmd
}

func newMsgSendCommand(g *GlobalFlags) *cobra.Command {
	var channel, text, threadTS string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message to a channel or DM",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "text": text}
			if threadTS != "" {
				params["thread_ts"] = threadTS
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] chat.postMessage channel=%s text=%q\n", channel, text)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("chat.postMessage", params, nil)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "sent")
			_ = raw
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID or user ID")
	cmd.Flags().StringVar(&text, "text", "", "message text")
	cmd.Flags().StringVar(&threadTS, "thread", "", "reply in this thread ts")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("text")
	return cmd
}
```

Remove the `newMsgCommand` stub from `stubs.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/commands/ -run TestMsg -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/
git commit -m "feat: add msg read and msg send commands"
```

### Task 6.2: `msg update`, `msg delete`, `msg react`, `msg schedule`

**Files:**
- Modify: `internal/commands/msg.go`
- Test: `internal/commands/msg_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/commands/msg_test.go`:
```go
func TestMsgUpdateDelete_DryRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"update", []string{"update", "--channel", "C1", "--ts", "1.0", "--text", "new"}, "chat.update"},
		{"delete", []string{"delete", "--channel", "C1", "--ts", "1.0"}, "chat.delete"},
		{"react", []string{"react", "--channel", "C1", "--ts", "1.0", "--emoji", "thumbsup"}, "reactions.add"},
		{"schedule", []string{"schedule", "--channel", "C1", "--text", "later", "--at", "1799999999"}, "chat.scheduleMessage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &GlobalFlags{DryRun: true}
			cmd := newMsgCommand(g)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("want %q in %q", tc.want, out.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestMsgUpdateDelete -v`
Expected: FAIL — subcommands not registered.

- [ ] **Step 3: Write the implementation**

Add to `internal/commands/msg.go` — register the new subcommands in `newMsgCommand`:
```go
func newMsgCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "msg", Short: "Read and send messages"}
	cmd.AddCommand(
		newMsgReadCommand(g),
		newMsgSendCommand(g),
		newMsgWriteCommand(g, "update", "chat.update", []string{"channel", "ts", "text"}),
		newMsgWriteCommand(g, "delete", "chat.delete", []string{"channel", "ts"}),
		newMsgReactCommand(g),
		newMsgScheduleCommand(g),
	)
	return cmd
}

// newMsgWriteCommand builds a simple write command mapping required flags to
// Slack form params of the same name.
func newMsgWriteCommand(g *GlobalFlags, use, method string, flags []string) *cobra.Command {
	values := map[string]*string{}
	cmd := &cobra.Command{
		Use:   use,
		Short: use + " a message",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{}
			for _, f := range flags {
				params[f] = *values[f]
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] %s %v\n", method, params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call(method, params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), use+" ok")
			return nil
		},
	}
	for _, f := range flags {
		v := new(string)
		values[f] = v
		cmd.Flags().StringVar(v, f, "", f+" value")
		cmd.MarkFlagRequired(f)
	}
	return cmd
}

func newMsgReactCommand(g *GlobalFlags) *cobra.Command {
	var channel, ts, emoji string
	cmd := &cobra.Command{
		Use:   "react",
		Short: "Add an emoji reaction to a message",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "timestamp": ts, "name": emoji}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] reactions.add %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("reactions.add", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "reacted")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&ts, "ts", "", "message timestamp")
	cmd.Flags().StringVar(&emoji, "emoji", "", "emoji name without colons")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("ts")
	cmd.MarkFlagRequired("emoji")
	return cmd
}

func newMsgScheduleCommand(g *GlobalFlags) *cobra.Command {
	var channel, text, at string
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Schedule a message for a future time",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "text": text, "post_at": at}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] chat.scheduleMessage %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("chat.scheduleMessage", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "scheduled")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&text, "text", "", "message text")
	cmd.Flags().StringVar(&at, "at", "", "Unix timestamp to post at")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("text")
	cmd.MarkFlagRequired("at")
	return cmd
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/commands/ -run TestMsg -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/msg.go internal/commands/msg_test.go
git commit -m "feat: add msg update/delete/react/schedule commands"
```

### Task 6.3: `thread read` and `thread reply`

**Files:**
- Create: `internal/commands/thread.go`
- Modify: `internal/commands/stubs.go` (remove `newThreadCommand` stub)
- Test: `internal/commands/thread_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/thread_test.go`:
```go
package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestThreadReply_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newThreadCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"reply", "--channel", "C1", "--thread", "1.0", "--text", "reply!"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "reply!") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestThread -v`
Expected: FAIL — `newThreadCommand` is a stub.

- [ ] **Step 3: Write the implementation**

`internal/commands/thread.go`:
```go
package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newThreadCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "thread", Short: "Read and reply to threads"}
	cmd.AddCommand(newThreadReadCommand(g), newThreadReplyCommand(g))
	return cmd
}

func newThreadReadCommand(g *GlobalFlags) *cobra.Command {
	var channel, ts string
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read replies in a thread",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.replies", map[string]string{
				"channel": channel, "ts": ts,
			}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Messages []struct {
					User string `json:"user"`
					Text string `json:"text"`
					TS   string `json:"ts"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			items := make([]msgItem, len(resp.Messages))
			for i, m := range resp.Messages {
				items[i] = msgItem{User: m.User, Text: m.Text, TS: m.TS}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&ts, "ts", "", "parent message ts")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("ts")
	return cmd
}

func newThreadReplyCommand(g *GlobalFlags) *cobra.Command {
	var channel, thread, text string
	cmd := &cobra.Command{
		Use:   "reply",
		Short: "Reply within a thread",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "thread_ts": thread, "text": text}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] chat.postMessage %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("chat.postMessage", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "replied")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&thread, "thread", "", "parent message ts")
	cmd.Flags().StringVar(&text, "text", "", "reply text")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("thread")
	cmd.MarkFlagRequired("text")
	return cmd
}
```

Remove the `newThreadCommand` stub from `stubs.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/commands/ -run TestThread -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/
git commit -m "feat: add thread read and reply commands"
```

---

## Phase 7: Search and User Commands

### Task 7.1: `search messages`, `search channels`, `search users`

**Files:**
- Create: `internal/commands/search.go`
- Modify: `internal/commands/stubs.go` (remove `newSearchCommand` stub)
- Test: `internal/commands/search_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/search_test.go`:
```go
package commands

import "testing"

func TestSearchHit_Concise(t *testing.T) {
	h := searchHit{Name: "general", ID: "C1", Extra: "42 members"}
	got := h.Concise()
	if got == "" || got == "C1" {
		t.Fatalf("concise should combine fields, got %q", got)
	}
}

func TestSearchCommand_HasSubcommands(t *testing.T) {
	cmd := newSearchCommand(&GlobalFlags{})
	want := map[string]bool{"messages": false, "channels": false, "users": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing search subcommand %q", name)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestSearch -v`
Expected: FAIL — `searchHit` / real `newSearchCommand` undefined.

- [ ] **Step 3: Write the implementation**

`internal/commands/search.go`:
```go
package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

// searchHit is one trimmed search/list result.
type searchHit struct {
	Name  string `json:"name"`
	ID    string `json:"id"`
	Extra string `json:"extra,omitempty"`
}

func (h searchHit) Concise() string {
	if h.Extra != "" {
		return fmt.Sprintf("%s (%s) — %s", h.Name, h.ID, h.Extra)
	}
	return fmt.Sprintf("%s (%s)", h.Name, h.ID)
}

func newSearchCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "search", Short: "Search messages, channels, users"}
	cmd.AddCommand(
		newSearchMessagesCommand(g),
		newSearchChannelsCommand(g),
		newSearchUsersCommand(g),
	)
	return cmd
}

func newSearchMessagesCommand(g *GlobalFlags) *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:   "messages",
		Short: "Search messages (requires a user token)",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("search.messages", map[string]string{"query": query}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				Messages struct {
					Matches []struct {
						Username string `json:"username"`
						Text     string `json:"text"`
						TS       string `json:"ts"`
					} `json:"matches"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			items := make([]msgItem, len(resp.Messages.Matches))
			for i, m := range resp.Messages.Matches {
				items[i] = msgItem{User: m.Username, Text: m.Text, TS: m.TS}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query")
	cmd.MarkFlagRequired("query")
	return cmd
}

func newSearchChannelsCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channels",
		Short: "List channels",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			pages, err := client.CallAll("conversations.list",
				map[string]string{"limit": "200", "types": "public_channel,private_channel"}, 10)
			if err != nil {
				return err
			}
			var hits []searchHit
			for _, raw := range pages {
				var resp struct {
					Channels []struct {
						ID         string `json:"id"`
						Name       string `json:"name"`
						NumMembers int    `json:"num_members"`
					} `json:"channels"`
				}
				if err := json.Unmarshal(raw, &resp); err != nil {
					return err
				}
				for _, c := range resp.Channels {
					hits = append(hits, searchHit{
						Name: c.Name, ID: c.ID,
						Extra: fmt.Sprintf("%d members", c.NumMembers),
					})
				}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	return cmd
}

func newSearchUsersCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List users",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			pages, err := client.CallAll("users.list", map[string]string{"limit": "200"}, 10)
			if err != nil {
				return err
			}
			var hits []searchHit
			for _, raw := range pages {
				var resp struct {
					Members []struct {
						ID       string `json:"id"`
						Name     string `json:"name"`
						RealName string `json:"real_name"`
					} `json:"members"`
				}
				if err := json.Unmarshal(raw, &resp); err != nil {
					return err
				}
				for _, m := range resp.Members {
					hits = append(hits, searchHit{Name: m.Name, ID: m.ID, Extra: m.RealName})
				}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	return cmd
}
```

Remove the `newSearchCommand` stub from `stubs.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/commands/ -run TestSearch -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/
git commit -m "feat: add search messages/channels/users commands"
```

### Task 7.2: `user list` and `user info`

**Files:**
- Create: `internal/commands/user.go`
- Modify: `internal/commands/stubs.go` (remove `newUserCommand` stub)
- Test: `internal/commands/user_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/user_test.go`:
```go
package commands

import "testing"

func TestUserCommand_HasSubcommands(t *testing.T) {
	cmd := newUserCommand(&GlobalFlags{})
	want := map[string]bool{"list": false, "info": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing user subcommand %q", name)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestUserCommand -v`
Expected: FAIL — `newUserCommand` is a stub.

- [ ] **Step 3: Write the implementation**

`internal/commands/user.go`:
```go
package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newUserCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "List and inspect users"}
	cmd.AddCommand(newUserListCommand(g), newUserInfoCommand(g))
	return cmd
}

func newUserListCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspace users",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			pages, err := client.CallAll("users.list", map[string]string{"limit": "200"}, 10)
			if err != nil {
				return err
			}
			var hits []searchHit
			for _, raw := range pages {
				var resp struct {
					Members []struct {
						ID       string `json:"id"`
						Name     string `json:"name"`
						RealName string `json:"real_name"`
					} `json:"members"`
				}
				if err := json.Unmarshal(raw, &resp); err != nil {
					return err
				}
				for _, m := range resp.Members {
					hits = append(hits, searchHit{Name: m.Name, ID: m.ID, Extra: m.RealName})
				}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	return cmd
}

func newUserInfoCommand(g *GlobalFlags) *cobra.Command {
	var userID string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show one user's profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("users.info", map[string]string{"user": userID}, nil)
			if err != nil {
				return err
			}
			if g.Raw {
				fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var resp struct {
				User struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					RealName string `json:"real_name"`
				} `json:"user"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return err
			}
			hit := searchHit{Name: resp.User.Name, ID: resp.User.ID, Extra: resp.User.RealName}
			return output.Emit(cmd.OutOrStdout(), g.Format, []searchHit{hit})
		},
	}
	cmd.Flags().StringVar(&userID, "user", "", "user ID")
	cmd.MarkFlagRequired("user")
	return cmd
}
```

Remove the `newUserCommand` stub from `stubs.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/commands/ -run TestUserCommand -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/
git commit -m "feat: add user list and info commands"
```

---

## Phase 8: Canvas and List Commands

### Task 8.1: `canvas create`, `canvas read`, `canvas update`

**Files:**
- Create: `internal/commands/canvas.go`
- Modify: `internal/commands/stubs.go` (remove `newCanvasCommand` stub)
- Test: `internal/commands/canvas_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/canvas_test.go`:
```go
package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestCanvasCreate_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newCanvasCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"create", "--title", "Plan", "--markdown", "# hi"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "canvases.create") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestCanvas -v`
Expected: FAIL — `newCanvasCommand` is a stub.

- [ ] **Step 3: Write the implementation**

`internal/commands/canvas.go`:
```go
package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newCanvasCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "canvas", Short: "Create, read, update canvases"}
	cmd.AddCommand(newCanvasCreateCommand(g), newCanvasReadCommand(g), newCanvasUpdateCommand(g))
	return cmd
}

// canvasDocumentContent builds the document_content param for canvas methods.
func canvasDocumentContent(markdown string) string {
	b, _ := json.Marshal(map[string]string{"type": "markdown", "markdown": markdown})
	return string(b)
}

func newCanvasCreateCommand(g *GlobalFlags) *cobra.Command {
	var title, markdown string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a standalone canvas",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{
				"title":            title,
				"document_content": canvasDocumentContent(markdown),
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] canvases.create title=%q\n", title)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("canvases.create", params, nil)
			if err != nil {
				return err
			}
			var resp struct {
				CanvasID string `json:"canvas_id"`
			}
			json.Unmarshal(raw, &resp)
			fmt.Fprintf(cmd.OutOrStdout(), "created canvas %s\n", resp.CanvasID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "canvas title")
	cmd.Flags().StringVar(&markdown, "markdown", "", "canvas body in markdown")
	cmd.MarkFlagRequired("title")
	cmd.MarkFlagRequired("markdown")
	return cmd
}

func newCanvasReadCommand(g *GlobalFlags) *cobra.Command {
	var canvasID string
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read a canvas as raw API output",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("canvases.sections.lookup",
				map[string]string{"canvas_id": canvasID}, nil)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return nil
		},
	}
	cmd.Flags().StringVar(&canvasID, "id", "", "canvas ID")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newCanvasUpdateCommand(g *GlobalFlags) *cobra.Command {
	var canvasID, markdown string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Replace a canvas's content",
		RunE: func(cmd *cobra.Command, args []string) error {
			changes, _ := json.Marshal([]map[string]any{
				{"operation": "replace", "document_content": map[string]string{
					"type": "markdown", "markdown": markdown,
				}},
			})
			params := map[string]string{"canvas_id": canvasID, "changes": string(changes)}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] canvases.edit canvas_id=%s\n", canvasID)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("canvases.edit", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "canvas updated")
			return nil
		},
	}
	cmd.Flags().StringVar(&canvasID, "id", "", "canvas ID")
	cmd.Flags().StringVar(&markdown, "markdown", "", "new canvas body in markdown")
	cmd.MarkFlagRequired("id")
	cmd.MarkFlagRequired("markdown")
	return cmd
}
```

> Verification note for the implementer: confirm the exact canvas read method against <https://docs.slack.dev/reference/methods/> before wiring `canvas read`. If `canvases.sections.lookup` is not the right read method for your use case, use `slk api` semantics — pass the canvas through whichever documented read method applies. The create/update methods (`canvases.create`, `canvases.edit`) are stable.

Remove the `newCanvasCommand` stub from `stubs.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/commands/ -run TestCanvas -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/
git commit -m "feat: add canvas create/read/update commands"
```

### Task 8.2: `list create`, `list read`, `list add-item`, `list update-item`

**Files:**
- Create: `internal/commands/list.go`
- Modify: `internal/commands/stubs.go` (remove `newListCommand` stub)
- Test: `internal/commands/list_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/list_test.go`:
```go
package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestListCreate_DryRun(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newListCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"create", "--title", "Sprint backlog"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "slackLists.create") {
		t.Fatalf("dry-run output = %q", out.String())
	}
}

func TestListCommand_HasSubcommands(t *testing.T) {
	cmd := newListCommand(&GlobalFlags{})
	want := map[string]bool{"create": false, "read": false, "add-item": false, "update-item": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing list subcommand %q", name)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestList -v`
Expected: FAIL — `newListCommand` is a stub.

- [ ] **Step 3: Write the implementation**

`internal/commands/list.go`:
```go
package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newListCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "Create and manage Slack Lists"}
	cmd.AddCommand(
		newListCreateCommand(g),
		newListReadCommand(g),
		newListAddItemCommand(g),
		newListUpdateItemCommand(g),
	)
	return cmd
}

func newListCreateCommand(g *GlobalFlags) *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new List",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"name": title}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.create name=%q\n", title)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.create", params, nil)
			if err != nil {
				return err
			}
			var resp struct {
				List struct {
					ID string `json:"id"`
				} `json:"list"`
			}
			json.Unmarshal(raw, &resp)
			fmt.Fprintf(cmd.OutOrStdout(), "created list %s\n", resp.List.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "list title")
	cmd.MarkFlagRequired("title")
	return cmd
}

func newListReadCommand(g *GlobalFlags) *cobra.Command {
	var listID string
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read items in a List",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("slackLists.items.list",
				map[string]string{"list_id": listID}, nil)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return nil
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newListAddItemCommand(g *GlobalFlags) *cobra.Command {
	var listID, fieldsJSON string
	cmd := &cobra.Command{
		Use:   "add-item",
		Short: "Add an item to a List",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"list_id": listID, "initial_fields": fieldsJSON}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.items.create list_id=%s\n", listID)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("slackLists.items.create", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "item added")
			return nil
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&fieldsJSON, "fields", "[]", "initial fields as JSON array")
	cmd.MarkFlagRequired("id")
	return cmd
}

func newListUpdateItemCommand(g *GlobalFlags) *cobra.Command {
	var listID, itemID, fieldsJSON string
	cmd := &cobra.Command{
		Use:   "update-item",
		Short: "Update an item in a List",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{
				"list_id": listID, "id": itemID, "cells": fieldsJSON,
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] slackLists.items.update list_id=%s id=%s\n", listID, itemID)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("slackLists.items.update", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "item updated")
			return nil
		},
	}
	cmd.Flags().StringVar(&listID, "id", "", "list ID")
	cmd.Flags().StringVar(&itemID, "item", "", "item ID")
	cmd.Flags().StringVar(&fieldsJSON, "fields", "[]", "updated cells as JSON array")
	cmd.MarkFlagRequired("id")
	cmd.MarkFlagRequired("item")
	return cmd
}
```

> Verification note for the implementer: the Slack Lists API is recent (released Sept 2025). Before wiring, confirm the exact parameter names for `slackLists.create`, `slackLists.items.create`, and `slackLists.items.update` against <https://docs.slack.dev/reference/methods/>. The method names are stable; only the param keys (`name`/`title`, `initial_fields`/`cells`) need confirming. Adjust the `params` maps to match the documented keys.

Remove the `newListCommand` stub from `stubs.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/commands/ -run TestList -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/
git commit -m "feat: add list create/read/add-item/update-item commands"
```

---

## Phase 9: Channel Commands

### Task 9.1: `channel list/create/archive/invite/topic`

**Files:**
- Create: `internal/commands/channel.go`
- Modify: `internal/commands/stubs.go` (remove `newChannelCommand` stub — `stubs.go` is now empty and should be deleted)
- Test: `internal/commands/channel_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/channel_test.go`:
```go
package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestChannelWrite_DryRun(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"create", "--name", "new-chan"}, "conversations.create"},
		{[]string{"archive", "--channel", "C1"}, "conversations.archive"},
		{[]string{"invite", "--channel", "C1", "--users", "U1,U2"}, "conversations.invite"},
		{[]string{"topic", "--channel", "C1", "--topic", "hello"}, "conversations.setTopic"},
	} {
		g := &GlobalFlags{DryRun: true}
		cmd := newChannelCommand(g)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(tc.args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Fatalf("want %q in %q", tc.want, out.String())
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestChannelWrite -v`
Expected: FAIL — `newChannelCommand` is a stub.

- [ ] **Step 3: Write the implementation**

`internal/commands/channel.go`:
```go
package commands

import (
	"encoding/json"
	"fmt"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

func newChannelCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "channel", Short: "List and manage channels"}
	cmd.AddCommand(
		newChannelListCommand(g),
		newChannelCreateCommand(g),
		newChannelArchiveCommand(g),
		newChannelInviteCommand(g),
		newChannelTopicCommand(g),
	)
	return cmd
}

func newChannelListCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List channels",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			pages, err := client.CallAll("conversations.list",
				map[string]string{"limit": "200", "types": "public_channel,private_channel"}, 10)
			if err != nil {
				return err
			}
			var hits []searchHit
			for _, raw := range pages {
				var resp struct {
					Channels []struct {
						ID         string `json:"id"`
						Name       string `json:"name"`
						NumMembers int    `json:"num_members"`
					} `json:"channels"`
				}
				if err := json.Unmarshal(raw, &resp); err != nil {
					return err
				}
				for _, c := range resp.Channels {
					hits = append(hits, searchHit{
						Name: c.Name, ID: c.ID,
						Extra: fmt.Sprintf("%d members", c.NumMembers),
					})
				}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, hits)
		},
	}
	return cmd
}

func newChannelCreateCommand(g *GlobalFlags) *cobra.Command {
	var name string
	var private bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"name": name}
			if private {
				params["is_private"] = "true"
			}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.create %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			raw, err := client.Call("conversations.create", params, nil)
			if err != nil {
				return err
			}
			var resp struct {
				Channel struct {
					ID string `json:"id"`
				} `json:"channel"`
			}
			json.Unmarshal(raw, &resp)
			fmt.Fprintf(cmd.OutOrStdout(), "created channel %s\n", resp.Channel.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "channel name")
	cmd.Flags().BoolVar(&private, "private", false, "create a private channel")
	cmd.MarkFlagRequired("name")
	return cmd
}

func newChannelArchiveCommand(g *GlobalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Archive a channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.archive %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("conversations.archive", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "archived")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.MarkFlagRequired("channel")
	return cmd
}

func newChannelInviteCommand(g *GlobalFlags) *cobra.Command {
	var channel, users string
	cmd := &cobra.Command{
		Use:   "invite",
		Short: "Invite users to a channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "users": users}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.invite %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("conversations.invite", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "invited")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&users, "users", "", "comma-separated user IDs")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("users")
	return cmd
}

func newChannelTopicCommand(g *GlobalFlags) *cobra.Command {
	var channel, topic string
	cmd := &cobra.Command{
		Use:   "topic",
		Short: "Set a channel's topic",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{"channel": channel, "topic": topic}
			if g.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] conversations.setTopic %v\n", params)
				return nil
			}
			client, err := buildClient(g)
			if err != nil {
				return err
			}
			if _, err := client.Call("conversations.setTopic", params, nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "topic set")
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel ID")
	cmd.Flags().StringVar(&topic, "topic", "", "new topic")
	cmd.MarkFlagRequired("channel")
	cmd.MarkFlagRequired("topic")
	return cmd
}
```

Remove the `newChannelCommand` stub from `stubs.go`. `stubs.go` is now empty — delete the file: `rm internal/commands/stubs.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... && go build ./...`
Expected: PASS (all packages) and clean build.

- [ ] **Step 5: Commit**

```bash
git rm internal/commands/stubs.go
git add internal/commands/
git commit -m "feat: add channel management commands"
```

### Task 9.2: Wire ID resolution into message output

**Files:**
- Create: `internal/commands/lookup.go`
- Modify: `internal/commands/msg.go` (the `msg read` RunE)
- Modify: `internal/commands/thread.go` (the `thread read` RunE)
- Test: `internal/commands/lookup_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/lookup_test.go`:
```go
package commands

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/howar31/slk/internal/api"
)

func TestNewResolver_NilWhenNoResolve(t *testing.T) {
	if newResolver(&GlobalFlags{NoResolve: true}, nil) != nil {
		t.Fatal("expected nil resolver when --no-resolve is set")
	}
}

func TestResolveUser_NilResolverPassthrough(t *testing.T) {
	if got := resolveUser(nil, "U123"); got != "U123" {
		t.Fatalf("nil resolver should pass through, got %q", got)
	}
}

func TestSlackLookup_Name(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"user":{"name":"bob","real_name":"Bob Brown"}}`))
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL
	name, err := slackLookup{c}.Name("U1")
	if err != nil || name != "Bob Brown" {
		t.Fatalf("name=%q err=%v", name, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run "TestNewResolver|TestResolveUser|TestSlackLookup" -v`
Expected: FAIL — `newResolver` / `resolveUser` / `slackLookup` undefined.

- [ ] **Step 3: Write the implementation**

`internal/commands/lookup.go`:
```go
package commands

import (
	"encoding/json"
	"path/filepath"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/resolve"
)

// slackLookup resolves user IDs to display names via users.info.
type slackLookup struct{ client *api.Client }

func (l slackLookup) Name(id string) (string, error) {
	raw, err := l.client.Call("users.info", map[string]string{"user": id}, nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		User struct {
			Name     string `json:"name"`
			RealName string `json:"real_name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if resp.User.RealName != "" {
		return resp.User.RealName, nil
	}
	return resp.User.Name, nil
}

// newResolver returns an ID resolver, or nil if --no-resolve is set.
func newResolver(g *GlobalFlags, client *api.Client) *resolve.Resolver {
	if g.NoResolve {
		return nil
	}
	return resolve.New(filepath.Join(cacheDir(), "names.json"), slackLookup{client})
}

// resolveUser applies r to id; a nil r (or empty id) returns id unchanged.
func resolveUser(r *resolve.Resolver, id string) string {
	if r == nil || id == "" {
		return id
	}
	return r.Resolve(id)
}
```

Modify `internal/commands/msg.go` — in `newMsgReadCommand`'s `RunE`, replace the
loop that builds `items` with a resolver-aware version:
```go
			r := newResolver(g, client)
			items := make([]msgItem, len(resp.Messages))
			for i, m := range resp.Messages {
				items[i] = msgItem{
					User:   resolveUser(r, m.User),
					Text:   m.Text,
					TS:     m.TS,
					Thread: m.ThreadTS,
				}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
```

Modify `internal/commands/thread.go` — in `newThreadReadCommand`'s `RunE`,
replace the loop that builds `items`:
```go
			r := newResolver(g, client)
			items := make([]msgItem, len(resp.Messages))
			for i, m := range resp.Messages {
				items[i] = msgItem{User: resolveUser(r, m.User), Text: m.Text, TS: m.TS}
			}
			return output.Emit(cmd.OutOrStdout(), g.Format, items)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... && go build ./...`
Expected: PASS (all packages) and clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/lookup.go internal/commands/lookup_test.go internal/commands/msg.go internal/commands/thread.go
git commit -m "feat: resolve user IDs to names in message output"
```

---

## Phase 10: Auth Commands, Distribution, and Skill

### Task 10.1: `auth set-token`, `auth status`, `auth login`, `auth switch`, `auth logout`

**Files:**
- Create: `internal/commands/auth.go`
- Test: `internal/commands/auth_test.go`

- [ ] **Step 1: Write the failing test**

`internal/commands/auth_test.go`:
```go
package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/howar31/slk/internal/auth"
)

func TestAuthSetTokenAndStatus(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("SLK_CONFIG", cfgPath)

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
	_ = os.Unsetenv("SLK_CONFIG")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestAuthSetToken -v`
Expected: FAIL — real `newAuthCommand` undefined.

- [ ] **Step 3: Write the implementation**

First, add `SLK_CONFIG` override support. Modify `internal/auth/store.go` — add below `DefaultPath`:
```go
// ConfigPath returns the config path, honoring the SLK_CONFIG env override.
func ConfigPath() (string, error) {
	if p := os.Getenv("SLK_CONFIG"); p != "" {
		return p, nil
	}
	return DefaultPath()
}
```
Then in `internal/commands/clientutil.go`, change `buildClient` to call `auth.ConfigPath()` instead of `auth.DefaultPath()`.

`internal/commands/auth.go`:
```go
package commands

import (
	"fmt"

	"github.com/howar31/slk/internal/auth"
	"github.com/spf13/cobra"
)

func newAuthCommand(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage Slack credentials"}
	cmd.AddCommand(
		newAuthSetTokenCommand(),
		newAuthStatusCommand(),
		newAuthLoginCommand(),
		newAuthSwitchCommand(),
		newAuthLogoutCommand(),
	)
	return cmd
}

func loadConfig() (string, *auth.Config, error) {
	path, err := auth.ConfigPath()
	if err != nil {
		return "", nil, err
	}
	cfg, err := auth.Load(path)
	return path, cfg, err
}

func newAuthSetTokenCommand() *cobra.Command {
	var profile, workspace, userToken, botToken string
	cmd := &cobra.Command{
		Use:   "set-token",
		Short: "Store tokens for a profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			p := cfg.Profiles[profile]
			p.Workspace = workspace
			if userToken != "" {
				p.UserToken = userToken
			}
			if botToken != "" {
				p.BotToken = botToken
			}
			cfg.Profiles[profile] = p
			if cfg.Active == "" {
				cfg.Active = profile
			}
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved profile %q\n", profile)
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "default", "profile name")
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace label")
	cmd.Flags().StringVar(&userToken, "user", "", "user token (xoxp-)")
	cmd.Flags().StringVar(&botToken, "bot", "", "bot token (xoxb-)")
	return cmd
}

func newAuthStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show configured profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if len(cfg.Profiles) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no profiles configured")
				return nil
			}
			for name, p := range cfg.Profiles {
				marker := " "
				if name == cfg.Active {
					marker = "*"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s (workspace=%s user=%v bot=%v)\n",
					marker, name, p.Workspace, p.UserToken != "", p.BotToken != "")
			}
			return nil
		},
	}
}

func newAuthSwitchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <profile>",
		Short: "Set the active profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if _, ok := cfg.Profiles[args[0]]; !ok {
				return fmt.Errorf("profile %q not found", args[0])
			}
			cfg.Active = args[0]
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "active profile: %s\n", args[0])
			return nil
		},
	}
}

func newAuthLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout <profile>",
		Short: "Remove a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			delete(cfg.Profiles, args[0])
			if cfg.Active == args[0] {
				cfg.Active = ""
			}
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed profile %q\n", args[0])
			return nil
		},
	}
}

func newAuthLoginCommand() *cobra.Command {
	var profile, clientID, clientSecret, scopes, port string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Run the OAuth flow with your own Slack app credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			redirectURI := "http://localhost:" + port + "/callback"
			authURL := fmt.Sprintf(
				"https://slack.com/oauth/v2/authorize?client_id=%s&user_scope=%s&redirect_uri=%s",
				clientID, scopes, redirectURI)
			fmt.Fprintf(cmd.OutOrStdout(), "Open this URL to authorize:\n%s\n", authURL)

			code, err := auth.WaitForCode(":"+port, "/callback")
			if err != nil {
				return err
			}
			pair, err := auth.ExchangeCode("https://slack.com/api/oauth.v2.access",
				clientID, clientSecret, code, redirectURI)
			if err != nil {
				return err
			}
			path, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			p := cfg.Profiles[profile]
			p.ClientID = clientID
			p.ClientSecret = clientSecret
			if pair.UserToken != "" {
				p.UserToken = pair.UserToken
			}
			if pair.BotToken != "" {
				p.BotToken = pair.BotToken
			}
			cfg.Profiles[profile] = p
			if cfg.Active == "" {
				cfg.Active = profile
			}
			if err := auth.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "authorized profile %q\n", profile)
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "default", "profile name")
	cmd.Flags().StringVar(&clientID, "client-id", "", "your Slack app client ID")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "your Slack app client secret")
	cmd.Flags().StringVar(&scopes, "scopes", "channels:history,channels:read,chat:write,users:read", "comma-separated user scopes")
	cmd.Flags().StringVar(&port, "port", "3000", "local callback port")
	cmd.MarkFlagRequired("client-id")
	cmd.MarkFlagRequired("client-secret")
	return cmd
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... && go build ./...`
Expected: PASS (all packages) and clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/commands/auth.go internal/commands/auth_test.go internal/commands/clientutil.go internal/auth/store.go
git commit -m "feat: add auth set-token/status/login/switch/logout commands"
```

### Task 10.2: GoReleaser config and README

**Files:**
- Create: `.goreleaser.yaml`
- Create: `README.md`

- [ ] **Step 1: Write `.goreleaser.yaml`**

```yaml
version: 2
project_name: slk
builds:
  - main: ./cmd/slk
    binary: slk
    env: [CGO_ENABLED=0]
    goos: [darwin, linux]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w -X main.version={{.Version}}
archives:
  - formats: [tar.gz]
brews:
  - repository:
      owner: howar31
      name: homebrew-tap
    description: Agent-facing Slack CLI
    license: MIT
```

- [ ] **Step 2: Verify the release build locally**

Run: `go build -ldflags "-X main.version=0.1.0" -o /tmp/slk ./cmd/slk && /tmp/slk --version`
Expected: prints `slk version 0.1.0`.

- [ ] **Step 3: Write `README.md`**

````markdown
# slk

Agent-facing Slack CLI. Read and send Slack messages, manage canvases, lists,
and channels — with token-efficient output designed for AI agents.

## Install

```bash
brew install howar31/tap/slk
```

Or download a binary from the Releases page.

## Auth

`slk` uses your own Slack app. Create one at <https://api.slack.com/apps>, add
user scopes, then either:

```bash
# Paste a token directly
slk auth set-token --profile work --workspace acme --user xoxp-... --bot xoxb-...

# Or run the OAuth flow
slk auth login --profile work --client-id ... --client-secret ...
```

## Usage

```bash
slk msg read --channel C0123456789 --limit 20
slk msg send --channel C0123456789 --text "hello"
slk search channels
slk canvas create --title "Plan" --markdown "# Heading"
slk list create --title "Backlog"
slk channel archive --channel C123
slk api conversations.info --params '{"channel":"C123"}'
```

Global flags: `--format concise|json|jsonl|table`, `--as user|bot`,
`--profile`, `--raw`, `--dry-run`, `--no-resolve`.

## License

MIT
````

- [ ] **Step 4: Commit**

```bash
git add .goreleaser.yaml README.md
git commit -m "chore: add GoReleaser config and README"
```

### Task 10.3: Companion Claude Code skill

**Files:**
- Create: `skill/SKILL.md`

- [ ] **Step 1: Write `skill/SKILL.md`**

```markdown
---
name: slk
description: "slk CLI: read/send Slack messages, manage canvases, lists, channels from the terminal with token-efficient output."
metadata:
  version: 0.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - slk
---

# slk — Agent-facing Slack CLI

## Syntax

`slk <group> <verb> [flags]`

Groups: `auth`, `msg`, `thread`, `search`, `canvas`, `list`, `channel`, `user`,
`api`.

## Global flags

| Flag | Description |
|------|-------------|
| `--format` | `concise` (default), `json`, `jsonl`, `table` |
| `--as` | `user` (default) or `bot` |
| `--profile` | config profile to use |
| `--raw` | return the raw Slack API response |
| `--dry-run` | validate a write without calling the API |
| `--no-resolve` | do not resolve IDs to names |

## Common commands

```bash
slk msg read --channel C123 --limit 20
slk msg send --channel C123 --text "hello"
slk thread reply --channel C123 --thread 1779191572.0 --text "hi"
slk search channels
slk canvas create --title "Plan" --markdown "# Heading"
slk list create --title "Backlog"
slk channel invite --channel C123 --users U1,U2
slk api <method> --params '{"k":"v"}'   # escape hatch for any method
```

## Security rules

- Never print tokens. They live in `~/.config/slk/config.toml` (mode 0600).
- Confirm with the user before any write command (`send`, `create`, `archive`,
  `delete`, etc.). Use `--dry-run` first to preview.

## Exit codes

`0` ok · `2` bad args · `3` auth error · `4` not found · `5` rate limited ·
`1` other.
```

- [ ] **Step 2: Commit**

```bash
git add skill/
git commit -m "docs: add companion Claude Code skill"
```

### Task 10.4: Full verification pass

- [ ] **Step 1: Run the whole test suite**

Run: `go test ./... -v`
Expected: PASS across all packages.

- [ ] **Step 2: Run vet and build**

Run: `go vet ./... && go build -o /tmp/slk ./cmd/slk`
Expected: no vet warnings, clean build.

- [ ] **Step 3: Smoke-test the command tree**

Run: `/tmp/slk --help && /tmp/slk msg --help && /tmp/slk api --help`
Expected: each prints usage without error.

- [ ] **Step 4: Manual E2E (optional, requires a real token)**

With `SLK_TOKEN` set to a real test-workspace user token:
Run: `slk search channels --format table`
Expected: prints the real channel list. Skip in CI.

- [ ] **Step 5: Commit any final fixes**

```bash
git add -A
git commit -m "test: full verification pass for v1"
```

---

## Self-Review

**1. Spec coverage**

| Spec section | Covered by |
|--------------|-----------|
| §3 D1 replace MCP | Full command surface, Phases 6–9 |
| §3 D2 Go single binary | Phase 0, Task 10.2 |
| §3 D4 BYO token + OAuth | Tasks 2.1–2.3, 10.1 |
| §3 D6 hybrid (curated + `api`) | Task 5.2 (`api`) + Phases 6–9 (curated) |
| §3 D7 thin HTTP client | Phase 1 |
| §4 module boundaries | File Structure table; one package per responsibility |
| §5 command surface | Phases 5–10, one task per group |
| §5.2 Lists API | Task 8.2 (with verification note) |
| §6 auth model | Phase 2 + Task 10.1; `SLK_TOKEN` env override in Task 5.2 / 10.1 |
| §7 output (4 formats, `--raw`, `--dry-run`) | Phase 3, Task 5.1 flags, dry-run in every write task |
| §7 ID resolution | Phase 4 (package) + Task 9.2 (wired into `msg`/`thread` output) |
| §8 error handling / exit codes | Task 1.1 + exit propagation in Task 5.2 |
| §9 testing (unit + integration mock) | `httptest` mocks in Phases 1–2; unit tests throughout |
| §10 distribution + skill | Tasks 10.2, 10.3 |

**Resolution wiring:** The `resolve` package (Phase 4) is built and unit-tested, then wired into `msg read` and `thread read` output by Task 9.2 via `slackLookup`/`newResolver`/`resolveUser`. `--no-resolve` short-circuits `newResolver` to `nil`, and `resolveUser` degrades gracefully (nil resolver or failed lookup yields the raw ID). `search`/`user` commands need no resolution — their source APIs (`conversations.list`, `users.list`, `search.messages`) already return names.

**2. Placeholder scan:** No "TBD"/"TODO"/"implement later". Two "verification note" callouts (Tasks 8.1, 8.2) intentionally instruct the implementer to confirm recent/less-stable Slack method params against official docs — this is a verification instruction, not a placeholder; the code provided is complete and runnable as written.

**3. Type consistency:** `GlobalFlags`, `api.Client`, `api.APIError`, `auth.Config`/`Profile`, `output.Emit`/`Concise`, `msgItem`, `searchHit` are defined once and used with consistent signatures throughout. `buildClient(g)` is the single client constructor used by every command. Stub constructors in `stubs.go` share the exact signatures of their real replacements (`func(*GlobalFlags) *cobra.Command`).
