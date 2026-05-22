# slk Distribution & Install Ergonomics — Design

**Date:** 2026-05-20
**Status:** Draft, pending user review
**Companion plan:** `docs/superpowers/plans/2026-05-20-slk-distribution.md` (to be written by writing-plans)
**Inspiration:** `googleworkspace/cli` (Rust, ~26k stars) — distribution model studied at the same date.

## Purpose

slk's binary is currently only installable via `go install github.com/howar31/slk/cmd/slk@latest`. There are no GitHub Releases, the `brews:` block in `.goreleaser.yaml` points at a tap repo (`howar31/homebrew-tap`) that does not exist, and the Claude Code skill ships as a `mkdir + cp` recipe in the README. Non-Go users, Mac CLI users who prefer Homebrew, Node-ecosystem users, and AI agents on harnesses other than Claude Code all hit friction.

This document specifies a two-PR refresh that brings slk's install surface up to the `gws` model: pre-built binaries on GitHub Releases, Homebrew tap, npm scoped wrapper, Gemini CLI extension, and OpenClaw auto-install hook — all driven from a single `release.yml` workflow triggered by a semver tag push.

## Goals

1. `git tag v0.x.y && git push --tags` produces, with no further human action, all of:
   - GitHub Release with darwin/linux × amd64/arm64 tarballs + SHA256 + build provenance
   - `howar31/homebrew-tap/Formula/slk.rb` updated to the new version
   - `@howar31/slk` published to npm with the new version
2. Five install paths documented in README in recommended order: Homebrew → npm → pre-built binary → `go install` → from source.
3. Skill install for Claude Code remains the existing `mkdir + cp`; Gemini CLI gets a one-command extension install; OpenClaw auto-installs the binary via npm by reading the skill frontmatter.
4. Distribution decisions encoded in the codebase, not in tribal knowledge — `CLAUDE.md` `## Run / build` reflects the GHA-on-tag flow; `SPEC.md` `## Deploy` drops the stale "no CI" and "tap not created" notes.

## Non-goals

