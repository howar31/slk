# slk — SPEC

## Purpose

`slk` is an agent-facing command-line client for the Slack Web API. It
exists so AI coding agents (and the humans they collaborate with) can
read, send, and manage Slack content from a shell without dragging the
official Slack MCP connector's verbose JSON envelopes into the model's
context window. The CLI emits curated, low-token output by default and
exposes `--raw` for callers that want full API responses.

Primary consumer: a developer agent (Claude Code / Sera-class) running on
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

External dependencies are intentionally narrow:

- `github.com/spf13/cobra` (+ `pflag`) for the command tree
- `github.com/BurntSushi/toml` for config persistence
- `golang.org/x/net/html` for the canvas converter
- `crypto/rand` for draft `client_msg_id` UUIDs
- standard library for everything else (HTTP, JSON, files)

## Layout

```
.
├── cmd/slk/main.go            # process entry; error→exit-code wiring
├── internal/
│   ├── api/                   # HTTP client, error mapping, paginator
│   │   ├── client.go          # api.Client; BaseURL overridable for tests
│   │   ├── errors.go          # APIError + ExitCodeFor() mapping
│   │   └── paginate.go        # CallAll: walks next_cursor up to N pages
│   ├── auth/                  # credential management
│   │   ├── store.go           # TOML config at ~/.config/slk/config.toml
│   │   ├── token.go           # ResolveToken precedence
│   │   ├── oauth.go           # local-callback OAuth flow
│   │   └── errors.go          # AuthError → exit code 3
│   ├── commands/              # Cobra command tree (one file per group)
│   │   ├── root.go            # NewRootCommand + global flag binding
│   │   ├── context.go         # GlobalFlags struct
│   │   ├── clientutil.go      # buildClient(g) → *api.Client
│   │   ├── input.go           # readContent: --text vs --text-file/stdin
│   │   ├── lookup.go          # slackLookup + resolveUser wiring
│   │   ├── api.go             # escape-hatch `slk api`
│   │   ├── auth.go            # set-token / status / switch / logout / login
│   │   ├── msg.go             # send / read / update / delete / react /
│   │   │                      # schedule / draft + slackMessage helper
│   │   ├── thread.go          # read / reply (uses --thread on both)
│   │   ├── canvas.go          # create / read / update / list
│   │   ├── channel.go         # create / archive / invite / topic / list
│   │   ├── list.go            # Slack Lists: create / read / add-item /
│   │   │                      # update-item; injectRowID helper
│   │   ├── user.go            # info / profile / list (default-filter)
│   │   └── search.go          # messages / channels / users
│   ├── output/                # concise|json|jsonl|table renderers
│   ├── quip/                  # canvas HTML→Markdown converter
│   │   ├── convert.go
│   │   └── testdata/          # canvas_fixture.{html,md} golden file
│   └── resolve/               # ID→name cache (~/.config/slk/cache)
├── skill/SKILL.md             # agent-facing usage skill
├── docs/superpowers/          # design spec + implementation plan
│   ├── specs/2026-05-19-slack-cli-design.md
│   └── plans/2026-05-19-slk-slack-cli.md
├── .goreleaser.yaml           # darwin/linux × amd64/arm64 + homebrew tap
├── go.mod / go.sum
├── README.md
└── LICENSE                    # MIT
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

## Verification

- Build: `go build -ldflags "-X main.version=<v>" -o slk ./cmd/slk`
- Test: `go test ./...` (no external services touched)
- Forced refresh: `go clean -testcache && go test ./...`
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

Releases are tag-driven. Push a semver tag matching
`v[0-9]+.[0-9]+.[0-9]+*` and `.github/workflows/release.yml`:

1. Runs `goreleaser release --clean` on ubuntu-latest, producing
   `slk_<os>_<arch>.tar.gz` for `darwin/linux × amd64/arm64` plus
   `checksums.txt`.
2. Creates the GitHub Release with the artifacts (`release.prerelease:
   auto` marks tags containing `-`, e.g. `v0.1.0-rc1`, as prereleases).
3. Generates SLSA build provenance attestations via
   `actions/attest-build-provenance@v3`.
4. Pushes a Homebrew formula update to `howar31/homebrew-tap` (uses the
   `HOMEBREW_TAP_TOKEN` PAT).
5. For non-prerelease tags, runs a `publish-npm` job that bumps
   `npm/package.json`'s version to match the tag and publishes
   `@howar31/slk` (uses `NPM_TOKEN`). Prereleases skip the npm publish.

The `npm/` directory is a thin postinstall-driven wrapper:

| File | Responsibility |
|---|---|
| `package.json` | Declares `bin.slk = run.js`, `scripts.postinstall = install.js`, and `supportedPlatforms` (4 darwin/linux × arm64/x64 entries). Version is overwritten by CI at publish time. |
| `install.js` | Downloads the matching tarball + `checksums.txt` from GitHub Releases, verifies SHA256, extracts to `bin/`. |
| `platform.js` | Maps `os.type()`/`os.arch()` to a `supportedPlatforms` key. |
| `run.js` | Re-execs `bin/slk`; triggers `install.js` if the binary is missing (e.g. when the user ran `npm install --ignore-scripts`). |

Runtime state:

- OAuth tokens live in `~/.config/slk/config.toml` (mode `0600`,
  TOML-encoded, multi-profile).
- The ID-to-name resolver caches in `~/.config/slk/cache/` (mode `0700`).
- Two env overrides: `SLK_PROFILE` (active profile) and `SLK_TOKEN`
  (raw token, highest precedence).

## Known Limitations / Non-goals

- **No browser session tokens** (`xoxc`/`xoxd`). The trust property of
  the open-source binary requires public OAuth scopes only; this rules
  out `drafts.list`/`drafts.delete`/`drafts.update` and the internal
  `search.modules.*` endpoints.
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
- **No CI**: tests run locally.
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
