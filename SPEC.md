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
OAuth user or bot tokens (`xoxp-`/`xoxb-`). 1:1 parity with the 13 Slack MCP tools was
the original baseline; the curated surface has since grown to ~96 verbs
across 15 command groups plus the `api` escape hatch, covering most
user- or bot-token-reachable Slack Web API methods (everything the default OAuth
scope set grants, excluding
Enterprise-Grid `admin.*`, `xoxc`/`xoxd`-only browser tokens, deprecated, and
app-framework methods).

## Architecture

`slk` is a single static Go binary. The process:

1. Cobra builds the command tree (`internal/commands.NewRootCommand`).
2. The root command binds global flags via `internal/commands.GlobalFlags`
   (`--format`, `--profile`, `--raw`, `--dry-run`, `--no-resolve`,
   `--as`). `--as user|bot` (default "") serves two roles: on a normal
   command it asserts the active token's derived scope (a known mismatch
   returns `*auth.AuthError`, exit 3); on `auth login` it selects which
   token to mint (default user).
3. Each subcommand resolves a token through `internal/auth` via
   `auth.ResolveToken(cfg, profileName, assertScope, envToken)`:
   precedence env(`SLK_TOKEN`) → named profile → `cfg.Active`.
   The `assertScope` argument is the `--as` value; when non-empty and the
   resolved token's derived scope is known and differs, the call returns
   `*auth.AuthError` (exit 3); an unknown prefix passes. The client is
   then an `internal/api.Client` pointed at `https://slack.com/api`.
