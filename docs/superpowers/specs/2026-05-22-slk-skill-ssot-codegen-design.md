# slk — Skill as a Generated SSOT Artifact — Design

Date: 2026-05-22
Status: approved (brainstorming), pending implementation plan

## 1. Purpose

Make the `slk` binary the single source of truth (SSOT) for its agent-facing
skill. A new `slk generate-skill` command emits the complete `SKILL.md` from the
binary's own Cobra command tree plus a small binary-owned template, so the three
agent-facing surfaces — the code, `slk --help`, and `SKILL.md` — can no longer
drift from each other. The skill version is derived from one committed version
of record, ending the current binary/skill version drift.

This is a binary-first design: the CLI describes itself, and every agent's skill
is a derived artifact.

## 2. Background: current state and the drift problem

- `SKILL.md` is hand-written at `skill/SKILL.md`. It restates command names,
  flags, and exit codes that Cobra already knows, and it carries curated prose
  (Slack traps, security rules, multi-line guidance) that lives nowhere else.
- `slk --help` is already generated from the Cobra tree, so it is already
  SSOT-linked to the code. `SKILL.md` is the only surface that drifts — PR #17
  fixed one such drift (a non-existent `2 bad args` exit code, stale command
  coverage).
- Skill `metadata.version` is `0.1.0` while the binary tag is `v0.2.1`. Nothing
  syncs them; the skill version was forgotten across the v0.2.0 / v0.2.1
  releases.
- Distribution: `SKILL.md` is not shipped in the goreleaser tarball. It reaches
  agents straight from the repo — via the `skills` tool (`npx skills add <repo>`)
  and via OpenClaw reading the frontmatter. The repo ref (default `main`) is
  therefore the live skill. The README install guide never mentions
  `npx skills add`, and the skill lives at the non-standard `skill/` (singular)
  path.

## 3. Goals and non-goals

Goals:
- One source for the skill: `slk generate-skill` produces `SKILL.md` entirely.
- One source for the version: a committed version of record feeds
  `slk --version`, `npm/package.json`, and `SKILL.md` `metadata.version`.
- Unify `--help` and `SKILL.md`: per-command prose lives in Cobra annotations so
  both surfaces share it.
- First-class installation: `skills/slk/SKILL.md` standard layout +
  `npx skills add` documented as the primary path.
- Hard drift guard in CI.

Non-goals (YAGNI; scoped for a single-API CLI):
- No multi-skill taxonomy (no surface/action/persona/recipe/workflow split).
- No Changesets, no scheduled auto-regeneration, no external skill-registry
  publish step.
- No `slk schema`: Slack has no live, machine-readable Discovery API; the only
  published OpenAPI spec, `slackapi/slack-api-specs`, was archived in 2021.
- README is not generated; it stays hand-written.

## 4. Key decisions

1. **Full generation.** The entire `SKILL.md` is generated; no hand-maintained
   `SKILL.md` remains as a deliverable.
2. **Single file.** slk is one focused Slack CLI, so the skill is one file,
   `skills/slk/SKILL.md`, listing all commands as preamble + per-command
   sections within that one file. (Revisit only if the command count grows
   dramatically.)
3. **Curated prose homes (the SSOT boundary):**
   - Prose attached to a single command (Slack traps, the `api` nested-params
     trap, draft lifecycle) → that command's Cobra `Long` / `Example`. Feeds
     both `--help` and `SKILL.md`.
   - Cross-cutting agent guidance (security rules, concise-by-default, shell
     tips, syntax, global flags, discovering-commands, auth) → a binary-owned
     embedded template, emitted as the preamble. The agent still sees the safety
     guardrails; it is still SSOT.
   - Pure design rationale (the "why" behind a decision) → `SPEC.md`, human-only,
     not emitted into the skill.
4. **Version SSOT.** Introduce one committed version of record; bumping it is the
   release action. All version-bearing artifacts derive from it. This also lets
   `generate-skill` produce a deterministic, reproducible file (no build-time
   `-ldflags` value leaking `dev` into the committed skill).
5. **Hard CI drift fail.** slk's skill derives only from its own code, so any
   drift is a developer omission and must fail CI (not merely warn).

## 5. The `slk generate-skill` command

- A hidden utility subcommand (`Hidden: true`) so it does not clutter normal
  user help.
- Flags: `--output` (default `skills/slk/SKILL.md`).
- Pure local operation: no Slack call, no token. Walks the root command tree,
  assembles markdown, writes the file.
- Reads the version of record (see §8) to stamp `metadata.version`.

## 6. Generated SKILL.md structure (single file)

Frontmatter:
- `name: slk`
- `description: <one-liner>`
- `metadata.version: <version of record>`
- `metadata.openclaw`: `category`, `requires.bins: [slk]`, the `install:` block
  (npm / brew / go), and `cliHelp: "slk --help"`.

Preamble (file-level, from the binary-owned embedded template; global flags and
the command index are generated from the Cobra tree):
- `## Syntax`
- `## Discovering Commands` — `slk --help`, `slk <group> --help`,
  `slk <group> <verb> --help`.
- `## Global Flags` — generated from persistent flags.
- `## Authentication` — brief `slk auth login` / `set-token` note.
- `## Security Rules` — never print tokens; confirm before write/destructive
  commands; prefer `--dry-run`.
