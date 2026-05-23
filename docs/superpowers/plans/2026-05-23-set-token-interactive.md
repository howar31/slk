# Interactive `slk auth set-token` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `slk auth set-token` prompt interactively (with hidden token entry) for any field not supplied via flags, accept tokens from stdin, never block in non-interactive contexts, and refuse to save a token-less profile.

**Architecture:** Keep the flag interface intact. Add a tiny, testable prompt layer (`internal/commands/prompt.go`) with two overridable seams (`isTerminal`, `readSecret`) plus pure field-resolution helpers. Rewrite `newAuthSetTokenCommand` to resolve each field through "flag → stdin `-` → prompt-if-TTY → error/empty", then validate that at least one token is present.

**Tech Stack:** Go, cobra, `golang.org/x/term` (new dep, for hidden input + TTY detection).

**Spec:** `docs/superpowers/specs/2026-05-23-set-token-interactive-design.md`

**Commit policy:** Per repo CLAUDE.md, commits happen only when the user invokes `/commit` or explicitly instructs, and it is **one commit per feature**. Do NOT commit between tasks. The final step stops and presents the work for the user to commit.

---

## File Structure

- Create `internal/commands/prompt.go` — TTY/secret seams + generic prompt & field-resolution helpers (no auth-specific logic).
- Create `internal/commands/prompt_test.go` — unit tests for the helpers.
- Modify `internal/commands/auth.go` — rewrite `newAuthSetTokenCommand`; add `os` and `bufio` imports.
- Modify `internal/commands/auth_test.go` — integration tests for set-token.
- Modify `go.mod` / `go.sum` — add `golang.org/x/term`.
- Modify `README.md` — note interactive / hidden / stdin entry in Authentication step 4.
- Regenerate `skills/` via `go run ./cmd/slk generate-skills` (help text changes).

---

## Task 1: Prompt seams and primitives

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/commands/prompt.go`
- Test: `internal/commands/prompt_test.go`

- [ ] **Step 1: Add the dependency**

Run:
```bash
go get golang.org/x/term@latest && go mod tidy
```
Expected: `go.mod` gains a `golang.org/x/term` require line; `go.sum` updated. No build yet (no usage).

- [ ] **Step 2: Write failing tests for the primitives**

Create `internal/commands/prompt_test.go`:
```go
package commands

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestPromptLine_TrimsAndWritesPrompt(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("  hello \n"))
	var w bytes.Buffer
	got, err := promptLine(in, &w, "Name: ")
	if err != nil {
		t.Fatalf("promptLine: %v", err)
	}
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
	if !strings.Contains(w.String(), "Name: ") {
		t.Fatalf("prompt not written: %q", w.String())
	}
}

func TestReadLine_NoTrailingNewline(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("xoxp-piped"))
	got, err := readLine(in)
	if err != nil {
		t.Fatalf("readLine: %v", err)
	}
	if got != "xoxp-piped" {
		t.Fatalf("got %q", got)
	}
}

func TestPromptSecret_UsesSeamAndTrims(t *testing.T) {
	orig := readSecret
	t.Cleanup(func() { readSecret = orig })
	readSecret = func(int) ([]byte, error) { return []byte(" xoxp-secret \n"), nil }

	var w bytes.Buffer
	got, err := promptSecret(0, &w, "Token: ")
	if err != nil {
		t.Fatalf("promptSecret: %v", err)
	}
	if got != "xoxp-secret" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(w.String(), "Token: ") {
		t.Fatalf("prompt not written: %q", w.String())
	}
}

var _ = io.Discard
```

- [ ] **Step 3: Run tests to verify they fail**

Run:
```bash
go test ./internal/commands/ -run 'TestPromptLine|TestReadLine|TestPromptSecret' -v
```
Expected: FAIL — `undefined: promptLine` / `readLine` / `promptSecret` / `readSecret`.

- [ ] **Step 4: Implement the primitives**

Create `internal/commands/prompt.go`:
```go
package commands

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"golang.org/x/term"
)

