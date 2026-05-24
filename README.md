# slk

> Agent-facing Slack CLI — token-efficient read, send, and manage for AI agents.

<!-- Status -->
[![CI](https://img.shields.io/github/actions/workflow/status/howar31/slk/ci.yml?branch=main&label=CI)](https://github.com/howar31/slk/actions/workflows/ci.yml)
[![Go 1.25+](https://img.shields.io/badge/go-1.25+-00ADD8.svg)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Conventional Commits](https://img.shields.io/badge/conventional%20commits-1.0.0-yellow)](https://www.conventionalcommits.org)
[![Dependabot](https://img.shields.io/badge/dependabot-enabled-025E8C?logo=dependabot)](.github/dependabot.yml)

<!-- Release & distribution -->
[![GitHub release](https://img.shields.io/github/v/release/howar31/slk)](https://github.com/howar31/slk/releases)
[![GitHub release downloads](https://img.shields.io/github/downloads/howar31/slk/total?label=release%20downloads)](https://github.com/howar31/slk/releases)
[![npm version](https://img.shields.io/npm/v/@howar31/slk)](https://www.npmjs.com/package/@howar31/slk)
[![npm downloads](https://img.shields.io/npm/dm/@howar31/slk?label=npm%20downloads)](https://www.npmjs.com/package/@howar31/slk)

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

- [Why slk?](#why-slk)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Authentication](#authentication)
- [Agent setup](#agent-setup)
- [Usage](#usage)
  - [Checking for updates](#checking-for-updates)
  - [Multi-line content](#multi-line-content)
  - [Drafts](#drafts)
  - [Slack Lists item shape](#slack-lists-item-shape)
  - [Escape hatch — `slk api`](#escape-hatch--slk-api)
- [Environment variables](#environment-variables)
- [Exit codes](#exit-codes)
- [Bot mode](#bot-mode)
- [Known Slack-side limitations](#known-slack-side-limitations)
- [Team setup](#team-setup)
- [Development](#development)
- [License](#license)
- [Disclaimer](#disclaimer)

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
slk auth set-token --profile work --user xoxp-...

# Read the last 5 messages of a channel
slk msg read --channel C0123456789 --limit 5

# Send a message
slk msg send --channel C0123456789 --text "hello"

# Read a canvas as Markdown
slk canvas read --id F0123456789
```

The token in the second Quick start command comes from the
[Authentication](#authentication) steps below.

## Authentication

`slk` authenticates with **your own** Slack app — it does not embed a client secret. Create the
app once (steps 1–2), then get a token into `slk` by **either** pasting one **or** running the
OAuth flow (step 3); both are first-class and either works for a single user.

### 1. Create your Slack app

Visit <https://api.slack.com/apps> → **Create New App**. Two paths:

- **From an app manifest** (recommended) — pick your workspace, paste the JSON manifest below
  into the editor (it opens in JSON; YAML also works), and create the app. It pre-fills every
  scope `slk` uses, so you can skip step 2 and go straight to installing.
- **From scratch** — pick a name and workspace, then add scopes by hand in step 2.

The manifest's `user:` list is every scope `slk`'s curated commands can use as a user token,
and the `bot:` list covers the same commands that also work with a bot token. Not every command
needs every scope; trim the list to the subset you actually run.

<!-- BEGIN GENERATED MANIFEST -->
```json
{
  "display_information": {
    "name": "slk"
  },
  "oauth_config": {
    "scopes": {
      "user": [
        "bookmarks:read",
        "bookmarks:write",
        "canvases:read",
        "canvases:write",
        "channels:history",
        "channels:read",
        "channels:write",
        "chat:write",
        "dnd:read",
        "dnd:write",
        "emoji:read",
        "files:read",
        "files:write",
        "groups:history",
        "groups:read",
        "groups:write",
        "im:history",
        "im:read",
        "im:write",
        "lists:read",
        "lists:write",
        "mpim:history",
        "mpim:read",
        "mpim:write",
        "pins:read",
        "pins:write",
        "reactions:read",
        "reactions:write",
        "search:read",
        "team:read",
        "usergroups:read",
        "usergroups:write",
        "users.profile:read",
        "users.profile:write",
        "users:read",
        "users:read.email",
        "users:write"
      ],
      "bot": [
        "bookmarks:read",
        "bookmarks:write",
        "canvases:read",
        "canvases:write",
        "channels:history",
        "channels:join",
        "channels:manage",
        "channels:read",
        "chat:write",
        "dnd:read",
        "emoji:read",
        "files:read",
        "files:write",
        "groups:history",
        "groups:read",
        "groups:write",
        "im:history",
        "im:read",
        "im:write",
        "lists:read",
        "lists:write",
        "mpim:history",
        "mpim:read",
        "mpim:write",
        "pins:read",
        "pins:write",
        "reactions:read",
        "reactions:write",
        "team:read",
        "usergroups:read",
        "usergroups:write",
        "users.profile:read",
        "users:read",
        "users:read.email",
        "users:write"
      ]
    }
  },
  "settings": {
    "org_deploy_enabled": false,
    "socket_mode_enabled": false,
    "token_rotation_enabled": false
  }
}
```
<!-- END GENERATED MANIFEST -->

### 2. Optional — Add or edit OAuth scopes (skip if you created from the manifest)

Under **OAuth & Permissions** → **User Token Scopes**, add scopes one at a time, choosing the
ones you need from the `user:` list above.

To change scopes on an app you already created, open **Features → App Manifest** in the app's
settings, edit the `user:` list there, and **Save Changes** — Slack applies the diff and
prompts you to reinstall if you added new scopes.

### 3. Get a token into `slk`

Two ways — pick one. Both store the same kind of per-user `xoxp-` token; the OAuth flow is also
how a team shares one app (see [Team setup](#team-setup)).

- **Method A — paste a token** (simplest): install the app, copy its token, paste it in. No
  client secret, no redirect URL.
- **Method B — OAuth flow (`slk auth login`)**: `slk` mints the token through your browser, so
  you never copy a raw token by hand.

#### Method A — paste a token

On the **OAuth & Permissions** page (the same one as step 2), scroll to the top and click
**Install to Workspace**. Approve the prompt; the **User OAuth Token** (`xoxp-…`) then appears
at the top of that page — copy it. Some workspaces require an admin to approve the install.

Then store it. Run it in a terminal with no arguments — `slk` asks for each field and hides the
token as you paste it, so nothing lands in your shell history:

```bash
slk auth set-token
```

It prompts for three things:

- **`Profile name [default]:`** — a label for this set of credentials, so you can keep more
  than one (e.g. `work`, `personal`) and switch between them with `slk auth switch <name>`.
  Press Enter to accept `default`.
- **`Paste user token (xoxp-, hidden):`** — the **User OAuth Token** you just copied.
  Input is hidden; paste it and press Enter.
- **`Paste bot token (xoxb-, optional, hidden):`** — only if you also use a bot token
  (`xoxb-`); most users press Enter to skip.

Scripting it instead? Pass values as flags (`slk auth set-token --help`), and use `--user -`
to read the token from stdin so it stays out of history:
`printf '%s' "$TOKEN" | slk auth set-token --profile work --user -`.

#### Method B — OAuth flow (`slk auth login`)

1. **OAuth & Permissions → Redirect URLs**: add `http://localhost:3000/callback` (the port
   `slk auth login` listens on; override with `--port`), and **Install to Workspace** if you
   have not already.
2. Copy the **Client ID** and **Client Secret** from **Basic Information → App Credentials**.
3. Run the flow:

```bash
slk auth login --client-id <id> --client-secret <secret>
```

`slk` prints an authorize URL and starts a local listener; open the URL, approve, and `slk`
exchanges the code for your own `xoxp-` token and stores it. The client secret is used only for
that exchange and is **never** persisted. Run with no arguments in a terminal to be prompted for
each field instead (the client secret is entered hidden).

Either way, the token lands in `~/.config/slk/config.toml` (mode `0600`) and is encrypted at
rest (see [Credential storage](#credential-storage)). `slk` never prints token contents.

### 4. Verify it worked

```bash
slk auth status   # lists profiles + verifies the active one live against Slack
slk auth test     # explicit live check of the active token
```

`slk auth status` lists each profile as `<name> (user=true bot=false)` plus the encryption
backend, and verifies the **active** profile live against Slack — appending its identity
(`<team> (<team_id>) — <user> (<user_id>) @ <url>`) on success, `(invalid token: …)` if Slack
rejects it, or `(offline: …)` if Slack is unreachable. Pass `--all` to verify every profile, or
`--offline` to skip the network and list local info only. It never prints the token itself.

`slk auth test` is the explicit single-token check: it calls Slack's `auth.test` and prints the
same identity line. A bad token returns an auth error (`invalid_auth` / `not_authed`) and exits
`3` — re-check the token and redo step 3.

### Credential storage

The `user_token` and `bot_token` fields are encrypted at rest with AES-256-GCM. Tokens you
add with `slk auth set-token` or `slk auth login` are encrypted on write; an existing
plaintext value (for example one you hand-edited into the file) keeps working and is encrypted
the next time `slk` writes the config — no re-authentication is ever required. `slk` does
**not** persist the OAuth `client_id` / `client_secret`: `auth login` uses them only
transiently for the token exchange, never writing them to the config.

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

`slk`'s agent skills (the `skills/` tree) are **generated from the CLI itself** —
every command, flag, the Slack method each one wraps, and a confirm-before-writing
caution on destructive verbs. The skill is split per command group: a small `slk`
index lists the groups and links to one `slk-<group>` skill each, alongside a shared
`slk-shared` reference — so an agent loads only the surface it needs instead of one
large file. Because they are generated and drift-guarded in CI, your agent always
sees accurate, in-sync docs instead of hand-written files that lag the code. Install
them with one command, or wire them up per agent below.

### Claude Code, Cursor, and other skill-aware agents

```bash
npx skills add https://github.com/howar31/slk
```

This installs slk's skills into your agent's skills directory (e.g.
`~/.claude/skills/`). Claude Code activates the matching skill automatically when a
task mentions Slack. Manual fallback (copy the whole tree):

```bash
cp -R ./skills/slk ./skills/slk-* ~/.claude/skills/
```

### Gemini CLI

```bash
gemini extensions install https://github.com/howar31/slk
```

Requires `slk` on your `$PATH` (install via Homebrew or npm first). The extension
loads the `slk` index skill; browse group-level detail with `slk <group> --help`.

### OpenClaw

OpenClaw reads the skills' frontmatter. Symlink the tree to stay in sync with the repo:
`ln -s $(pwd)/skills/slk* ~/.openclaw/skills/`. If the `slk` binary is missing, OpenClaw
auto-installs it from one of the `install:` specs in the `slk` index skill's metadata
(npm `@howar31/slk`, the Homebrew tap, or `go install`).

### Cursor / aider / others

Either reference `slk --help` from your agent's instruction file, or paste the contents of
the index skill [`skills/slk/SKILL.md`](skills/slk/SKILL.md) (which links to the per-group
skills) into the agent's persistent rules file (e.g., `.cursorrules`, `GEMINI.md`).

## Usage

The complete, always-current command reference is the CLI's own help. It shares a
single source with the binary — the same command tree the agent skill is generated
from — so it can't drift out of sync:

```bash
slk --help                 # all command groups
slk <group> --help         # a group's verbs   (e.g. slk msg --help)
slk <group> <verb> --help  # a verb's flags     (e.g. slk msg send --help)
```

Global flags such as `--format`, `--raw`, and `--dry-run` apply to every command;
`slk --help` lists them all. Agents get the same surface as a generated skill
([`skills/slk/SKILL.md`](skills/slk/SKILL.md)) — see [Agent setup](#agent-setup).

The rest of this section documents only what `--help` can't convey on its own —
usage traps, conceptual data shapes, and behavior notes:

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

## Bot mode

A bot token (`xoxb-`) is the **automation subset** of user mode, not a parallel replacement.
The write-heavy core — posting, scheduling, reactions, channel management, canvas, Slack Lists,
file upload, pins, bookmarks — works fully. Three areas degrade or vanish:

- ❌ **Not available under a bot:** `search messages` / `search files` / `search all` and
  `canvas list` (all rely on the Search API, which is user-token-only); `user set-profile` /
  `set-photo` / `delete-photo` (personal profile writes); `dnd snooze` / `end-snooze` / `end`
  (DND state is per-user); `file public` / `revoke-public`; `msg draft`. (`user set-presence`
  runs but has no effect for a bot.)
- ⚠️ **Reduced effect:** `msg read` and `thread read` only see channels the bot has joined
  (DM history is limited to the bot's own DMs); `msg update`, `msg delete`, and `file delete`
  only operate on the bot's own messages or files.
- ✅ **Everything else** works the same as user mode. The `bot:` scope list in the app manifest
  above is the live reference for what the bot token covers; per-verb flags are in `--help`.

**Getting a bot token — two paths:**

```bash
# OAuth flow — slk mints the xoxb- through your browser:
slk auth login --as bot --client-id <id> --client-secret <secret>

# Paste flow — copy the Bot User OAuth Token (xoxb-...) from the app's
# OAuth & Permissions page, then store it:
slk auth set-token --token xoxb-...
```

Both paths store the token in the same profile alongside your user token (if any).
`slk auth status` will show `bot=true` once it is in place.

## Known Slack-side limitations

These behaviors come from Slack itself, not from `slk`:

- **Scheduled messages within ~5 minutes of `post_at` may still fire after
  `chat.deleteScheduledMessage` returns `ok=true`.** Slack appears to lock the message into
  its delivery queue before firing; the cancel succeeds in the API but the message still
  posts. Empirically T+180 s is unreliable, T+600 s is reliable. For cancellable schedules,
  pick `--at` at least 5–10 minutes in the future.
- **`msg delete` on a self-DM returns `chat.delete: internal_error`.** Slack restricts API
  deletion of 1:1 DMs; use the Slack desktop or web UI.
- **Slack Lists have no whole-list delete API.** `slackLists.delete` returns `unknown_method`,
  so an entire List must be removed in the Slack UI. Individual rows can be removed with
  `list delete-item` (`slackLists.items.delete`).
- **`channel invite` cannot invite a channel's creator or any existing member.** Slack
  returns `cant_invite_self` / `already_in_channel`. `slk` surfaces the error verbatim.
- **`canvas read` cannot recover the original code-block language hint.** Slack's HTML
  download route drops the triple-backtick language identifier. Text content is preserved;
  the language tag is not.
- **`drafts.list` / `drafts.delete` / `drafts.update` require a Slack-client token type
  that is not available to OAuth user tokens.** Manage drafts in the Slack UI.

## Team setup

An advanced rollout for sharing one Slack app across a team. One app serves everyone — you do
**not** create an app per person. One person sets it up once; each teammate then authorizes it
and gets their **own** user token (`xoxp-`). Tokens are per-user (each carries that person's
identity and permissions), so never share a single token.

### One-time, by the app owner

1. Create the app and add scopes — [Authentication](#authentication) steps 1–2. The manifest's
   `user:` list is the set everyone gets: the OAuth consent screen is **all-or-nothing**
   (**Allow** / **Cancel**, with the scope checkboxes greyed out), so a teammate can't pick
   scopes in the browser. To grant fewer, a teammate narrows `--scopes` on the `slk auth login`
   command *before* authorizing (it can never exceed the app's set); to make scopes toggleable
   in the browser, mark them **optional** in the app settings / manifest.
2. **OAuth & Permissions → Redirect URLs**: add `http://localhost:3000/callback` (the port
   `slk auth login` listens on; override with `--port`).
3. **Manage Distribution → Activate Public Distribution**, so teammates who are *not* app
   collaborators can authorize it.
4. Copy the **Client ID** and **Client Secret** from **Basic Information → App Credentials**.

### Per teammate

Each teammate runs the OAuth flow with the shared app credentials to mint their own token:

```bash
slk auth login --client-id <id> --client-secret <secret>
```

This is exactly [Authentication](#authentication) **Method B** — it prints an authorize URL and
starts a local listener on `localhost:3000` (the `--port`). Open the printed URL in a browser and
approve; Slack redirects back to that listener with a code, which `slk` exchanges and stores as
the teammate's own `xoxp-` token (encrypted at rest). The listener waits ~5 minutes, then times
out. Verify with `slk auth status` / `slk auth test` — see [Authentication](#authentication)
step 4.

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

# Regenerate the agent skills after changing any command (CI enforces no drift)
go run ./cmd/slk generate-skills
```

Architecture, conventions, and design decisions live in [SPEC.md](SPEC.md).

## License

[MIT](LICENSE)

## Disclaimer

`slk` is not affiliated with or endorsed by Slack Technologies. "Slack" is a trademark of
Slack Technologies, LLC.
