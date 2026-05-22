# slk — SPEC

## Purpose

`slk` is an agent-facing command-line client for the Slack Web API. It
exists so AI coding agents (and the humans they collaborate with) can
read, send, and manage Slack content from a shell without dragging the
official Slack MCP connector's verbose JSON envelopes into the model's
context window. The CLI emits curated, low-token output by default and
exposes `--raw` for callers that want full API responses.

Primary consumer: a developer agent (e.g. Claude Code) running on
macOS or Linux, authenticated against the operator's own Slack app via
OAuth user tokens (`xoxp-…`). slk replaces the 13 Slack MCP tools the
operator has connected, with 1:1 capability parity.

## Architecture

`slk` is a single static Go binary. The process:

1. Cobra builds the command tree (`internal/commands.NewRootCommand`).
2. The root command binds global flags via `internal/commands.GlobalFlags`
   (`--format`, `--profile`, `--raw`, `--dry-run`, `--no-resolve`,
   `--as`).
3. Each subcommand resolves a token through `internal/auth`
   (env override → named profile → default profile) and builds an
   `internal/api.Client` pointed at `https://slack.com/api`.
4. The client serializes form-urlencoded params, performs the HTTP call,
   maps `ok=false` responses into `*api.APIError`, retries on 429
   (`Retry-After`) up to a small bound, and returns raw JSON.
5. Read commands deserialize into compact item types, optionally pass
   IDs through `internal/resolve` (cached `users.info` lookups), and
   emit via `internal/output.Emit` in `concise|json|jsonl|table`.
6. Write commands print a one-line success summary unless `--raw` is
   set, in which case the raw JSON envelope is emitted.
7. Errors with structured exit-code mappings (`*api.APIError`,
   `*auth.AuthError`) are unwrapped in `cmd/slk/main.go` so each error
   class produces a stable process exit code.

The escape hatch `slk api <method> --params '<json>'` lets callers
invoke any Slack Web method bypassing curation; nested object values
must be pre-serialized as JSON strings (form-urlencoded transport).

Canvas reading is special: there is no public Slack endpoint that
returns canvas content as markdown. `canvas read` calls `files.info`,
authenticates a GET against `url_private_download`, and converts the
returned quip HTML to Markdown via `internal/quip` — including
non-standard `<lnk>`, code blocks (`class="prettyprint"`), checklists
(`data-section-style='7'`), and table-cell section IDs.

Update checking is special too: `slk version --check` does not touch
Slack and needs no token. It performs a single read-only GET to the
GitHub Releases API (`/repos/howar31/slk/releases/latest`, with a
`User-Agent` header), compares the parsed `tag_name` against the
embedded `VERSION`, and reports whether a newer release exists. It never downloads
or replaces the binary. Plain `slk version` / `slk --version` stay
fully offline.

The agent skill is a generated artifact, not a hand-maintained file. `slk
generate-skill` (a hidden command) renders `skills/slk/SKILL.md` from the live
Cobra command tree plus an embedded preamble template (`internal/skillgen`), so
the code, `slk --help`, and the skill share one source and cannot drift. Each
command carries `Annotations["slackMethod"]` (the Slack method it wraps) and, for
mutating commands, `Annotations["write"]="true"`; the generator emits these as a
per-command `**Slack API:**` line and a write `CAUTION` callout, and renders flag
tables (`Required` from `MarkFlagRequired`, `Default` from the flag default). A
CI job regenerates the skill and fails on any diff.

The binary version is single-sourced: a committed `VERSION` file at the repo root
is embedded via `//go:embed` (root `package slk`, `version.go` → `slk.Version`)
and feeds `slk --version`, the skill's `metadata.version`, npm, and the release.
There is no build-time `-ldflags` version injection.

External dependencies are intentionally narrow:

- `github.com/spf13/cobra` (+ `pflag`) for the command tree
- `github.com/BurntSushi/toml` for config persistence
- `golang.org/x/net/html` for the canvas converter
- `github.com/zalando/go-keyring` for the opt-in OS-keyring backend
  that holds the at-rest encryption key
- `crypto/aes` + `crypto/cipher` (AES-256-GCM) for credential
  encryption at rest
- `crypto/rand` for the AES key, GCM nonces, and draft `client_msg_id`
  UUIDs
- standard library for everything else (HTTP, JSON, files)

## Layout

```
.
├── VERSION                     # version source of truth (embedded via go:embed)
├── version.go                  # package slk: //go:embed VERSION → slk.Version
├── cmd/slk/main.go             # process entry; error→exit-code wiring; uses slk.Version
├── internal/
│   ├── api/                    # HTTP client, error mapping, paginator
│   │   ├── client.go           # api.Client; BaseURL overridable for tests
│   │   ├── errors.go           # APIError + ExitCodeFor() mapping
│   │   └── paginate.go         # CallAll: walks next_cursor up to N pages
│   ├── auth/                   # credential management
│   │   ├── store.go            # TOML config at ~/.config/slk/config.toml
│   │   ├── crypto.go           # AES-256-GCM field encrypt/decrypt
│   │   ├── keyprovider.go      # key file / OS-keyring backends
│   │   ├── status.go           # EncryptionStatus line for `auth status`
│   │   ├── token.go            # ResolveToken precedence
│   │   ├── oauth.go            # local-callback OAuth flow
│   │   └── errors.go           # AuthError → exit code 3
│   ├── commands/               # Cobra command tree (one file per group)
│   │   ├── root.go             # NewRootCommand + global flag binding
│   │   ├── context.go          # GlobalFlags struct
│   │   ├── clientutil.go       # buildClient(g) → *api.Client
│   │   ├── input.go            # readContent: --text vs --text-file/stdin
│   │   ├── lookup.go           # slackLookup + resolveUser wiring
│   │   ├── api.go              # escape-hatch `slk api`
│   │   ├── auth.go             # set-token / status / switch / logout / login
│   │   ├── msg.go              # send / read / update / delete / react /
│   │   │                       # schedule / draft + slackMessage helper
│   │   ├── thread.go           # read / reply (uses --thread on both)
│   │   ├── canvas.go           # create / read / update / list
│   │   ├── channel.go          # create / archive / invite / topic / list
│   │   ├── list.go             # Slack Lists: create / read / add-item /
│   │   │                       # update-item; injectRowID helper
│   │   ├── user.go             # info / profile / list (default-filter)
│   │   ├── search.go           # messages / channels / users
│   │   ├── version.go          # version + --check GitHub-release probe
│   │   └── generateskill.go    # hidden `generate-skill`: writes skills/slk/SKILL.md
│   ├── skillgen/               # SKILL.md generator
│   │   ├── skillgen.go         # Generate(tree, version) → markdown
│   │   └── skill.md.tmpl       # embedded preamble + frontmatter template
│   ├── output/                 # concise|json|jsonl|table renderers
│   ├── quip/                   # canvas HTML→Markdown converter
│   │   ├── convert.go
│   │   └── testdata/           # canvas_fixture.{html,md} golden file
│   └── resolve/                # ID→name cache (~/.config/slk/cache)
├── skills/slk/SKILL.md         # GENERATED agent skill — do not hand-edit
├── npm/                        # npm wrapper (postinstall downloads the binary)
│   ├── package.json            # bin.slk=run.js, postinstall=install.js
│   ├── install.js              # download tarball + verify checksum
│   ├── platform.js             # os/arch → supportedPlatforms key
│   └── run.js                  # re-exec bin/slk (install if missing)
├── gemini-extension.json       # Gemini CLI extension → contextFileName skills/slk/SKILL.md
├── docs/superpowers/           # design specs + implementation plans
├── .github/
│   ├── workflows/
│   │   ├── ci.yml              # PR + push-main: gofmt, vet, build, test, goreleaser check, skill drift
│   │   └── release.yml         # VERSION-driven release: gate → tag → goreleaser + npm + homebrew
│   ├── dependabot.yml          # security-only updates (routine version bumps disabled)
│   └── release.yml             # GitHub release-notes categorization by PR label
├── .goreleaser.yaml            # darwin/linux × amd64/arm64 + homebrew tap + github-native changelog
├── go.mod / go.sum
├── README.md
├── SECURITY.md                 # vulnerability reporting + token policy
└── LICENSE                     # MIT
```

