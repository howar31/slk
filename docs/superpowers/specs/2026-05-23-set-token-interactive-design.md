# Design: Interactive `slk auth set-token`

- Date: 2026-05-23
- Status: brainstorming approved — pending spec review

## Problem

`slk auth set-token` accepts only flags (`--profile`, `--workspace`, `--user`, `--bot`). Two
issues:

1. **No safe token entry.** The token is passed as `--user xoxp-…`, so it lands in shell
   history and is visible via `ps` / `/proc/<pid>` while the process runs. There is no hidden
   prompt and no stdin path.
2. **Empty-profile footgun.** Running `slk auth set-token` with no flags silently saves an
   empty profile named `default` (no token), marks it active, and prints
   `saved profile "default"`. Surprising and useless.

## Goals

- Keep the existing flag interface unchanged — agents and scripts depend on it.
- When required fields are missing and a human is at a terminal, prompt for them
  interactively; token entry is hidden (no echo).
- Allow token entry via stdin (`--user -` / `--bot -`) so it never reaches shell history.
- Never block in non-interactive contexts (agents / CI / pipes): missing required input is a
  clear error, not a hang.
- Refuse to save a profile that has no token.

## Non-goals

- The broader bot-token end-to-end flow is fixed separately; this change only adds the
  bot-token **prompt/field** to set-token.
- OAuth (`auth login`) is unaffected — `client_id` / `client_secret` are not part of
  set-token.
- No "switch active profile" prompt; use `auth switch`.

## Behavior

### Interactivity decision

- **Non-interactive** when stdin is not a TTY, OR `--non-interactive` is passed. Never
  prompts.
- **Interactive** otherwise. Prompts only for fields not already supplied.

The `--non-interactive` flag is a defensive escape hatch for the edge case of an agent/CI
harness that allocates a pseudo-TTY (where TTY detection alone would misclassify the call as
interactive).

### Per-field resolution (each field independently)

Precedence:

1. Flag value is `-` → read one trimmed line from stdin.
2. Flag value is non-empty → use it.
3. Flag not supplied →
   - Interactive → prompt (tokens hidden).
   - Non-interactive → required: error; optional: leave empty.

"Supplied" for `--profile` is detected via cobra's `Flags().Changed("profile")` (it has a
non-empty default of `default`, so an empty-string check is insufficient).

If more than one flag is `-`, stdin is consumed line-by-line in field order (user token, then
bot token).

### Fields

| Field | Prompt (stderr) | Required | Notes |
|---|---|---|---|
| profile | `Profile name [default]: ` | no | empty input → `default` |
| workspace | `Workspace label (optional): ` | no | may be empty |
| user token | `Paste user token (xoxp-, hidden): ` | yes for a NEW profile | hidden; NEW profile → re-prompts until non-empty (Ctrl-C aborts); existing profile → empty Enter keeps current |
| bot token | `Paste bot token (xoxb-, optional, hidden): ` | no | hidden; empty skips; existing profile → empty Enter keeps current |

Because the interactive path always collects a user token for a new profile, an interactive
run can never trip the validation guard below.

### Validation

The invariant: after merging resolved values onto the existing profile (if any), the
resulting profile must have **at least one token** (user or bot). Otherwise refuse to save —
error, non-zero exit. This fixes the empty-profile footgun while still allowing a
workspace-only update of an existing profile (the merge preserves the existing token). A
bot-only new profile is therefore creatable only via flags / non-interactive
(`--bot xoxb-…`), since the interactive wizard always asks for a user token first.

### Active profile

Unchanged: if no active profile exists, the saved profile becomes active. No prompt.

### Streams

- All prompts and hints go to **stderr**.
- The final `saved profile "<name>"` line stays on **stdout**.

### Errors

- Non-interactive, new profile, no token supplied (neither `--user`/`--bot` nor their `-`
  stdin form):
  `error: no token; pass --user <token> or --bot <token> (use - to read stdin), or run in a terminal`
  → exit 1.

## Implementation

- Hidden input via `golang.org/x/term` (`term.ReadPassword`, `term.IsTerminal`). New
  dependency; a minimal official `golang.org/x/` package, not a TUI framework — consistent
  with the repo rule that interactive UI is human-onboarding-only and the command path stays
  framework-free.
- stdin `-` reuses the existing convention from `internal/commands/input.go`.
- Test seam — two package-level function variables, overridable in tests:
  - `isTerminal func(fd int) bool` (default `term.IsTerminal`)
  - `readSecret func(fd int) ([]byte, error)` (default `term.ReadPassword`)
  Visible-line input is read from `cmd.InOrStdin()`; output is asserted via
  `cmd.OutOrStdout()` and a captured stderr. Tests override the two seams to simulate a
  terminal and feed canned secret input without a real TTY.
- New flag: `--non-interactive` (bool, default false).
- `--user xoxp-…` behavior is unchanged (backward compatible).

## Testing

- Flag mode (all values via flags) → no prompt, profile saved (existing behavior preserved).
- Non-interactive (`isTerminal`=false) + missing user token → error, nothing written
  (empty-profile footgun fixed).
- `--user -` reads the token from stdin.
- Interactive (`isTerminal`=true, injected input + `readSecret`) → prompts only the missing
  fields, saves the expected profile; bot token empty → skipped.
- Existing profile + empty token Enter → keeps current token (workspace-only update works).
- `--non-interactive` flag registered.
- Validation: a profile that resolves to no token is rejected.

## Release impact

Additive feature → minor bump (pre-1.0). Help text changes, so `go run ./cmd/slk
generate-skills` must run and be committed. Per repo policy the VERSION bump is its own
`chore(release)` PR, timed by the user.