4. The client serializes form-urlencoded params, performs the HTTP call,
   maps `ok=false` responses into `*api.APIError`, retries on 429
   (`Retry-After`) up to a small bound, and returns raw JSON. A separate
   `Client.CallMultipart` (`internal/api/multipart.go`) sends a
   multipart/form-data body with a single file part for the few methods
   that take a binary upload (`users.setPhoto`; the external-upload step
   of `file upload` POSTs raw bytes to Slack's returned `upload_url`).
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
generate-skills` (a hidden command) renders a skill tree from the live Cobra
command tree plus embedded templates (`internal/skillgen`), so the code, `slk
--help`, and the skills share one source and cannot drift. The tree is a small
`slk` index skill (syntax, a command-group directory linking to each group's
skill, the top-level leaf commands `api`/`version`, and the sole copy of the
`install`/openclaw block), a `slk-shared` cross-cutting reference (global flags,
security rules, exit codes, shell tips), and one `slk-<group>` skill per command
group. Each group skill carries only `requires.bins: [slk]` plus a PREREQUISITE
pointer to `slk-shared`, a quick command-index table, and per-verb detail.
The group set is derived from the command tree at generation time, so adding or
removing a command group adds or removes its skill with no hand-maintained list.
Each command carries `Annotations["slackMethod"]` (the Slack method it wraps)
and, for mutating commands, `Annotations["write"]="true"`; the generator emits
these as a per-command `**Slack API:**` line and a write `CAUTION` callout, and
renders flag tables (`Required` from `MarkFlagRequired`, `Default` from the flag
default). A CI job regenerates the tree and fails on any diff (staged, so added
or removed skill files are caught too).

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
│   │   ├── multipart.go        # CallMultipart: multipart/form-data file upload
│   │   ├── errors.go           # APIError + ExitCodeFor() mapping
│   │   └── paginate.go         # CallAll: walks next_cursor up to N pages
│   ├── auth/                   # credential management
│   │   ├── store.go            # TOML config at ~/.config/slk/config.toml
│   │   ├── crypto.go           # AES-256-GCM field encrypt/decrypt
│   │   ├── keyprovider.go      # key file / OS-keyring backends
│   │   ├── status.go           # EncryptionStatus / EncryptionInfo for `auth status`
│   │   ├── token.go            # ResolveToken precedence + assertScope
│   │   ├── scope.go            # TokenScope (prefix→"user"/"bot") + IsEncrypted wrapper
│   │   ├── oauth.go            # local-callback OAuth flow
│   │   └── errors.go           # AuthError → exit code 3
│   ├── commands/               # Cobra command tree (one file per group)
│   │   ├── root.go             # NewRootCommand + global flag binding
│   │   ├── context.go          # GlobalFlags struct
│   │   ├── clientutil.go       # buildClient(g) → *api.Client
│   │   ├── input.go            # readContent: --text vs --text-file/stdin
│   │   ├── prompt.go           # interactive prompts + field resolution (set-token + login)
│   │   ├── lookup.go           # slackLookup + resolveUser wiring
│   │   ├── api.go              # escape-hatch `slk api`
│   │   ├── auth.go             # set-token / status / switch / logout / login /
│   │   │                       # test (auth.test) / revoke (auth.revoke)
│   │   ├── msg.go              # send / read / update / delete / react / unreact /
│   │   │                       # schedule / unschedule / scheduled / permalink /
│   │   │                       # ephemeral / me / reactions / reacted / draft
│   │   ├── thread.go           # read / reply (uses --thread on both)
│   │   ├── canvas.go           # create / read / update / list / delete / share / unshare
│   │   ├── channel.go          # create / archive / invite / topic / list / info /
│   │   │                       # members / join / leave / purpose / kick / rename /
│   │   │                       # unarchive / open / mark / close
│   │   ├── list.go             # Slack Lists: create / read / add-item / update-item /
│   │   │                       # delete-item / update; injectRowID helper
│   │   ├── user.go             # info / profile / list / by-email / presence /
│   │   │                       # channels / set-profile / set-photo / delete-photo /
│   │   │                       # set-presence
│   │   ├── search.go           # messages / channels / users / files / all
│   │   ├── file.go             # files: list / info / upload / delete / public / revoke-public
│   │   ├── pin.go              # pins: add / remove / list
│   │   ├── bookmark.go         # bookmarks: add / edit / remove / list
│   │   ├── team.go             # team: info / profile
│   │   ├── emoji.go            # emoji: list
│   │   ├── dnd.go              # dnd: info / team / snooze / end-snooze / end
│   │   ├── usergroup.go        # usergroups: list / create / update / enable /
│   │   │                       # disable / users / set-users
│   │   ├── version.go          # version + --check GitHub-release probe
│   │   ├── scopes.go           # scopeUnion: builds sorted scope sets from annotations
│   │   ├── generateskill.go    # hidden `generate-skills`: writes the skills/ tree
│   │   └── generatemanifest.go # hidden `generate-manifest`: rewrites README manifest block
│   ├── skillgen/               # skill-tree generator
│   │   ├── skillgen.go         # GenerateAll(tree, version) → map[path]content
│   │   └── templates/          # embedded index / shared / group templates
│   │       ├── index.md.tmpl
│   │       ├── shared.md.tmpl
│   │       └── group.md.tmpl
│   ├── output/                 # concise|json|jsonl|table renderers
│   ├── quip/                   # canvas HTML→Markdown converter
│   │   ├── convert.go
│   │   └── testdata/           # canvas_fixture.{html,md} golden file
│   └── resolve/                # ID→name cache (~/.config/slk/cache)
├── skills/                     # GENERATED agent skills — do not hand-edit
│   ├── slk/SKILL.md            # index (group directory + api/version + install block)
│   ├── slk-shared/SKILL.md     # shared: global flags, security, exit codes, shell tips
│   └── slk-<group>/SKILL.md    # one per command group (msg, channel, …)
├── npm/                        # npm wrapper (postinstall downloads the binary)
│   ├── package.json            # bin.slk=run.js, postinstall=install.js
│   ├── install.js              # download tarball + verify checksum
│   ├── platform.js             # os/arch → supportedPlatforms key
│   └── run.js                  # re-exec bin/slk (install if missing)
├── docs/superpowers/           # design specs + implementation plans
├── .github/
│   ├── workflows/
│   │   ├── ci.yml              # PR + push-main: gofmt, vet, build, test, goreleaser check; skill/version-sync/manifest drift guards
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
- **Version source of truth**: the root `VERSION` file is the single
  version source. The binary embeds it via `//go:embed` (`version.go`,
  feeding `slk --version`); no `-ldflags` injection. The git release tag
  (`v<VERSION>`) and the published npm version derive automatically in CI
  at release time (`npm/package.json`'s committed `0.0.0` is a
  placeholder). Several committed files carry derived content, fanned out via
  **three separate concerns**: (1) the `skills/` tree (each skill's
  `metadata.version`) is produced by `slk generate-skills`, which rebuilds
  the whole tree from the command tree and stamps the version (CI `skill`
  job guards drift); (2) `SECURITY.md` (supported-versions table) is a pure
  version copy rewritten by `scripts/sync-version.sh` (CI `version-sync` job
  guards drift);
  (3) the README app-manifest scope block (between
  `<!-- BEGIN/END GENERATED MANIFEST -->` markers) is regenerated by
  `slk generate-manifest` from the command tree's scope annotations — this
  is **scope/command-driven, not version-driven** and is NOT part of the
  VERSION-bump fan-out (CI `manifest` job guards drift; run on scope or
  command annotation changes).