- `## Shell Tips` — `--params` JSON quoting; literal `"\n"` → use file/stdin.
- `## Installation` — one line pointing to the README (install prose SSOT =
  README; this avoids README↔SKILL duplication).
- `## Commands` — index table of every command with its one-line `Short`
  (generated from Cobra `Use` + `Short`; same data `--help` lists).

Per-command sections, grouped by command group, generated from the Cobra tree:

```
## <group>            (from the group command's Short)
### slk <group> <verb>
<description from Short>
**Slack API:** `<method>`     (from the command's slackMethod annotation)
<Usage code block from Use>
<Flags table: Flag | Required | Default | Description>
<Examples from Example>
**Tips:** <curated prose from Long>
> [!CAUTION]                    (only for write commands)
> Write command — confirm with the user before executing; preview with --dry-run.
```

Footer:
- `## Feedback` — one line instructing the agent to suggest the user star
  `https://github.com/howar31/slk` and to file bugs (search existing issues
  first).

## 7. Per-command content sourcing (Cobra annotations)

Each command carries the information the generator needs, co-located with the
command definition so it stays SSOT:
- `Use` → command name + usage line; also the index entry.
- `Short` → one-line description (index + section intro).
- `Long` → curated Tips prose (also shown by `--help`).
- `Example` → the Examples block (also shown by `--help`).
- Flags → the Flags table; `Required` is derived from `MarkFlagRequired`,
  `Default` from the flag's default value.
- `Annotations["slackMethod"]` → the `**Slack API:**` line, set next to the
  command's `client.Call("<method>", …)` (the generator runs from the compiled
  binary and cannot read source, so the method must be recorded as an
  annotation).
- `Annotations["write"]` (or equivalent boolean) → marks write/destructive
  commands so the generator emits the `CAUTION` callout. Reads omit it.

This pushes slk's currently scattered Slack traps (README "Known Slack-side
limitations", SPEC, CLAUDE.md) into the relevant command's `Long` as their SSOT;
README may keep a short human summary, accepted as low-churn overlap.

## 8. Version SSOT

Introduce a single committed `VERSION` file at the repo root as the version of
record. The binary embeds it via `//go:embed VERSION`, so `slk --version` and
`generate-skill` read the same value and the build-time `-ldflags -X
main.version` injection is dropped. Everything derives from `VERSION`:
- `slk --version` (from the embedded `VERSION`).
- `npm/package.json` `version` (already synced from the tag today; re-point to
  the version of record).
- `SKILL.md` `metadata.version`, stamped by `generate-skill`.

Bumping `VERSION` (and committing it) is the release action; the existing
tag-driven GHA release continues to fire on `vX.Y.Z`, and the tag must match
`VERSION`. Because `generate-skill` reads the committed `VERSION` (not a
build-time value), regenerating in CI reproduces exactly the committed file, so
the drift check is stable.

## 9. Repo layout and installation alignment

- Move `skill/SKILL.md` → `skills/slk/SKILL.md` (the `skills` tool's first-class
  layout). Update path references: `gemini-extension.json` `contextFileName`,
  `CLAUDE.md`, `SPEC.md` Layout.
- README "Agent setup": add `npx skills add https://github.com/howar31/slk` as
  the primary, cross-agent method; OpenClaw via symlink
  `ln -s $(pwd)/skills/slk ~/.openclaw/skills/`; keep the Gemini extension;
  demote manual `cp` to a fallback.

## 10. Drift prevention (CI)

Add a job to `.github/workflows/ci.yml`: run `slk generate-skill`, then
`git diff --exit-code skills/`. Any difference fails the build, forcing the
developer to regenerate after changing a command.

## 11. Skill content conventions

The generated skill uses these conventions, all sourced from the binary:
- `cliHelp` frontmatter field pointing to `slk --help`.
- A rich Flags table with `Required` and `Default` columns.
- A `Discovering Commands` preamble section for the `--help` tiers.
- Per-command `Tips` from Cobra `Long`.
- A per-command `CAUTION` callout for write/destructive commands.
- A per-command `**Slack API:** <method>` line sourced from the command's
  `slackMethod` annotation (slk's own code), which also strengthens the
  `slk api` escape hatch.
- An in-file `## Commands` index table.
- A one-line `Feedback` footer (star the repo, file issues).

slk keeps its frontmatter `install:` block so OpenClaw can auto-install the
binary when it is missing.

## 12. Verification

- Golden test for `generate-skill`: render against the live command tree and
  diff against a committed golden `SKILL.md`; the CI drift job is the
  integration form of this.
- Unit coverage for the generator helpers (frontmatter, flags table incl.
  required/default extraction, write→CAUTION, slackMethod line).
- Manual: run `npx skills add <repo>` against the new `skills/slk/` layout and
  confirm the skill installs into `~/.claude/skills/`.

## 13. Open questions / risks

- Release wiring must keep the git tag and `VERSION` in agreement (a CI check
  asserting `tag == VERSION` is a candidate guard) — detail for the plan.
- Tagging every command write/read and adding `slackMethod` annotations touches
  all command files; mechanical but broad.
- Single-file size grows with the command count; acceptable now (~33 commands),
  revisit if it balloons.
