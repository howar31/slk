# slk — Agent-facing Slack CLI — Design

**Date:** 2026-05-19
**Status:** Approved design, pending implementation plan
**Repo:** `/opt/projects/slk`

## 1. Purpose

`slk` is an open-source, agent-facing command-line interface for Slack. It lets
AI agents (and humans) read and send Slack messages, manage canvases, lists, and
channels — operating either as the authorizing user or as a bot.

It is designed to fully replace the official Claude Slack MCP connector for the
maintainer's workflow. The motivation is token efficiency: the MCP returns
verbose, fixed-shape API JSON; a purpose-built CLI can emit curated, trimmed
output that costs an agent far fewer tokens per call.

## 2. Goals and non-goals

**Goals**
- Cover the full Slack feature surface needed to drop the MCP entirely.
- Token-efficient output tuned for agent consumption.
- Legal, sanctioned auth only — OAuth user/bot tokens, no browser session tokens.
- Single static binary, no runtime dependency.
- Ship as an open-source project with a companion Claude Code skill.

**Non-goals**
- No interactive prompts (hostile to agent callers).
- No browser session token (`xoxc`/`xoxd`) extraction — outside Slack's
  sanctioned API surface.
- No embedded OAuth app credentials — a public binary cannot ship a client
  secret. Users bring their own Slack app.

## 3. Key decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Fully replace the official Slack MCP | Maintainer prefers CLI over MCP; consolidate on one tool. |
| D2 | Implemented in Go, single static binary | Fast startup for frequent agent calls; no runtime deps. |
| D3 | Open-source, MIT license | Matches the agent-slack / slackcli ecosystem. |
| D4 | Auth: BYO token + self-provided OAuth | Public binary cannot embed a client secret; user supplies their own Slack app. |
| D5 | v1 ships the full feature surface | One coherent release; enough to drop the MCP. |
| D6 | Hybrid command architecture (curated verbs + `api` escape hatch) | Curated verbs give token-efficient agent UX; escape hatch guarantees API coverage. |
| D7 | Thin hand-written HTTP client, no `slack-go/slack` SDK | Full control over output trimming; escape hatch is naturally raw passthrough; smaller binary. |

## 4. Architecture

Single static Go binary named `slk`. Command tree built with Cobra (subcommands,
`--help`, shell completion).

### 4.1 Project layout

```
slk/
  cmd/slk/main.go          # entry point
  internal/
    api/                   # HTTP client, rate-limit backoff, cursor pagination
    auth/                  # token store, profiles, OAuth flow
    commands/              # Cobra command tree
    output/                # agent-facing formatters (concise/json/jsonl/table)
    resolve/               # ID <-> name resolution with local cache
  docs/
  README.md
  .goreleaser.yaml
```

### 4.2 Module boundaries

Each unit has one responsibility, a defined interface, and is independently
testable.

- `api` — sends HTTP requests, returns raw JSON, handles rate-limit backoff and
  cursor pagination. Knows nothing about commands.
- `output` — converts data into token-efficient text. Does not call the API.
- `auth` — token storage, profile management, OAuth flow.
- `resolve` — optional ID-beautification layer (`U…`/`C…` to names); degrades
  gracefully to raw IDs on failure.
- `commands` — wires the above together, parses flags.

`api` and `output` are unit-testable in isolation; `commands` is tested with a
mock API.

## 5. Command surface

Global syntax: `slk <group> <verb> [flags]`

### 5.1 Curated commands

| Group | Verbs | Slack API |
|-------|-------|-----------|
| `auth` | `login`, `set-token`, `status`, `logout`, `switch` | OAuth / local token store |
| `msg` | `read`, `send`, `update`, `delete`, `react`, `schedule` | `conversations.history`, `chat.*` |
| `thread` | `read`, `reply` | `conversations.replies`, `chat.postMessage` |
| `search` | `messages`, `channels`, `users` | `search.*`, `conversations.list`, `users.list` |
| `canvas` | `create`, `read`, `update` | `canvases.*`, `conversations.canvases.create` |
| `list` | `create`, `read`, `add-item`, `update-item` | `slackLists.*` |
| `channel` | `list`, `create`, `archive`, `invite`, `topic` | `conversations.*` |
| `user` | `list`, `info` | `users.*` |
| `api` | `<method>` | Escape hatch: any method, `--params` / `--json` passthrough |

DMs are not a separate group. `msg read` / `msg send` accept a target that may
be a channel ID, `#name`, `@user`, or user ID; the `resolve` layer disambiguates.

The `api` escape hatch uses the same auth as curated commands; it does not
bypass identity management.