## Conventions

- **Module path**: `github.com/howar31/slk`. Go 1.25.
- **Internal-only**: all packages other than `cmd/slk` live under
  `internal/` — no public Go API surface is offered.
- **One file per command group** in `internal/commands/`, paired with a
  `_test.go` containing dry-run + flag-registration checks.
- **Comments are English**. Test names use `Test<Subject>_<Behavior>`.
- **Exit codes**: `0` ok · `1` other · `2` (currently unused; reserved
  for Cobra usage errors) · `3` auth · `4` not found · `5` rate-limited.
  Mappings live in `internal/api/errors.go::ExitCodeFor`.
- **Resolver default**: `--no-resolve` short-circuits to `nil` so user
  ID rendering is opt-out, not opt-in.
- **Concise output**: types implement `output.Concise` (a one-line
  `Concise() string`); `output.Emit` dispatches by `--format`.
- **Write-verb output contract**: every write verb prints a short
  human line on success unless `--raw` is set, in which case the raw
  Slack JSON envelope is emitted.
- **Multi-line content**: write verbs accept either `--markdown` /
  `--text` (inline, no shell newlines) or `--markdown-file` /
  `--text-file` (`-` for stdin) — both forms are mutually exclusive at
  runtime via `readContent`.
- **Version source of truth**: the root `VERSION` file is embedded via
  `//go:embed` (`version.go`); `slk --version`, the skill's
  `metadata.version`, npm, and the release all derive from it. No
  `-ldflags` version injection.
- **The skill is generated**: `skills/slk/SKILL.md` is produced by `slk
  generate-skill` from the Cobra tree — never hand-edit it. Per-command
  `Annotations["slackMethod"]` and `Annotations["write"]` drive the
  generated Slack-method line and the write `CAUTION`; CI fails if the
  committed skill drifts from the generator.
- **README scope**: the README is human onboarding plus knowledge `slk
  --help` cannot convey (usage traps, data shapes, behavior notes). The
  complete, in-sync command reference is `slk --help` and the generated
  `SKILL.md`; do not reintroduce a per-command example list or a
  global-flags table in the README — it would be a partial, drift-prone
  mirror of the command tree, contradicting the generated-skill SSOT.

## Verification

- Build: `go build -o slk ./cmd/slk` (version is read from the embedded `VERSION` file)
- Test: `go test ./...` (no external services touched)
- Forced refresh: `go clean -testcache && go test ./...`
- CI (`.github/workflows/ci.yml`): on every PR and push to `main` (code
  paths only — `**.md`, `docs/**`, `LICENSE`, `.gitignore` are ignored),
  GitHub Actions runs a gofmt check, `go vet`, `go build ./...`,
  `go test ./...`, and `goreleaser check` on Go 1.25, plus a `skill` job
  that regenerates `skills/slk/SKILL.md` and fails on any `git diff`
  (the drift guard).
- Coverage snapshot: `go test ./... -coverpkg=./...
  -coverprofile=/tmp/slk.cov && go tool cover -func=/tmp/slk.cov`

Each package carries focused tests:

| Package | Coverage focus |
|---|---|
| `internal/api` | error mapping, paginator cursor handling, retry on 429 |
| `internal/auth` | TOML load/save, token-precedence matrix, OAuth callback |
| `internal/commands` | dry-run output, flag registration, response-shape parsing helpers |
| `internal/output` | each render branch + unknown-format error path |
| `internal/quip` | per-element-family unit tests + full-fixture golden diff |
| `internal/resolve` | cache hit, cross-instance disk persistence, graceful degradation |

Command tests stay at dry-run + flag-registration depth because
`buildClient` reaches real config; helpers that parse Slack responses
are exposed as small functions (`parseListCreateID`,
`parseScheduledMessageID`, `parseListItems`, `messageDisplay`,
`injectRowID`, `fetchChannelsWith`, `fetchUsersWith`) and unit-tested
in isolation against `httptest.NewServer`-backed `api.Client` instances.

End-to-end coverage against a live Slack workspace is run manually
when changes touch write paths. The matrix and findings are recorded
in the session transcript or handoff document that performed the run,
not committed to the repo (the live workspace identifiers belong
outside the public tree).

## Deploy

Releases are **VERSION-driven**, not tag-driven. Bump the `VERSION` file (and
regenerate the skill) in a release PR; on merge to `main`,
`.github/workflows/release.yml` runs. A `gate` job derives `v<VERSION>` and skips
if that tag already exists; otherwise the `goreleaser` job creates and pushes the
tag and releases in the same run (the tag is an artifact of the release, not its
trigger — no PAT needed). The release:

1. Runs `goreleaser release --clean` on ubuntu-latest, producing
   `slk_<os>_<arch>.tar.gz` for `darwin/linux × amd64/arm64` plus
   `checksums.txt`.
2. Creates the GitHub Release with the artifacts (`release.prerelease:
   auto` marks versions containing `-`, e.g. `v0.1.0-rc1`, as prereleases).
   Release notes use goreleaser `changelog: use: github-native`, so they follow
   `.github/release.yml`'s PR-label categories.
3. Generates SLSA build provenance attestations via
   `actions/attest-build-provenance@v4`.
4. Pushes a Homebrew formula update to `howar31/homebrew-tap` (uses the
   `HOMEBREW_TAP_TOKEN` PAT). Prereleases skip the formula push via
   goreleaser's `brews.skip_upload: auto` (the npm-only prerelease guard
   in `release.yml` does not cover brews).
5. For non-prerelease versions, runs a `publish-npm` job that bumps
   `npm/package.json`'s version to match `VERSION` and publishes
   `@howar31/slk` (uses `NPM_TOKEN`). Prereleases skip the npm publish.

Both release secrets expire and must be rotated, or the corresponding
step fails on the next release:

- `NPM_TOKEN` — npm granular token, **max 90-day** expiry (npm's hard
  limit). Regenerate at npmjs.com (Read/write on `@howar31`) and
  re-run `gh secret set NPM_TOKEN --repo howar31/slk`.
- `HOMEBREW_TAP_TOKEN` — GitHub fine-grained PAT, up to ~1-year expiry,
  scope **Contents: Read and write** on `howar31/homebrew-tap` only.

Dependency and release-notes automation:

- `.github/dependabot.yml` disables routine version-bump PRs
  (`open-pull-requests-limit: 0` for both `gomod` and `github-actions`);
  the ecosystem blocks are kept only so repository-level Dependabot
  security PRs inherit the `chore`-scope commit formatting. Dependabot
  alerts and automated security fixes are enabled at the repo level, so
  PRs for vulnerable dependencies are still opened; non-security bumps
  are made manually.
- `.github/release.yml` categorizes the auto-generated GitHub Release
  notes by PR label (Features / Fixes / Documentation / Dependencies /
  Maintenance / Other).
- Repo merge policy: merge commits disabled — squash and rebase only,
  for linear history; head branches auto-delete on merge.

Security reporting: `SECURITY.md` directs vulnerability reports to
GitHub private advisories (private vulnerability reporting is enabled);
it reiterates that tokens live in `~/.config/slk/config.toml` (`0600`)
and are never printed or logged.

