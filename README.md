# slk

> Agent-facing Slack CLI — token-efficient read, send, and manage for AI agents.

<!-- Status -->
[![CI](https://img.shields.io/github/actions/workflow/status/howar31/slk/ci.yml?branch=main&label=CI)](https://github.com/howar31/slk/actions/workflows/ci.yml)
[![Go 1.25+](https://img.shields.io/badge/go-1.25+-00ADD8.svg)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

<!-- Release & distribution -->
[![GitHub release](https://img.shields.io/github/v/release/howar31/slk)](https://github.com/howar31/slk/releases)
[![npm version](https://img.shields.io/npm/v/@howar31/slk)](https://www.npmjs.com/package/@howar31/slk)
[![Downloads](https://img.shields.io/github/downloads/howar31/slk/total)](https://github.com/howar31/slk/releases)

<!-- Project standards -->
[![Conventional Commits](https://img.shields.io/badge/conventional%20commits-1.0.0-yellow)](https://www.conventionalcommits.org)
[![Dependabot](https://img.shields.io/badge/dependabot-enabled-025E8C?logo=dependabot)](.github/dependabot.yml)

<!-- Activity & community -->
[![Last commit](https://img.shields.io/github/last-commit/howar31/slk)](https://github.com/howar31/slk/commits/main)
[![Open issues](https://img.shields.io/github/issues/howar31/slk)](https://github.com/howar31/slk/issues)
[![Stars](https://img.shields.io/github/stars/howar31/slk)](https://github.com/howar31/slk/stargazers)
[![Sponsor on Ko-fi](https://img.shields.io/badge/sponsor-Ko--fi-FF5E5B?logo=ko-fi&logoColor=white)](https://ko-fi.com/howar31)

`slk` is a single static Go binary for the Slack Web API, designed for AI agents and the
humans they collaborate with. Compared to the official Slack MCP connector — which returns
verbose, fixed-shape JSON envelopes — `slk` emits curated, low-token output by default and
exposes `--raw` when a caller wants full API responses.

## Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Why slk?](#why-slk)
- [Authentication](#authentication)
- [Agent setup](#agent-setup)
- [Usage](#usage)
  - [Global flags](#global-flags)
  - [Checking for updates](#checking-for-updates)
  - [Multi-line content](#multi-line-content)
  - [Drafts](#drafts)
  - [Slack Lists item shape](#slack-lists-item-shape)
  - [Escape hatch — `slk api`](#escape-hatch--slk-api)
- [Environment variables](#environment-variables)
- [Exit codes](#exit-codes)
- [Known Slack-side limitations](#known-slack-side-limitations)
- [Development](#development)
- [License](#license)
- [Disclaimer](#disclaimer)

## Prerequisites

- Go 1.25+ (only required if you install from source).
- A Slack workspace where you can create your own Slack app. `slk` uses your own OAuth
  credentials; it never embeds a client secret in the binary.
- A Slack user OAuth token (`xoxp-…`) with the scopes for the commands you plan to use. See
  [Authentication](#authentication).

## Installation

### Homebrew (macOS / Linux)

```bash
brew install howar31/tap/slk
```

Adds the `howar31/homebrew-tap` formula automatically on first install.

### npm (anywhere Node.js 18+ runs)

```bash
npm install -g @howar31/slk
```

The package is scoped (`@howar31/slk`) because the unscoped name `slk` is already taken on
npm. `postinstall` downloads the matching prebuilt binary from GitHub Releases and verifies
its SHA256 checksum.

### Pre-built binary

Download from [GitHub Releases](https://github.com/howar31/slk/releases).
Replace `<os>` with `darwin` or `linux`, and `<arch>` with `amd64` or `arm64`.
The `latest/download/` path always resolves to the newest release — no version to update.

```bash
curl -sLO https://github.com/howar31/slk/releases/latest/download/slk_<os>_<arch>.tar.gz
curl -sLO https://github.com/howar31/slk/releases/latest/download/checksums.txt
shasum -a 256 -c checksums.txt --ignore-missing
tar xzf slk_<os>_<arch>.tar.gz
sudo mv slk /usr/local/bin/
slk --version
```

### `go install` (Go 1.25+)

```bash
go install github.com/howar31/slk/cmd/slk@latest
```

Places `slk` in `$GOBIN` (typically `$HOME/go/bin`); ensure `$GOBIN` is on your `$PATH`.

### From source

```bash
git clone https://github.com/howar31/slk
cd slk
go build -o slk ./cmd/slk   # version is read from the committed VERSION file
```

## Quick start

```bash
# Confirm the install
slk --version

# Store your token (see Authentication for how to obtain one)
slk auth set-token --profile work --workspace acme --user xoxp-...

# Read the last 5 messages of a channel
slk msg read --channel C0123456789 --limit 5

# Send a message
slk msg send --channel C0123456789 --text "hello"

# Read a canvas as Markdown
slk canvas read --id F0123456789
```

## Why slk?

`slk` is built for agent workflows that talk to Slack frequently. Compared to the official
Slack MCP connector, it is meaningfully cheaper per round-trip:

| Operation | MCP response | `slk` default response |
|---|---|---|
| Send message | 200–400 tokens (full message object) | ~7 tokens (`sent <ts>`) |
| Read 3 messages | 1000–2000 tokens (full metadata each) | ~150 tokens (concise JSON) |
| Search users | 500+ tokens per user (profile + avatars + tz) | ~20 tokens per user (`name id extra`) |
| Delete a message | Full envelope echo | ~2 tokens (`delete ok`) |

Why the gap:

1. **No persistent tool schema.** `slk` is invoked via `Bash`; the agent does not carry the
   13 MCP tool schemas (~5–10 K tokens) in its context.
2. **Curated by default.** Every read command renders a concise summary; `--format json` and
   `--raw` are opt-in for full structure.
3. **Default ID resolution.** `slk` maps `U…` / `C…` to human-readable names so the model
   does not need a second round-trip to interpret IDs.
4. **Shell composability.** Pipe through `jq`, `head`, `grep` to filter bytes before they
   reach the model.
5. **Opt-in detail.** `--raw` and `--format json` return the full envelope only when asked;
   MCP returns it every call.

When MCP is still the better choice:

- The agent cannot execute a shell at all.
- You need strict JSON-schema contracts for tool-calling integration.

In typical agent workflows the savings compound: roughly **5–20× cheaper per call** and
**2–5× cheaper across a full session**, depending on how much of the traffic is short
confirmations and ID-resolved reads — the regime `slk` is designed to shine in.

## Authentication

`slk` walks the standard Slack OAuth flow using **your own** Slack app — slk does not embed a
client secret.

### 1. Create your Slack app

Visit <https://api.slack.com/apps> → **Create New App** → **From scratch**. Pick a workspace.

### 2. Add OAuth scopes

Under **OAuth & Permissions** → **User Token Scopes**, add the scopes you need. The full set
used by `slk`'s curated commands:

```
channels:history channels:read channels:write
groups:history   groups:read   groups:write
im:history       im:read       im:write
mpim:history     mpim:read     mpim:write
chat:write       reactions:write
search:read
users:read       users.profile:read
files:read
canvases:read    canvases:write
lists:read       lists:write
```

You can paste an equivalent **App Manifest** into the same UI to add them in one shot.

### 3. Install to your workspace, copy the token

Click **Install to \<Workspace\>**. After approval, copy the **User OAuth Token** (begins with
`xoxp-`). Some workspaces require an admin to approve the install.

### 4. Store the token in `slk`

```bash
# Paste a token directly
slk auth set-token --profile work --workspace acme --user xoxp-...

# Or run the OAuth flow (requires your own client_id / client_secret)
slk auth login --profile work --client-id ... --client-secret ...
```

Tokens land in `~/.config/slk/config.toml` (mode `0600`) and are encrypted at rest (see
[Credential storage](#credential-storage)). `slk` never prints token contents; `slk auth
status` shows presence booleans only.

### Credential storage

The `user_token`, `bot_token`, and `client_secret` fields are encrypted at rest with
AES-256-GCM. Tokens you add with `slk auth set-token` or `slk auth login` are encrypted on
write; an existing plaintext value (for example one you hand-edited into the file) keeps
working and is encrypted the next time `slk` writes the config — no re-authentication is ever
required.

The 32-byte encryption key is held in one of two backends, selected by the
`SLK_KEYRING_BACKEND` environment variable:

- `auto` (default) — use the OS keyring if one is available, otherwise fall back to a key
  file. This keeps `slk` usable in headless / CI / agent environments with no interactive
  keyring.
- `keyring` — always use the OS keyring (macOS Keychain, Linux Secret Service, Windows
  Credential Manager). Strongest protection; may prompt to unlock.
- `file` — store the key in `~/.config/slk/.encryption_key` (mode `0600`).

The backend actually used is recorded in the config so reads stay deterministic. `slk auth
status` shows the backend and whether decryption is healthy, never any secret.

**What this protects against:** accidental disclosure — a token no longer sits in the config
file as readable text, so it will not leak through a casual `cat`, screen-sharing, dotfile
sync, or backups. The `file` backend keeps the key next to the config, so it is **not** a
defense against someone who can already read your `~/.config/slk/` directory; for real
local-attacker protection use the `keyring` backend.

### Precedence

Active credential resolution, highest precedence first:

1. `SLK_TOKEN` environment variable.
2. `--profile <name>` command-line flag.
3. `SLK_PROFILE` environment variable.
4. The `active` profile in the config file.

## Agent setup

`slk`'s agent skill (`skills/slk/SKILL.md`) is **generated from the CLI itself** —
every command, flag, the Slack method each one wraps, and a confirm-before-writing
caution on destructive verbs. Because it is generated and drift-guarded in CI, your
agent always sees accurate, in-sync docs instead of a hand-written file that lags the
code. Install it with one command, or wire it up per agent below.

### Claude Code, Cursor, and other skill-aware agents

```bash
npx skills add https://github.com/howar31/slk
```

This installs `skills/slk/SKILL.md` into your agent's skills directory (e.g.
`~/.claude/skills/`). Claude Code activates the skill automatically when a task
mentions Slack. Manual fallback:

```bash
mkdir -p ~/.claude/skills/slk
cp ./skills/slk/SKILL.md ~/.claude/skills/slk/SKILL.md
```

### Gemini CLI

```bash
gemini extensions install https://github.com/howar31/slk
```

Requires `slk` on your `$PATH` (install via Homebrew or npm first).

### OpenClaw

OpenClaw reads `skills/slk/SKILL.md`'s frontmatter. Symlink it to stay in sync with the repo:
`ln -s $(pwd)/skills/slk ~/.openclaw/skills/`. If the `slk` binary is missing, OpenClaw
auto-installs it from one of the `install:` specs in the skill metadata (npm `@howar31/slk`,
the Homebrew tap, or `go install`).

### Cursor / aider / others

Either reference `slk --help` from your agent's instruction file, or paste the contents of
[`skills/slk/SKILL.md`](skills/slk/SKILL.md) into the agent's persistent rules file (e.g.,
`.cursorrules`, `GEMINI.md`).

## Usage

```bash
slk msg read    --channel C0123456789 --limit 20
slk msg send    --channel C0123456789 --text "hello"
slk msg send    --channel C0123456789 --thread 1700000000.000000 --text "in-thread"
slk thread reply --channel C0123456789 --thread 1700000000.000000 --text "…"
slk search channels
slk canvas create --title "Plan" --markdown "# Heading"
slk canvas read   --id F0123456789
slk list create   --title "Backlog"
slk channel archive --channel C0123456789
slk api conversations.info --params '{"channel":"C0123456789"}'
slk version                # print the running version (offline)
slk version --check        # check GitHub Releases for a newer version (read-only)
```

### Global flags

| Flag | Description |
|---|---|
| `--format concise\|json\|jsonl\|table` | Output format (default `concise`). |
| `--as user\|bot` | Identity selection when a profile has both tokens (default `user`). |
| `--profile <name>` | Use a specific profile from the config file. |
| `--raw` | Return the raw Slack API response, skipping concise rendering. |
| `--dry-run` | Validate locally and print what would be sent; do not call the API. |
| `--no-resolve` | Skip ID-to-name resolution (faster, less readable). |

### Checking for updates

```bash
slk version          # print the running version (fully offline)
slk version --check  # ask GitHub Releases whether a newer version exists
```

`slk version --check` makes a single read-only request to the GitHub Releases
API and reports whether a newer version is available, plus the command to
upgrade. It **never downloads or replaces the binary** — upgrading stays your
package manager's job (`brew upgrade slk`, `npm i -g @howar31/slk@latest`, or a
fresh download from the [Releases](https://github.com/howar31/slk/releases)
page).

The check is best-effort: if GitHub is unreachable or rate-limited, slk prints
your current version with a note and still exits `0`. Pass `--format json` for
machine-readable output — agents can read the `update_available` and `checked`
fields. A locally built binary reports the version from the committed `VERSION`
file — the same value a released build embeds.

Plain `slk version` and `slk --version` perform no network I/O.

### Multi-line content

Bash double-quoted `"\n"` is a literal backslash-n, not a newline. To pass real multi-line
content, use the file or stdin alternative:

```bash
slk canvas create --title "Weekly" --markdown-file weekly.md
cat weekly.md | slk canvas update --id F0123456789 --action prepend --markdown-file -
```

`--markdown-file` is available on `canvas create` / `canvas update`. `--text-file` is
available on `msg send` / `msg draft` / `msg schedule` / `msg update` / `thread reply`. Use
`-` as the path to read from stdin.

### Drafts

`msg draft` creates a draft via Slack's `drafts.create` endpoint. The companion lifecycle
endpoints (`drafts.list` / `drafts.delete` / `drafts.update`) require Slack-client token types
that are not available to OAuth user tokens; use the Slack desktop or web client's
**Drafts & Sent** panel to list, edit, or delete drafts. The URL emitted by `msg draft` opens
the channel where the draft lives.

### Slack Lists item shape

Slack's Lists API uses rich-text blocks even for plain-text columns:

```bash
# Find the column ID once
slk api slackLists.create --params '{"name":"my list"}'
# → { "list_id": "F0…", "list_metadata": { "schema": [ { "id": "Col0…", … } ] } }

# Add an item
slk list add-item --id F0… --fields '[
  {
    "column_id": "Col0…",
    "rich_text": [{
      "type": "rich_text",
      "elements": [{
        "type": "rich_text_section",
        "elements": [{"type": "text", "text": "hello"}]
      }]
    }]
  }
]'

# Update a cell — slk auto-fills row_id into any cell that omits it.
slk list update-item --id F0… --row-id Rec0… --fields '[
  {
    "column_id": "Col0…",
    "rich_text": [{
      "type": "rich_text",
      "elements": [{
        "type": "rich_text_section",
        "elements": [{"type": "text", "text": "updated"}]
      }]
    }]
  }
]'
```

To update cells across multiple rows in one call, put `row_id` inside each cell; `--row-id`
becomes the fallback for cells that omit it.

### Escape hatch — `slk api`

`slk api <method>` invokes any Slack Web API method. Nested objects in `--params` must be
pre-serialized JSON strings (Slack's Web API uses form-urlencoded transport):

```bash
# WRONG — criteria is a nested object literal
slk api canvases.sections.lookup \
  --params '{"canvas_id":"F0…","criteria":{"section_types":["any_header"]}}'

# RIGHT — criteria is a JSON-encoded string
slk api canvases.sections.lookup \
  --params '{"canvas_id":"F0…","criteria":"{\"section_types\":[\"any_header\"]}"}'
```

## Environment variables

| Variable | Purpose |
|---|---|
| `SLK_TOKEN` | Token override. Highest precedence — bypasses the config file entirely. |
| `SLK_PROFILE` | Active profile name. Used when `--profile` is not passed. |
| `SLK_CONFIG` | Config file path override. Default: `~/.config/slk/config.toml`. |
| `SLK_KEYRING_BACKEND` | At-rest encryption-key backend: `auto` (default), `keyring`, or `file`. See [Credential storage](#credential-storage). |

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success. |
| `1` | Other error (network, JSON parse, miscellaneous). |
| `2` | Reserved (Cobra usage errors currently still map to `1`; v1.2). |
| `3` | Auth error (`invalid_auth`, `token_expired`, `not_authed`, missing profile). |
| `4` | Not found (`channel_not_found`, `user_not_found`, `message_not_found`, …). |
| `5` | Rate limited after the retry budget was exhausted. |

## Known Slack-side limitations

These behaviors come from Slack itself, not from `slk`:

- **Scheduled messages within ~5 minutes of `post_at` may still fire after
  `chat.deleteScheduledMessage` returns `ok=true`.** Slack appears to lock the message into
  its delivery queue before firing; the cancel succeeds in the API but the message still
  posts. Empirically T+180 s is unreliable, T+600 s is reliable. For cancellable schedules,
  pick `--at` at least 5–10 minutes in the future.
- **`msg delete` on a self-DM returns `chat.delete: internal_error`.** Slack restricts API
  deletion of 1:1 DMs; use the Slack desktop or web UI.
- **Slack Lists have no public delete API.** `slackLists.delete` returns `unknown_method`,
  and `files.delete` requires the `files:write` scope which is not in `slk`'s default set.
  Delete lists in the Slack UI.
- **`channel invite` cannot invite a channel's creator or any existing member.** Slack
  returns `cant_invite_self` / `already_in_channel`. `slk` surfaces the error verbatim.
- **`canvas read` cannot recover the original code-block language hint.** Slack's HTML
  download route drops the triple-backtick language identifier. Text content is preserved;
  the language tag is not.
- **`drafts.list` / `drafts.delete` / `drafts.update` require a Slack-client token type
  that is not available to OAuth user tokens.** Manage drafts in the Slack UI.

## Development

```bash
# Build (version is read from the committed VERSION file)
go build -o slk ./cmd/slk

# Run the full test suite (uncached)
go clean -testcache && go test ./...

# Coverage snapshot
go test ./... -coverpkg=./... -coverprofile=/tmp/slk.cov >/dev/null
go tool cover -func=/tmp/slk.cov | tail -1

# A single test
go test ./internal/commands/ -run TestInjectRowID -v

# Regenerate the agent skill after changing any command (CI enforces no drift)
go run ./cmd/slk generate-skill
```

Architecture, conventions, and design decisions live in [SPEC.md](SPEC.md).

## License

[MIT](LICENSE)

## Disclaimer

`slk` is not affiliated with or endorsed by Slack Technologies. "Slack" is a trademark of
Slack Technologies, LLC.