- **The skills are generated**: the `skills/` tree is produced by `slk
  generate-skills` from the Cobra tree — never hand-edit it. Per-command
  `Annotations["slackMethod"]` and `Annotations["write"]` drive the
  generated Slack-method line and the write `CAUTION`; CI fails if the
  committed tree drifts from the generator.
- **README scope**: the README is human onboarding plus knowledge `slk
  --help` cannot convey (usage traps, data shapes, behavior notes). The
  complete, in-sync command reference is `slk --help` and the generated
  `skills/` tree; do not reintroduce a per-command example list or a
  global-flags table in the README — it would be a partial, drift-prone
  mirror of the command tree, contradicting the generated-skill SSOT.

## Verification

- Build: `go build -o slk ./cmd/slk` (version is read from the embedded `VERSION` file)
- Test: `go test ./...` (no external services touched)
- Forced refresh: `go clean -testcache && go test ./...`
- CI (`.github/workflows/ci.yml`): on every PR and push to `main` (code
  paths only — `**.md`, `docs/**`, `LICENSE`, `.gitignore` are ignored),
  GitHub Actions runs a gofmt check, `go vet`, `go build ./...`,
  `go test ./...`, and `goreleaser check` on Go 1.25, plus three drift-guard
  jobs: `skill` (regenerates `skills/` and fails on any staged diff),
  `version-sync` (runs `sync-version.sh` and fails if `SECURITY.md` drifts),
  and `manifest` (runs `generate-manifest` and fails
  if `README.md`'s manifest block drifts).
- Coverage snapshot: `go test ./... -coverpkg=./...
  -coverprofile=/tmp/slk.cov && go tool cover -func=/tmp/slk.cov`

Each package carries focused tests:

| Package | Coverage focus |
|---|---|
| `internal/api` | error mapping, paginator cursor handling, retry on 429, multipart upload |
| `internal/auth` | TOML load/save, token-precedence matrix, OAuth callback |
| `internal/commands` | dry-run output, flag registration, response-shape parsing helpers |
| `internal/output` | each render branch + unknown-format error path |
| `internal/quip` | per-element-family unit tests + full-fixture golden diff |
| `internal/resolve` | cache hit, cross-instance disk persistence, graceful degradation |

Command tests stay at dry-run + flag-registration depth because
`buildClient` reaches real config; helpers that parse Slack responses
are exposed as small functions (`parseListCreateID`,
`parseScheduledMessageID`, `parseListItems`, `messageDisplay`,
`injectRowID`, `fetchChannelsWith`, `fetchUsersWith`, and the newer
per-group parsers such as `parseChannelMembers`, `parseUsergroups`,
`fileFetchList`, `parseBookmarks`, `parseEmojiList`, `formatAuthIdentity`)
and unit-tested in isolation against `httptest.NewServer`-backed
`api.Client` instances.

End-to-end coverage against a live Slack workspace is run manually
when changes touch write paths. The matrix and findings are recorded
in the session transcript or handoff document that performed the run,
not committed to the repo (the live workspace identifiers belong
outside the public tree).

## Deploy

Releases are **VERSION-driven**, not tag-driven. Bump the `VERSION` file, then
regenerate the skills (`go run ./cmd/slk generate-skills`) and run
`scripts/sync-version.sh` (fans VERSION into `SECURITY.md`) in a release PR. Note: `go run ./cmd/slk generate-manifest`
is **not** a per-VERSION step — run it only when command annotations or scope
sets change (guarded by the CI `manifest` job). On merge to `main`,
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

**Release recovery (failed run).** The release is one-shot and not currently
re-runnable: the `gate` skips if the tag exists, the tag-creation step is not
idempotent, and there is no `workflow_dispatch`. If a run fails:

1. Treat the failed attempt as a record — do **not** erase it. Leave the
   `chore(release)` PR and its commit in place, and comment on that PR with the
   cause and whether it was transient/resolved.
2. Do **not** reuse the version number. Bump to the next patch in a fresh
   `chore(release)` PR and let the automation release it.
3. Delete only the orphaned `v<failed>` tag — it has no attached release, so it
   is not a meaningful record. Keep anything that actually published.
4. A transient cause (e.g. a goreleaser `401 Bad credentials` GitHub auth glitch)
   usually clears on the re-release; if it recurs, fix the token before retrying.

Precedent: v0.5.0 hit a transient goreleaser 401 while creating the GitHub
release; nothing published, so it was re-released as v0.5.1 and PR #33 was
annotated as the record.

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

- Credentials live in `~/.config/slk/config.toml` (mode `0600`,
  TOML-encoded, multi-profile). Each profile stores a **single `token`
  field** (user `xoxp-` or bot `xoxb-`); token scope is derived at runtime
  from the prefix via `auth.TokenScope` and is never stored. The old
  `user_token`/`bot_token` dual-field layout is removed; legacy configs
  with those keys are silently ignored on load (BurntSushi TOML drops
  unknown keys) — a clean-break migration with no migration code. The
  `token` field is encrypted at rest with AES-256-GCM (`enc:v1:` prefix).
  The OAuth `client_id` / `client_secret` are NOT persisted — `auth login`
  uses them only transiently for the token exchange. The 32-byte key is held
  in the OS keyring or, for headless/agent use, a key file at
  `~/.config/slk/.encryption_key` (mode `0600`); the backend is chosen by
  `SLK_KEYRING_BACKEND` (`auto` default — keyring if available, else file)
  and recorded as `key_backend` in the config. Tokens set via
  `set-token`/`login` are encrypted on write; a pre-existing plaintext value
  is still read and is encrypted on the next write (no re-auth).
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
- **No whole-List delete**: `slackLists.delete` does not exist on the
  public API, so a whole List must be cleaned in the Slack UI. Individual
  rows are deletable via `list delete-item` (`slackLists.items.delete`).
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
- **Coverage extends beyond MCP parity.** Once curation proved out, the
  surface was expanded to wrap most user- or bot-token-reachable Web API
  methods, not just the 13 MCP equivalents. The boundary is "what an OAuth
  user or bot token can be granted": included families are gated only by
  adding their scope to per-command annotations; excluded are Enterprise-Grid
  `admin.*`, `xoxc`/`xoxd`-only browser tokens, deprecated
  (`reminders.*`/`stars.*`), and app-framework
  (`views`/`workflows`/`functions`/…) methods. The default OAuth scope set
  for `auth login` is **generated at runtime** by `scopeUnion` from the
  command tree's `userScopes`/`botScopes` annotations (adding a scope to a
  command annotation widens the default automatically); the oracle locks
  **37 user / 35 bot** scopes (`canvases:read` is included).