### 5.2 Slack Lists API note

`slackLists.create`, `slackLists.items.create`, and `slackLists.items.list` are
officially available (released September 2025), so the `list` group is feasible.

## 6. Auth model

- **Profile** — `{ name, workspace, user_token?, bot_token?, oauth_creds? }`.
  Supports multiple workspaces and identities.
- **Config file** — `~/.config/slk/config.toml`, permissions `0600`. Tokens are
  never written elsewhere and never printed to stdout.
- **Identity selection** — when a profile has both tokens, `--as user|bot`
  selects; default is `user` (matches the "act as you" primary use case).
- **Two ways to obtain a token:**
  - `slk auth set-token --user xoxp-… --bot xoxb-…` — paste directly.
  - `slk auth login --client-id … --client-secret …` — user supplies their own
    Slack app's OAuth credentials; the CLI starts a local redirect server,
    completes OAuth, and writes the token back to the profile.
- **Environment override** — `SLK_TOKEN` (+ `SLK_PROFILE`) take precedence over
  the config file, for CI / agent container injection.

No interactive confirmation prompts (they would hang agent callers). Write
safety is handled by `--dry-run` (see Section 7) plus the calling agent's own
confirmation discipline.

## 7. Output design

The headline feature. Global `--format` flag with four modes:

| Format | Use | Example (reading a message) |
|--------|-----|------------------------------|
| `concise` (default) | Everyday agent reads, lowest token cost | `Bob: 新人進來前我們兩個先看 [05-19 20:12]` |
| `json` | Structured, machine-consumed | Trimmed object, meaningful fields only |
| `jsonl` | Paginated streaming, bulk data | One object per line |
| `table` | Human scanning | Aligned columns |

**Trimming rule** — `json` does not return the raw Slack response. It keeps only
meaningful fields: `user` (resolved to a name), `text`, `ts`, `thread` (if any),
`channel`. It drops `blocks`, `team`, `bot_profile`, `client_msg_id`, `edited`,
and similar noise.

**ID beautification** — the `resolve` layer maps `U…`/`C…` IDs to user/channel
names and caches results locally (`~/.config/slk/cache/`) to avoid repeated
`users.info` calls. On failure it degrades gracefully to the raw ID without
erroring. Enabled by default; `--no-resolve` disables it.

**`--raw`** — skips all trimming and beautification, returns the raw Slack API
response. The `api` command defaults to `--raw`; curated commands accept
`--raw` for debugging or to retrieve trimmed-out fields.

**Pagination** — `--limit N`, `--cursor <c>`; `--page-all` (auto-paginates with
`jsonl`, bounded by `--page-limit`).

**Write safety** — every write verb supports `--dry-run`, which validates
arguments and prints what would be sent without calling the API. This replaces
interactive confirmation.

## 8. Error handling

Slack API failures return `ok: false` + an `error` string. The CLI maps these to
exit codes so the calling agent can branch programmatically:

| Exit | Class | Handling |
|------|-------|----------|
| 0 | Success | — |
| 2 | Argument error | Missing flag / bad format; rejected locally before any API call |
| 3 | Auth error | `invalid_auth` / `token_expired`; prompts to re-run `auth login` |
| 4 | Not found | `channel_not_found`, etc. |
| 5 | Rate limited | Auto-retries with `Retry-After` backoff N times; exits 5 only if still failing |
| 1 | Other API / network error | — |

Errors always go to stderr. With `--format json`, an additional
`{"error":"...","code":N}` object is emitted; stdout stays clean for agent
parsing.

## 9. Testing

Per the project's verification requirement, every part has a defined approach.

- **Unit** — `output` formatters, `api` error mapping, `auth` token store, flag
  parsing. Table-driven Go tests.
- **Integration** — `httptest` mock Slack server with recorded/replayed
  fixtures, covering pagination, rate-limit backoff, and `ok:false` paths.
- **E2E** (manual / optional) — runs against a real test workspace, gated by an
  environment variable; CI skips it by default.

## 10. Distribution

- **GoReleaser** — cross-platform binaries (darwin/linux x amd64/arm64),
  published to GitHub Releases.
- **Homebrew tap.**
- **Companion Claude Code skill** — the repo ships a `slk` reference skill
  (modeled on the `gws-shared` SKILL.md) so agents learn the usage directly.
- **License** — MIT.

## 11. v1 deliverables

- `slk` binary.
- All command groups in Section 5.
- Four output formats.
- Both auth paths.
- `--dry-run` on all write verbs.
- Mock-based test suite.
- README.
- Companion Claude Code skill.
- GoReleaser configuration.