The `npm/` directory is a thin postinstall-driven wrapper:

| File | Responsibility |
|---|---|
| `package.json` | Declares `bin.slk = run.js`, `scripts.postinstall = install.js`, and `supportedPlatforms` (4 darwin/linux × arm64/x64 entries). Version is overwritten by CI at publish time. |
| `install.js` | Downloads the matching tarball + `checksums.txt` from GitHub Releases, verifies SHA256, extracts to `bin/`. |
| `platform.js` | Maps `os.type()`/`os.arch()` to a `supportedPlatforms` key. |
| `run.js` | Re-execs `bin/slk`; triggers `install.js` if the binary is missing (e.g. when the user ran `npm install --ignore-scripts`). |

Runtime state:

- OAuth tokens live in `~/.config/slk/config.toml` (mode `0600`,
  TOML-encoded, multi-profile). The `user_token`, `bot_token`, and
  `client_secret` fields are encrypted at rest with AES-256-GCM
  (`enc:v1:` prefix). The 32-byte key is held in the OS keyring or, for
  headless/agent use, a key file at `~/.config/slk/.encryption_key`
  (mode `0600`); the backend is chosen by `SLK_KEYRING_BACKEND` (`auto`
  default — keyring if available, else file) and recorded as
  `key_backend` in the config. Tokens set via `set-token`/`login` are
  encrypted on write; a pre-existing plaintext value is still read and
  is encrypted on the next write (no re-auth).
- The ID-to-name resolver caches in `~/.config/slk/cache/` (mode `0700`).
- Env overrides: `SLK_PROFILE` (active profile), `SLK_TOKEN` (raw token,
  highest precedence), `SLK_CONFIG` (config path), and
  `SLK_KEYRING_BACKEND` (encryption-key backend).

## Known Limitations / Non-goals

- **No browser session tokens** (`xoxc`/`xoxd`). The trust property of
  the open-source binary requires public OAuth scopes only; this rules
  out `drafts.list`/`drafts.delete`/`drafts.update` and the internal
  `search.modules.*` endpoints.