// Test seams. Overridden in tests to simulate a terminal without a real TTY.
var (
	isTerminal = term.IsTerminal
	readSecret = term.ReadPassword
)

// promptLine writes prompt to w and reads one whitespace-trimmed line from in.
func promptLine(in *bufio.Reader, w io.Writer, prompt string) (string, error) {
	fmt.Fprint(w, prompt)
	return readLine(in)
}

// readLine reads one whitespace-trimmed line from in. Used for `--user -`.
func readLine(in *bufio.Reader) (string, error) {
	line, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptSecret writes prompt to w and reads one line from fd with echo disabled.
// A newline is emitted to w afterwards because the terminal does not echo Enter.
func promptSecret(fd int, w io.Writer, prompt string) (string, error) {
	fmt.Fprint(w, prompt)
	b, err := readSecret(fd)
	fmt.Fprintln(w)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run:
```bash
go test ./internal/commands/ -run 'TestPromptLine|TestReadLine|TestPromptSecret' -v
```
Expected: PASS (3 tests). Remove the `var _ = io.Discard` line from the test file if `io` is otherwise unused — it is used by Task 2 tests, so leave it for now.

---

## Task 2: Field-resolution helpers

**Files:**
- Modify: `internal/commands/prompt.go`
- Test: `internal/commands/prompt_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/commands/prompt_test.go`:
```go
func TestResolveLine_FlagWins(t *testing.T) {
	got, err := resolveLine(nil, io.Discard, "work", true, true, "Profile: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "work" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveLine_NonInteractiveEmpty(t *testing.T) {
	got, err := resolveLine(nil, io.Discard, "", false, false, "Profile: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveLine_InteractivePrompts(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("acme\n"))
	got, err := resolveLine(in, io.Discard, "", false, true, "Workspace: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "acme" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecret_StdinDash(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("xoxp-piped\n"))
	got, err := resolveSecret(in, io.Discard, 0, "-", true, false, false, "Token: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "xoxp-piped" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecret_FlagValueWins(t *testing.T) {
	got, err := resolveSecret(nil, io.Discard, 0, "xoxp-flag", true, true, true, "Token: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "xoxp-flag" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecret_NonInteractiveEmpty(t *testing.T) {
	got, err := resolveSecret(nil, io.Discard, 0, "", false, false, true, "Token: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecret_RequiredRepromptsUntilNonEmpty(t *testing.T) {
	orig := readSecret
	t.Cleanup(func() { readSecret = orig })
	vals := []string{"", "", "xoxp-final"}
	readSecret = func(int) ([]byte, error) {
		v := vals[0]
		vals = vals[1:]
		return []byte(v), nil
	}
	got, err := resolveSecret(nil, io.Discard, 0, "", false, true, true, "Token: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "xoxp-final" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSecret_OptionalAcceptsEmpty(t *testing.T) {
	orig := readSecret
	t.Cleanup(func() { readSecret = orig })
	readSecret = func(int) ([]byte, error) { return []byte(""), nil }
	got, err := resolveSecret(nil, io.Discard, 0, "", false, true, false, "Token: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/commands/ -run 'TestResolveLine|TestResolveSecret' -v
```
Expected: FAIL — `undefined: resolveLine` / `resolveSecret`.

- [ ] **Step 3: Implement the helpers**

Append to `internal/commands/prompt.go`:
```go
// resolveLine resolves a non-secret field. Precedence: an explicit flag (changed)
// wins; otherwise prompt when interactive; otherwise empty.
func resolveLine(in *bufio.Reader, w io.Writer, flagVal string, changed, interactive bool, prompt string) (string, error) {
	if changed {
		return flagVal, nil
	}
	if interactive {
		return promptLine(in, w, prompt)
	}
	return "", nil
}

// resolveSecret resolves a token field. Precedence: a flag value of "-" reads one
// line from stdin; any other explicit flag value wins; otherwise prompt (hidden)
// when interactive; otherwise empty. When required and interactive, an empty entry
// re-prompts.
func resolveSecret(in *bufio.Reader, w io.Writer, fd int, flagVal string, changed, interactive, required bool, prompt string) (string, error) {
	if changed {
		if flagVal == "-" {
			return readLine(in)
		}
		return flagVal, nil
	}
	if !interactive {
		return "", nil
	}
	for {
		v, err := promptSecret(fd, w, prompt)
		if err != nil {
			return "", err
		}
		if v != "" || !required {
			return v, nil
		}
		fmt.Fprintln(w, "  a token is required")
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/commands/ -run 'TestResolveLine|TestResolveSecret' -v
```
Expected: PASS (8 tests).

---

## Task 3: Rewrite `set-token` to use the resolvers

**Files:**
- Modify: `internal/commands/auth.go` (function `newAuthSetTokenCommand`, lines 117-153; imports)
- Test: `internal/commands/auth_test.go`

- [ ] **Step 1: Write failing integration tests**

Append to `internal/commands/auth_test.go`:
```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/commands/ -run 'TestSetToken_' -v
```
Expected: FAIL — `--non-interactive` unknown flag / empty profile still saved / stdin not read.

- [ ] **Step 3: Add imports to `auth.go`**

In `internal/commands/auth.go`, change the import block to add `bufio` and `os`:
```go
import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/howar31/slk/internal/auth"
	"github.com/spf13/cobra"
)
```

- [ ] **Step 4: Rewrite `newAuthSetTokenCommand`**

Replace the entire `newAuthSetTokenCommand` function (currently `internal/commands/auth.go:117-153`) with:
```go
func newAuthSetTokenCommand() *cobra.Command {
	var profile, workspace, userToken, botToken string
	var nonInteractive bool
	cmd := &cobra.Command{
		Use:   "set-token",
		Short: "Store tokens for a profile",
		Long: "Store tokens for a profile. Pass values via flags for scripts/agents, or run " +
			"with missing fields in a terminal to be prompted (token entry is hidden). Use " +
			"--user - / --bot - to read a token from stdin.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fd := int(os.Stdin.Fd())
			interactive := !nonInteractive && isTerminal(fd)
			in := bufio.NewReader(cmd.InOrStdin())
			errw := cmd.ErrOrStderr()
			flags := cmd.Flags()

			name, err := resolveLine(in, errw, profile, flags.Changed("profile"), interactive, "Profile name [default]: ")
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
			p, existed := cfg.Profiles[name]

			ws, err := resolveLine(in, errw, workspace, flags.Changed("workspace"), interactive, "Workspace label (optional): ")
			if err != nil {
				return err
			}
			if ws != "" {
				p.Workspace = ws
			}

			// A new profile must end up with a token; an existing one may keep its current.
			ut, err := resolveSecret(in, errw, fd, userToken, flags.Changed("user"), interactive, !existed, "Paste user token (xoxp-, hidden): ")
			if err != nil {
				return err
			}
			if ut != "" {
				p.UserToken = ut
			}

			bt, err := resolveSecret(in, errw, fd, botToken, flags.Changed("bot"), interactive, false, "Paste bot token (xoxb-, optional, hidden): ")
			if err != nil {
				return err
			}
			if bt != "" {
				p.BotToken = bt
			}

			if p.UserToken == "" && p.BotToken == "" {
				return fmt.Errorf("no token; pass --user <token> or --bot <token> (use - to read stdin), or run in a terminal")
			}

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
	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace label")
	cmd.Flags().StringVar(&userToken, "user", "", "user token (xoxp-, or - to read stdin)")
	cmd.Flags().StringVar(&botToken, "bot", "", "bot token (xoxb-, or - to read stdin)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "never prompt; require values via flags")
	return cmd
}
```

- [ ] **Step 5: Run the new tests to verify they pass**

Run:
```bash
go test ./internal/commands/ -run 'TestSetToken_' -v
```
Expected: PASS (5 tests).

- [ ] **Step 6: Run the existing auth tests to confirm no regression**

Run:
```bash
go test ./internal/commands/ -run 'TestAuth' -v
```
Expected: PASS — including `TestAuthSetTokenAndStatus`, `TestAuthLogout_*` (they call set-token with full `--user` flags; in `go test` stdin is not a TTY so they stay non-interactive and behave as before).

---

## Task 4: Docs, skill regen, full verification

**Files:**
- Modify: `README.md` (Authentication step 4)
- Regenerate: `skills/` tree

- [ ] **Step 1: Update README Authentication step 4**

In `README.md`, the step 4 block currently is:
````markdown
### 4. Store the token in `slk`

```bash
slk auth set-token --profile work --workspace acme --user xoxp-...
```
````
Replace it with:
````markdown
### 4. Store the token in `slk`

```bash
slk auth set-token --profile work --workspace acme --user xoxp-...
```

To keep the token out of your shell history, omit `--user` and run it in a terminal — `slk`
prompts for each missing field and hides the token as you paste it. For scripts, pipe the
token in instead: `printf '%s' "$TOKEN" | slk auth set-token --profile work --user -`.
````

- [ ] **Step 2: Regenerate the agent skills**

Run:
```bash
go run ./cmd/slk generate-skills
```
Expected: `skills/slk-auth/SKILL.md` (and any index) updated to show the new `--non-interactive` flag and the revised `--user`/`--bot` help. CI's `skill` job enforces no drift, so this must be committed.

- [ ] **Step 3: Run the full test suite (uncached)**

Run:
```bash
go clean -testcache && go test ./...
```
Expected: PASS across all packages.

- [ ] **Step 4: Build and smoke-check the help**

Run:
```bash
go build -o slk ./cmd/slk && ./slk auth set-token --help
```
Expected: build succeeds; help lists `--non-interactive` and the `- to read stdin` hints on `--user`/`--bot`.

- [ ] **Step 5: Stop — present for commit**

Do NOT commit automatically. Summarize the diff and the verification output, then ask the user to commit via `/commit` (one commit per feature). After it merges to `main`, ask whether to cut a release (additive feature → minor bump, pre-1.0), per repo policy.

---

## Self-Review

**Spec coverage:**
- Flag interface unchanged → Task 3 keeps all flags; `TestAuthSetTokenAndStatus` regression check (Task 3 Step 6).
- Interactive prompt for missing fields, hidden token → Task 3 (`TestSetToken_InteractiveFillsMissing`) + Task 1/2 primitives.
- stdin `--user -` / `--bot -` → Task 2 (`TestResolveSecret_StdinDash`) + Task 3 (`TestSetToken_UserTokenFromStdin`).
- Never block non-interactive; clear error → Task 3 (`TestSetToken_NonInteractiveNoTokenErrors`); `--non-interactive` flag + default TTY=false in tests.
- Refuse token-less profile → Task 3 validation guard + same test.
- Existing-profile keep-current / workspace-only update → Task 3 (`TestSetToken_InteractiveKeepsExistingToken`).
- New dep `golang.org/x/term` → Task 1 Step 1.
- Help/skill regen + release note → Task 4.

**Placeholder scan:** none — every step has concrete code/commands.

**Type consistency:** `isTerminal func(int) bool`, `readSecret func(int) ([]byte, error)`, `resolveLine(in *bufio.Reader, w io.Writer, flagVal string, changed, interactive bool, prompt string)`, `resolveSecret(in *bufio.Reader, w io.Writer, fd int, flagVal string, changed, interactive, required bool, prompt string)` — signatures match across Tasks 1-3. Validation error string is identical to the spec.