- **OAuth tokens only; no `xoxc`/`xoxd` browser tokens.** Both `xoxp-`
  (user) and `xoxb-` (bot) OAuth tokens are supported. Any feature that
  requires `xoxc`/`xoxd` (drafts list/delete/update, internal search
  modules) is intentionally unimplemented. README's "Known Slack-side
  limitations" section is the contract.
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
- **`auth set-token` and `auth login` share one interactive model;
  prompts are a human fallback, never for agents.** Fields can be supplied
  by flags (the agent/script path); set-token accepts a single `--token`
  flag (or `--token -` to read from stdin; keeps secrets out of shell
  history) with prefix validation (accepts `xoxp-`/`xoxb-` only; stores
  the one token; scope is derived from the prefix). Otherwise, when stdin
  is a TTY, the missing fields are prompted — tokens and the client secret
  entered hidden. Prompting is gated on a TTY and suppressed by
  `--non-interactive`, so headless/agent callers never block (a missing
  required value is a clear error, not a hang). Shared resolution lives in
  `promptCtx` (`internal/commands/auth.go` + `prompt.go`). set-token
  refuses a profile that resolves to no token (previously a no-flag
  invocation silently saved an empty profile); login mints a single token
  via OAuth (`--as bot` → uses `scope=` in the authorize URL; default/user
  → uses `user_scope=`) with default scopes generated by `scopeUnion`, and
  does NOT persist the `client_id` / `client_secret` used for the exchange
  (non-interactive login defaults the profile to `default` and prints a
  hint).