- **No macOS code-signing / notarization.** Release binaries are
  unsigned and un-notarized, and slk adds no Gatekeeper workaround (no
  quarantine-strip hook, no `--no-quarantine` guidance);
  `com.apple.quarantine` is treated as normal macOS behavior. Most
  install paths do not quarantine (`curl`/`wget`, the npm postinstall
  download, and Go's automatic ad-hoc `darwin/arm64` signing); a browser
  download or a Homebrew Cask install does, and that first-run Gatekeeper
  prompt is accepted. Developer ID signing + notarization (Apple
  Developer Program, paid yearly) is judged not worth it for a CLI;
  revisit only on a strong external driver such as an official
  `homebrew/cask` submission (which requires notarization).
- **Canvas read loses code-block language**: Slack's HTML route does
  not carry the original triple-backtick language hint. Inherent.
- **No Slack Lists delete**: `slackLists.delete` does not exist on the
  public API; `files.delete` requires `files:write` which slk's default
  scope set omits. Lists must be cleaned in the Slack UI.
- **Scheduled-message cancel race**: `chat.deleteScheduledMessage` may
  return `ok=true` for schedules within ~5 minutes of `post_at` yet the
  message still posts. Slack-side queue lock; documented in README.
- **DM `chat.delete`** returns `internal_error`. UI-only deletion.
- **`channel invite` cannot positively-test against the operator**:
  the maintainer is auto-member as creator; `cant_invite_self` is the
  only path that fits the privacy constraint.
- **Cobra usage errors map to exit 1**: spec calls for exit 2; the
  detection path is fiddly and deferred to v1.2.
- **`context.Context` is not threaded** through `api.Client.Call` /
  `CallAll`. CLI is short-lived; cancellation value is low. v1.2
  candidate.

## Key Decisions

- **MCP parity via curation, not pass-through.** The 13 Slack MCP tools
  are matched 1:1 by curated `slk` commands so an agent can replace MCP
  for every covered workflow. Token cost is the deciding factor: MCP
  ships ~5–10K tokens of schema and returns full message envelopes;
  slk emits 2–20-token confirmation lines by default.
- **xoxp-only.** Any feature that requires `xoxc`/`xoxd` (drafts
  list/delete/update, internal search modules) is intentionally
  unimplemented. README's "Known Slack-side limitations" section is the
  contract.
- **Canvas converter is in-house.** Off-the-shelf HTML→Markdown
  libraries drop `<lnk>` links, render quip code blocks as paragraphs,
  and lose checklist semantics — the three quip non-standardisms are
  encoded into `internal/quip` directly.
- **Slack Lists cell-shape responsibility lies with the caller.** Slack's
  rich_text block is verbose; rather than build a sugar DSL, slk passes
  the JSON through unchanged and documents the shape in `--fields` Long
  help + README. `--row-id` is the one ergonomic concession: when set,
  slk fills it into any cell that omits `row_id`, supporting both
  single-row and multi-row updates.
- **`thread read` and `thread reply` both use `--thread`.** A prior
  divergence (`thread read --ts`) was removed with no alias; the cost of
  a one-flag breaking change in pre-v1 is lower than the cost of dual
  flag names every agent must remember.
- **`user list` default-filters bots and deactivated users**, including
  the `is_bot=false` Slackbot which is special-cased by ID. The previous
  behavior (dump everything) leaked noise into every workspace listing;
  callers who need the full surface pass `--include-bots` /
  `--include-deactivated`.
- **`--raw` is the universal escape hatch on every API-touching verb.**
  Read verbs already honored it; write verbs now do too. This keeps the
  contract "concise by default, raw when asked" universal.
- **Update check is read-only and advisory.** `slk version --check`
  reports whether a newer GitHub release exists and prints the upgrade
  command, but never self-updates: homebrew-core rejects self-upgrading
  tools, and upgrades stay the package manager's job. The check is
  best-effort — a network or rate-limit failure prints the current
  version with a note and still exits `0`, so it never breaks a script;
  agents read the `update_available` / `checked` JSON fields rather than
  the exit code. Plain `version` performs no network I/O, preserving the
  offline, low-latency default.
- **Credentials are encrypted at rest, not just file-permissioned.**
  Sensitive fields use AES-256-GCM with the key in the OS keyring
  (opt-in, real local protection) or a key file (default; defends
  against accidental disclosure such as dotfile sync or screen-share,
  but not a local attacker who can already read `~/.config/slk/`).
  Default backend is `auto` because slk runs headless-first and must
  never block on an interactive keychain unlock. `Load` tolerates a
  pre-existing plaintext value and encrypts it on the next `Save`; it
  never rewrites the config on read.
- **The agent skill is a generated artifact (the binary is its SSOT).**
  Rather than hand-maintain `SKILL.md`, `slk generate-skill` renders it
  from the Cobra tree plus an embedded preamble, so the code, `--help`,
  and the skill cannot drift; a CI drift guard enforces it. Per-command
  `slackMethod` / `write` annotations supply the curated bits the tree
  alone cannot (the Slack method, the write CAUTION).
- **One committed `VERSION` file is the single version source**, embedded
  via `//go:embed`, replacing `-ldflags` injection so a locally built
  binary and the generated skill report the same version deterministically.
- **Releases are VERSION-driven, not tag-driven.** Changing `VERSION` on
  `main` triggers the release; a gate derives `v<VERSION>`, skips if it
  already exists, else creates the tag and releases in one run. The tag is
  an artifact, not the trigger — no manual tag step and no PAT (a
  `GITHUB_TOKEN`-pushed tag would not start a separate workflow).