- Windows binary (deferred to v0.2; reduces first-cut cross-compile surface and npm install.js zip handling).
- Nix flake (low user-base ROI given slk's current audience).
- ClawHub registry publish (user opted to rely on `npx skills add <github-url>` instead).
- Homebrew-core formula (slk does not meet notability rules; revisit post-v1.0).
- Splitting `skill/SKILL.md` into per-resource skills (slk's verb surface is too small to justify; gws splits because it has 100+ Discovery services).
- crates.io equivalent (Go's `go install` already covers source-install; no separate registry needed).

## Approach (chosen: Foundation-first, two PRs)

Two alternatives were considered:

- **A — Single big PR.** Releases + brew + npm + Gemini + skill autoinstall + README all together. Pros: one review, one README rewrite. Cons: ~600 LOC diff, hard to bisect a broken release pipeline.
- **C — Per-channel incremental, four PRs.** Pros: minimal review surface each PR. Cons: four release tags before all channels are live, README churns four times, per-PR setup overhead.

**Chosen: B — Foundation-first, two PRs.** Validates the binary pipeline in isolation (a `v0.1.0-rc1` tag) before stacking dependent channels (npm, brew) on top of it. Bundles the four downstream channels into one cohesive PR that shares one README rewrite.

## PR 1 — Release pipeline foundation

**Goal:** make `curl + tar` install work end-to-end; defer brew/npm to PR 2.

### Files

| File | Change | Notes |
|---|---|---|
| `.github/workflows/release.yml` | NEW (~120 LOC) | Tag-triggered goreleaser run + build provenance attestation |
| `.goreleaser.yaml` | EDIT | Comment out `brews:` block (re-added in PR 2); set `archives.name_template: 'slk_{{ .Os }}_{{ .Arch }}'`; ensure `checksum:` block present |
| `README.md` | EDIT | Replace "Pre-built binaries: Planned" with concrete `curl + shasum -c + tar xzf` example; keep brew/npm marked "PR 2" |

### `release.yml` shape

```yaml
name: Release
on:
  push:
    tags: ['v[0-9]+.[0-9]+.[0-9]+*']
permissions:
  contents: write
  id-token: write
  attestations: write
jobs:
  goreleaser:
    runs-on: ubuntu-latest    # cross-builds darwin from linux runner
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: '1.25' }
      - uses: goreleaser/goreleaser-action@v6
        with: { version: latest, args: release --clean }
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      - uses: actions/attest-build-provenance@v3
        with: { subject-path: 'dist/*.tar.gz' }
```

### Artifact naming

`slk_darwin_arm64.tar.gz` / `slk_darwin_amd64.tar.gz` / `slk_linux_arm64.tar.gz` / `slk_linux_amd64.tar.gz`. No version in filename — the GitHub Release URL path (`releases/download/v0.1.0/`) carries the version. This matches gws's no-version-in-artifact convention and simplifies the npm wrapper's URL construction in PR 2.

### Out-of-band actions (one-time)

1. `gh repo create howar31/homebrew-tap --public --description "Homebrew tap for howar31's tools"` — create empty tap repo. PR 1 does NOT push to it; PR 2 enables the goreleaser brews block.

### PR 1 verification

1. Local snapshot: `goreleaser release --snapshot --clean` produces 4 tarballs + `checksums.txt` in `dist/`.
2. Merge PR. Tag `v0.1.0-rc1`. Push tag. GHA run is green.
3. `curl -sLO https://github.com/howar31/slk/releases/download/v0.1.0-rc1/slk_darwin_arm64.tar.gz` on a Mac → tar xzf → `./slk --version` prints `0.1.0-rc1`.
4. Any failure: PR 1 is not done.

## PR 2 — Distribution channels

**Goal:** brew + npm + Gemini extension + skill autoinstall + full README rewrite. Tag `v0.1.0` is the final outcome.

### Files

| File | Change |
|---|---|
| `.github/workflows/release.yml` | EDIT — add `publish-npm` job downstream of `goreleaser` |
| `.goreleaser.yaml` | EDIT — restore and harden `brews:` block (token via `HOMEBREW_TAP_TOKEN`) |
| `npm/package.json` | NEW |
| `npm/install.js` | NEW |
| `npm/platform.js` | NEW |
| `npm/run.js` | NEW |
| `npm/.gitignore` | NEW (`bin/`, `*.tgz`) |
| `gemini-extension.json` | NEW (root) |
| `skill/SKILL.md` | EDIT — add `openclaw.install` hint pointing to `@howar31/slk` |
| `README.md` | EDIT — full Installation rewrite (5 paths) + Agent setup section |
| `SPEC.md` | EDIT — `## Deploy` section refresh |
| `CLAUDE.md` | EDIT — `## Run / build` and `## Workflow rules` refresh |

### npm wrapper design

Adapted from `googleworkspace/cli/npm/`. Four files:

- **`package.json`** — name `@howar31/slk`, `bin.slk = "run.js"`, `scripts.postinstall = "node install.js"`, `engines.node = ">=18"`, `supportedPlatforms` enumerates `darwin-arm64` / `darwin-x64` / `linux-arm64` / `linux-x64` mapping to `{ artifact, binary }`.
- **`install.js`** — reads `version` from `package.json`, fetches `https://github.com/howar31/slk/releases/download/v${version}/${artifact}` via native fetch, fetches `.sha256` companion, verifies hash with `crypto.createHash('sha256')`, extracts via `tar` to `npm/bin/`, `chmod 755`, writes `.version` sentinel to short-circuit re-install on the same version.
- **`platform.js`** — maps `os.type()` + `os.arch()` to a platform key, returns the matching entry from `supportedPlatforms`. Errors clearly on unsupported platforms (Windows, etc.).
- **`run.js`** — exec's `bin/slk` with `process.argv.slice(2)`. If `bin/slk` is missing (e.g. user ran `npm install --ignore-scripts`), spawns `install.js` synchronously first.

### npm version sync

`release.yml`'s `publish-npm` job runs `npm version ${GITHUB_REF_NAME#v} --no-git-tag-version` in the `npm/` directory before `npm publish`. Git tag is the single source of truth; `npm/package.json` version field is overwritten at release time and not committed back. Avoids the changesets infrastructure gws uses.

### `.goreleaser.yaml` brews

```yaml
brews:
  - repository:
      owner: howar31
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    directory: Formula
    description: Agent-facing Slack CLI
    license: MIT
    homepage: https://github.com/howar31/slk
    test: |
      assert_match version.to_s, shell_output("#{bin}/slk --version")
```

### `gemini-extension.json`

```json
{
  "name": "slk",
  "version": "latest",
  "description": "Agent-facing Slack CLI for AI agents.",
  "contextFileName": "skill/SKILL.md"
}
```

Install command for users: `gemini extensions install https://github.com/howar31/slk`. README notes that the user must still have `slk` on PATH (via brew / npm / binary).

### Skill frontmatter

```yaml
metadata:
  version: 0.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - slk
    install:
      npm: "@howar31/slk"
```

**Schema risk:** the `install:` key shape under `openclaw:` is inferred from gws README claims ("OpenClaw auto-installs the CLI via `npm` if `gws` isn't on PATH"), not from a publicly inspected OpenClaw spec. First implementation step: verify the actual schema. If different, conform; if the feature does not yet exist, ship skill without the `install:` block — the README install paths remain functional fallbacks.

### `release.yml` additions

PR 2 also edits the existing `goreleaser` job to pass `HOMEBREW_TAP_TOKEN` into goreleaser's env so the `brews:` block can push to the tap repo:

```yaml
      - uses: goreleaser/goreleaser-action@v6
        with: { version: latest, args: release --clean }
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
```

New `publish-npm` job downstream of `goreleaser`:

```yaml
publish-npm:
  needs: goreleaser
  if: ${{ !contains(github.ref_name, '-') }}   # skip prereleases
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-node@v4
      with:
        node-version: '20'
        registry-url: 'https://registry.npmjs.org'
    - working-directory: npm
      run: |
        VER=${GITHUB_REF_NAME#v}
        npm version "$VER" --no-git-tag-version
        npm publish --access public
      env:
        NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
```

### README rewrite

`## Installation` reorganized to 5 paths in recommendation order:

1. **Homebrew** — `brew install howar31/tap/slk`
2. **npm** — `npm i -g @howar31/slk` (note: scoped because unscoped `slk` was taken)
3. **Pre-built binary** — curl + checksum + tar (PR 1 content retained)
4. **`go install`** — `go install github.com/howar31/slk/cmd/slk@latest`
5. **From source** — `git clone && go build`

New `## Agent setup` section covers:
- Claude Code: `mkdir + cp skill/SKILL.md` (existing flow)
- Gemini CLI: `gemini extensions install https://github.com/howar31/slk`
- OpenClaw: auto-installs via the skill's `install:` block (assuming schema verified)
- Cursor / aider / others: paste `slk --help` into rules file

### SSOT updates inside PR 2

- **`SPEC.md` `## Deploy`** — remove the lines saying the Homebrew tap is not yet created and that no CI workflow exists; add a paragraph describing `npm/` subdirectory purpose and file responsibilities.
- **`CLAUDE.md` `## Run / build`** — change the `goreleaser release --clean` line to `git tag v0.x.y && git push --tags` (the GHA flow). Add a `## Workflow rules` bullet: **"Release is GHA-on-tag only; do not run `goreleaser release` locally against the real GitHub remote."** Local snapshot is still allowed via `goreleaser release --snapshot --clean`.

### Required secrets

| Secret | Purpose | Source |
|---|---|---|
| `NPM_TOKEN` | `npm publish` from `publish-npm` job | npmjs.com → Access Tokens → Granular → scope `@howar31` packages |
| `HOMEBREW_TAP_TOKEN` | goreleaser pushes formula to `howar31/homebrew-tap` | GitHub Fine-grained PAT, scope `contents: write` on `howar31/homebrew-tap` |

The default `GITHUB_TOKEN` covers release creation and provenance attestation on the slk repo itself.

### PR 2 verification

1. Local: `goreleaser release --snapshot --clean` produces `dist/howar31-homebrew-tap/Formula/slk.rb`.
2. Both secrets added to slk repo. Tag `v0.1.0`. Push tag. All GHA jobs green.
3. macOS: `brew install howar31/tap/slk && slk --version` → `0.1.0`.
4. Linux Docker (no Go installed): `npm i -g @howar31/slk && slk --version` → `0.1.0`.
5. `gemini extensions install https://github.com/howar31/slk` — install succeeds; `gemini` session can reference slk skill content.
6. Claude Code session with `~/.claude/skills/slk/SKILL.md` cp'd and `slk` on PATH — agent runs `slk msg read --dry-run` successfully.
7. Any failure: PR 2 is not done.

## Risks & rollback

| Risk | Mitigation |
|---|---|
| `HOMEBREW_TAP_TOKEN` expiry breaks future releases silently | Fine-grained PAT with 1-year expiry; GitHub emails owner 30 days before expiry; release job's brew step fails loudly (`continue-on-error: false`) |
| Corporate proxy blocks postinstall fetch of binary from GitHub | Documented Troubleshooting fallback: download binary manually, drop into a directory on PATH. Native fetch does not honor `HTTP(S)_PROXY` (same constraint as gws) |
| OpenClaw `install:` schema is not `{npm: "<pkg>"}` | Plan's first step verifies schema; ship without the block if necessary, README paths remain functional |
| Unscoped `slk` on npm (existing v0.0.6 "commandline power tool for slack") confuses users | README explicitly states scoped name and links to the npm page; one-line "Why @howar31/slk?" note |
| Bad release tagged | `gh release delete vX.Y.Z --yes && git push --delete origin vX.Y.Z`, fix, re-tag. npm: within 24h `npm unpublish @howar31/slk@X.Y.Z`, after 24h `npm deprecate` + bump patch. Brew: revert the offending commit in the tap repo |
| Local `goreleaser release` accidentally fires against the real remote | New CLAUDE.md rule forbids it; only `--snapshot` allowed locally |

## Out-of-band setup checklist

Captured here so it does not get lost in PR descriptions:

- [ ] `gh repo create howar31/homebrew-tap --public` (PR 1 prerequisite)
- [ ] npmjs.com — register / claim `@howar31` scope (free)
- [ ] npmjs.com — create granular token scoped to `@howar31`, store as `NPM_TOKEN` secret on `howar31/slk`
- [ ] GitHub — create fine-grained PAT, `contents: write` on `howar31/homebrew-tap` (1-year expiry), store as `HOMEBREW_TAP_TOKEN` secret on `howar31/slk`

## Open questions for plan phase

1. Exact OpenClaw `install:` schema — verify against OpenClaw docs / source before encoding in SKILL.md.
2. Whether Gemini extension's `version: "latest"` is acceptable, or whether it must track the slk binary's semver (gws uses `"latest"` — likely fine).
3. Whether to add a smoke-test job that, after publish, installs from npm and runs `slk --version` against the just-shipped tag. Cost: ~30s extra CI; benefit: catches "publish succeeded but package is broken" cases. Recommended yes; finalize in plan.