- **`auth status` verifies live, identity is derived not stored.** A profile
  holds only its token — there is no stored workspace/label field (a removed
  cosmetic that no logic read; the profile name already disambiguates). Instead
  `auth status` shows a derived `[user]`/`[bot]` scope label (also
  `unknown`/`encrypted`/`none`) beside each profile name, and resolves identity
  live: by default it runs `auth.test` for the active profile (`--all` for every
  profile, `--offline` to skip the network), appending
  `<team> (<team_id>) — <user> (<user_id>) @ <url>`. `--format json` emits a
  structured object: `encryption{backend, status}`, `active`, and `profiles[]`
  each with `name`/`active`/`scope`/`checked` and either `identity` on success
  or `error` on failure. Failures are classified by the error type from the
  `liveIdentity` seam: an `*api.APIError` is an invalid/rejected token, any
  other error is offline; a token Load could not decrypt (`auth.IsEncrypted`) is
  flagged locally and never hits the network. The check uses a short (4s) client
  timeout so offline fails fast, and `--all` fans the per-profile `auth.test`
  calls out concurrently (results are collected before printing to preserve
  sorted order). An older config's obsolete `workspace` key is ignored on Load
  and dropped on the next Save (BurntSushi toml ignores unknown keys).
- **Bot-token support via annotations, not separate storage.** Each command
  carries `userScopes`/`botScopes` annotations (comma-separated; empty means
  no additional scope beyond auth) and a `botCapable` annotation (`"true"` for
  commands that work with a bot token; `"false"` or absent for user-only verbs).
  `buildClient` reads `botCapable`: a bot token on a user-only verb fails fast
  with `*auth.AuthError` (exit 3) before any API call. `scopeUnion(root, mint)`
  in `internal/commands/scopes.go` builds the sorted, de-duplicated scope set
  for the chosen identity by walking the full command tree — this single source
  feeds both `auth login` default scopes and `generate-manifest`. The oracle test
  `TestScopeUnion_Oracle` locks the current counts (**37 user / 35 bot**); adding
  a scope annotation to any command widens the defaults automatically. The README
  app-manifest block (between `<!-- BEGIN/END GENERATED MANIFEST -->` markers) is
  regenerated by `slk generate-manifest` from the same annotations and guarded by
  the CI `manifest` job — run it on any scope or command annotation change, not
  as part of a VERSION bump.
- **The agent skill is a generated artifact (the binary is its SSOT).**
  Rather than hand-maintain `SKILL.md`, `slk generate-skills` renders a
  skill tree (a small `slk` index, a `slk-shared` reference, and one
  `slk-<group>` skill per group) from the Cobra tree plus embedded
  templates, so the code, `--help`, and the skills cannot drift; a CI
  drift guard enforces it. Splitting per group keeps each skill small so
  an agent loads only the surface it needs. Per-command `slackMethod` /
  `write` annotations supply the curated bits the tree alone cannot (the
  Slack method, the write CAUTION).
- **One committed `VERSION` file is the single version source**, embedded
  via `//go:embed`, replacing `-ldflags` injection so a locally built
  binary and the generated skill report the same version deterministically.
- **Releases are VERSION-driven, not tag-driven.** Changing `VERSION` on
  `main` triggers the release; a gate derives `v<VERSION>`, skips if it
  already exists, else creates the tag and releases in one run. The tag is
  an artifact, not the trigger — no manual tag step and no PAT (a
  `GITHUB_TOKEN`-pushed tag would not start a separate workflow).
