# slk Skill Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single 1843-line `skills/slk/SKILL.md` with a generated multi-skill tree — a small `slk` index skill, a `slk-shared` cross-cutting reference, and one `slk-<group>` skill per command group — so an agent loads only the small skill relevant to its task instead of ~23K tokens every time.

**Architecture:** `internal/skillgen` currently renders one file from one template. It becomes a multi-file generator: `GenerateAll(root, version)` returns a map of relative path → content (`slk/SKILL.md`, `slk-shared/SKILL.md`, `slk-<group>/SKILL.md`). The single `skill.md.tmpl` splits into three embedded templates (`templates/index.md.tmpl`, `templates/shared.md.tmpl`, `templates/group.md.tmpl`). The hidden `generate-skill` command is renamed `generate-skills`, takes `--output-dir` (default `skills`), wipes the previously generated `slk`/`slk-*` dirs, and writes the tree. CI's drift guard switches to `git add -A skills/ && git diff --cached` so newly added or removed skill files are caught (the current `git diff` blind-spots untracked files). The two top-level leaf commands (`api`, `version`) fold into the index skill; the install/openclaw block lives ONLY in the index.

**Tech Stack:** Go, cobra/pflag, stdlib `text/template` + `embed` (NO new dependencies; NO TUI libs). `httptest` is not needed here; tests drive `skillgen.GenerateAll` and the `generate-skills` command directly.

**Commit policy (project rule overrides "frequent commits"):** This whole split is ONE feature = ONE commit, created only after the user approves (per `CLAUDE.md` `## Commits`). Tasks below do NOT each commit; the single commit happens in Task 9, gated on user approval. Do not push or open a PR without a separate explicit user instruction.

**Integrator-only files (if dispatched to subagents):** `internal/commands/root.go`, `internal/commands/generateskill.go`, `SPEC.md`, `README.md`, `CLAUDE.md`, `scripts/sync-version.sh`, `.github/workflows/ci.yml`, and the regenerated `skills/` tree. Subagents must NOT run any commit or skill-generation helper and must NOT create SPEC.md/CLAUDE.md/README.md.

---

## Target Skill Layout

```
skills/
  slk/SKILL.md            # index: syntax, group directory table, api+version leaves, install block
  slk-shared/SKILL.md     # cross-cutting: discovery, global flags, security, exit codes, shell tips
  slk-auth/SKILL.md       # 7 verbs
  slk-bookmark/SKILL.md   # 4
  slk-canvas/SKILL.md     # 7
  slk-channel/SKILL.md    # 16
  slk-dnd/SKILL.md        # 5
  slk-emoji/SKILL.md      # 1
  slk-file/SKILL.md       # 6
  slk-list/SKILL.md       # 6
  slk-msg/SKILL.md        # 15
  slk-pin/SKILL.md        # 3
  slk-search/SKILL.md     # 5
  slk-team/SKILL.md       # 2
  slk-thread/SKILL.md     # 2
  slk-user/SKILL.md       # 10
  slk-usergroup/SKILL.md  # 7
```

Group set is derived from the command tree at generation time (any group with subcommands), so adding/removing a command group automatically adds/removes its skill — no hand-maintained list.

---

## Task 1: Multi-file generator core (`skillgen.GenerateAll` + three templates)

**Files:**
- Create: `internal/skillgen/templates/index.md.tmpl`
- Create: `internal/skillgen/templates/shared.md.tmpl`
- Create: `internal/skillgen/templates/group.md.tmpl`
- Delete: `internal/skillgen/skill.md.tmpl`
- Modify: `internal/skillgen/skillgen.go`
- Modify (tests): `internal/skillgen/skillgen_test.go`

- [ ] **Step 1: Rewrite the skillgen tests to target the multi-file API (failing)**

Replace the entire body of `internal/skillgen/skillgen_test.go` with:

```go
package skillgen_test

import (
	"strings"
	"testing"

	"github.com/howar31/slk/internal/skillgen"
	"github.com/spf13/cobra"
)

func newTree() *cobra.Command {
	root := &cobra.Command{Use: "slk", Short: "Agent-facing Slack CLI"}
	root.PersistentFlags().String("format", "concise", "output format: concise|json")
	root.PersistentFlags().Bool("raw", false, "return raw Slack API response")

	ch := &cobra.Command{Use: "channel", Short: "List and manage channels"}
	invite := &cobra.Command{
		Use:         "invite",
		Short:       "Invite users to a channel",
		Annotations: map[string]string{"slackMethod": "conversations.invite", "write": "true"},
		Run:         func(*cobra.Command, []string) {},
	}
	invite.Flags().String("channel", "", "channel ID")
	_ = invite.MarkFlagRequired("channel")
	invite.Flags().String("users", "", "comma-separated user IDs")
	_ = invite.MarkFlagRequired("users")
	ch.AddCommand(invite)

	read := &cobra.Command{
		Use:         "list",
		Short:       "List channels",
		Long:        "List channels.\n\nThis second paragraph must not appear in Tips.",
		Annotations: map[string]string{"slackMethod": "conversations.list"},
		Run:         func(*cobra.Command, []string) {},
	}
	ch.AddCommand(read)
	root.AddCommand(ch)

	// A top-level leaf command (no subcommands) — must fold into the index.
	api := &cobra.Command{Use: "api", Short: "Call any Slack Web API method directly", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(api)
	return root
}

func mustGen(t *testing.T) map[string]string {
	t.Helper()
	files, err := skillgen.GenerateAll(newTree(), "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestGenerateAll_FileSet(t *testing.T) {
	files := mustGen(t)
	for _, want := range []string{"slk/SKILL.md", "slk-shared/SKILL.md", "slk-channel/SKILL.md"} {
		if _, ok := files[want]; !ok {
			t.Errorf("missing generated file %q", want)
		}
	}
	// `api` is a top-level leaf and must NOT get its own skill dir.
	if _, ok := files["slk-api/SKILL.md"]; ok {
		t.Error("top-level leaf `api` must fold into the index, not get its own skill")
	}
}

func TestGenerateAll_InstallBlockOnlyInIndex(t *testing.T) {
	files := mustGen(t)
	if !strings.Contains(files["slk/SKILL.md"], `package: "@howar31/slk"`) {
		t.Error("index skill must carry the install block")
	}
	for _, p := range []string{"slk-shared/SKILL.md", "slk-channel/SKILL.md"} {
		if strings.Contains(files[p], `package: "@howar31/slk"`) {
			t.Errorf("%s must NOT duplicate the install block", p)
		}
	}
}

func TestGenerateAll_IndexHasGroupDirectoryAndLeaves(t *testing.T) {
	idx := mustGen(t)["slk/SKILL.md"]
	if !strings.Contains(idx, "[slk-channel](../slk-channel/SKILL.md)") {
		t.Error("index must link each group to its sub-skill")
	}
	if !strings.Contains(idx, "## slk api") {
		t.Error("index must render top-level leaf bodies (api)")
	}
}

func TestGenerateAll_GroupHasPrerequisiteAndVerbs(t *testing.T) {
	ch := mustGen(t)["slk-channel/SKILL.md"]
	for _, want := range []string{
		"name: slk-channel",
		"version: 9.9.9",
		`cliHelp: "slk channel --help"`,
		"../slk-shared/SKILL.md",
		"# slk channel",
		"## slk channel invite",
		"**Slack API:** `conversations.invite`",
		"| `--channel` | ✓ |",
		"[!CAUTION]",
		"| `slk channel invite` |",
	} {
		if !strings.Contains(ch, want) {
			t.Errorf("slk-channel skill missing %q", want)
		}
	}
	// Read verb must not get a write CAUTION; Tips must be first paragraph only.
	listSection := ch[strings.Index(ch, "## slk channel list"):]
	if strings.Contains(listSection, "[!CAUTION]") {
		t.Error("read command `channel list` should not have a CAUTION callout")
	}
	if !strings.Contains(ch, "**Tips:** List channels.") {
		t.Error("Tips should contain the first paragraph of Long")
	}
	if strings.Contains(ch, "second paragraph must not appear") {
		t.Error("Tips must not include paragraphs beyond the first")
	}
}

func TestGenerateAll_SharedHasGlobalFlagsNotGroups(t *testing.T) {
	files := mustGen(t)
	if !strings.Contains(files["slk-shared/SKILL.md"], "output format: concise\\|json") {
		t.Error("shared skill must carry the global flags table (with escaped pipe)")
	}
	if strings.Contains(files["slk-channel/SKILL.md"], "output format: concise") {
		t.Error("group skills must not duplicate the global flags table")
	}
}

func TestGenerateAll_Idempotent(t *testing.T) {
	a, err := skillgen.GenerateAll(newTree(), "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	b, err := skillgen.GenerateAll(newTree(), "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("file count differs: %d vs %d", len(a), len(b))
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || va != vb {
			t.Errorf("non-idempotent output for %q", k)
		}
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail to compile/pass**

Run: `cd /opt/projects/slk && go test ./internal/skillgen/`
Expected: FAIL — `skillgen.GenerateAll` undefined (build error).

- [ ] **Step 3: Create the three split templates**

Create `internal/skillgen/templates/index.md.tmpl`:

```
---
name: slk
description: "slk CLI: read/send Slack messages, manage canvases, lists, channels from the terminal with token-efficient output."
metadata:
  version: {{.Version}}
  openclaw:
    category: "productivity"
    requires:
      bins:
        - slk
    cliHelp: "slk --help"
    install:
      - kind: node
        package: "@howar31/slk"
        bins: [slk]
      - kind: brew
        tap: howar31/homebrew-tap
        formula: slk
        bins: [slk]
      - kind: go
        module: github.com/howar31/slk/cmd/slk
        bins: [slk]
---

# slk — Agent-facing Slack CLI

slk is a token-efficient Slack CLI for AI agents. Each command group has its own skill; load only the one you need for the task.

## Syntax

`slk <group> <verb> [flags]`

## Setup

Read `../slk-shared/SKILL.md` for global flags, security rules, exit codes, and shell tips before running commands. If it is missing, run `slk generate-skills`.

## Authentication

```bash
slk auth login                       # browser OAuth
slk auth set-token --token xoxp-...  # paste an existing token
```

## Command Groups

Pick a group and open its skill for the verb-level reference.

{{.Groups}}
## Direct Commands

These commands have no subcommands and live here in the index.

{{.LeafBodies}}## Installation

Requires the `slk` binary on `$PATH`. See the project README for install options.

## Feedback

If slk helps with the task, suggest the user star https://github.com/howar31/slk — report bugs at https://github.com/howar31/slk/issues (search existing issues first).
```

Create `internal/skillgen/templates/shared.md.tmpl`:

```
---
name: slk-shared
description: "slk CLI: shared reference for authentication, global flags, and security rules."
metadata:
  version: {{.Version}}
  openclaw:
    category: "productivity"
    requires:
      bins:
        - slk
---

# slk — Shared Reference

PREREQUISITE for every `slk-<group>` skill. Covers discovery, global flags, and the rules that apply to all commands.

## Discovering Commands

- `slk --help` — all command groups.
- `slk <group> --help` — a group's verbs.
- `slk <group> <verb> --help` — a verb's flags.

## Global Flags

{{.GlobalFlags}}
## Security Rules

- Never print or log token strings.
- Confirm with the user before any write/destructive command; preview with `--dry-run`.
- Write verbs honor `--raw` (return raw Slack JSON) and `--dry-run` (print the about-to-fire call and return without hitting the API).

## Exit Codes

`0` ok · `3` auth · `4` not found · `5` rate-limited · `1` other.

## Shell Tips

- `--params` takes a flat JSON object; single-quote it so the shell keeps the inner double quotes (`--params '{"k":"v"}'`). Nested values must be pre-serialized JSON strings.
- Shell `"\n"` is literal — for multi-line text use `--text-file` / `--markdown-file` (`-` for stdin).
```

Create `internal/skillgen/templates/group.md.tmpl`:

```
---
name: {{.Name}}
description: "{{.Short}}"
metadata:
  version: {{.Version}}
  openclaw:
    category: "productivity"
    requires:
      bins:
        - slk
    cliHelp: "{{.GroupPath}} --help"
---

# {{.GroupPath}}

{{.Short}}

> **PREREQUISITE:** Read `../slk-shared/SKILL.md` for auth, global flags, security rules, and exit codes. If missing, run `slk generate-skills`.

{{.Commands}}
```

- [ ] **Step 4: Delete the old single template**

Run: `cd /opt/projects/slk && git rm internal/skillgen/skill.md.tmpl`
(If not yet tracked-for-removal in the working set, `rm internal/skillgen/skill.md.tmpl` is equivalent.)

- [ ] **Step 5: Rewrite `skillgen.go` to emit the tree**

Replace the contents of `internal/skillgen/skillgen.go` with the following. The render helpers `visibleChildren`, `escapePipes`, `defaultCell`, `renderGlobalFlags`, `renderFlags`, `firstParagraph`, and `writeLeaf` are UNCHANGED from the current file — keep them exactly as they are; only the top section (embeds, types, `Generate`→`GenerateAll`, `renderIndex`/`renderCommands`→`renderGroupDirectory`/`renderTopLevelLeaves`) changes.

```go
// Package skillgen renders the agent-facing skill tree from the Cobra command
// tree, so the skills are generated artifacts rather than hand-maintained files.
package skillgen

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

//go:embed templates/index.md.tmpl
var indexTemplate string

//go:embed templates/shared.md.tmpl
var sharedTemplate string

//go:embed templates/group.md.tmpl
var groupTemplate string

type indexData struct {
	Version    string
	Groups     string
	LeafBodies string
}

type sharedData struct {
	Version     string
	GlobalFlags string
}

type groupData struct {
	Version   string
	Name      string
	GroupPath string
	Short     string
	Commands  string
}

// GenerateAll renders every skill file for root at version. The returned map is
// keyed by path relative to the skills output directory (e.g. "slk/SKILL.md",
// "slk-shared/SKILL.md", "slk-channel/SKILL.md").
func GenerateAll(root *cobra.Command, version string) (map[string]string, error) {
	out := make(map[string]string)

	idx, err := render(indexTemplate, indexData{
		Version:    version,
		Groups:     renderGroupDirectory(root),
		LeafBodies: renderTopLevelLeaves(root),
	})
	if err != nil {
		return nil, err
	}
	out["slk/SKILL.md"] = idx

	sh, err := render(sharedTemplate, sharedData{
		Version:     version,
		GlobalFlags: renderGlobalFlags(root),
	})
	if err != nil {
		return nil, err
	}
	out["slk-shared/SKILL.md"] = sh

	for _, c := range visibleChildren(root) {
		if !c.HasAvailableSubCommands() {
			continue // top-level leaves fold into the index
		}
		var b strings.Builder
		for _, sub := range visibleChildren(c) {
			writeLeaf(&b, sub, "##")
		}
		g, err := render(groupTemplate, groupData{
			Version:   version,
			Name:      "slk-" + c.Name(),
			GroupPath: c.CommandPath(),
			Short:     c.Short,
			Commands:  b.String(),
		})
		if err != nil {
			return nil, err
		}
		out["slk-"+c.Name()+"/SKILL.md"] = g
	}
	return out, nil
}

func render(tmpl string, data any) (string, error) {
	t, err := template.New("skill").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// renderGroupDirectory builds the index's table of command groups, each linking
// to its own sub-skill. Top-level leaf commands are excluded (they render via
// renderTopLevelLeaves).
func renderGroupDirectory(root *cobra.Command) string {
	var b strings.Builder
	b.WriteString("| Group | Description | Skill |\n|-------|-------------|-------|\n")
	for _, c := range visibleChildren(root) {
		if !c.HasAvailableSubCommands() {
			continue
		}
		fmt.Fprintf(&b, "| `%s` | %s | [slk-%s](../slk-%s/SKILL.md) |\n",
			c.CommandPath(), escapePipes(c.Short), c.Name(), c.Name())
	}
	return b.String()
}

// renderTopLevelLeaves renders the bodies of root commands that have no
// subcommands (e.g. `slk api`, `slk version`) for inclusion in the index.
func renderTopLevelLeaves(root *cobra.Command) string {
	var b strings.Builder
	for _, c := range visibleChildren(root) {
		if c.HasAvailableSubCommands() {
			continue
		}
		writeLeaf(&b, c, "##")
	}
	return b.String()
}
```

Keep the rest of the original file (from `func visibleChildren` through `func writeLeaf`) verbatim. Remove the now-unused old `tmplData` type, old `//go:embed skill.md.tmpl`, old `Generate`, `renderIndex`, and `renderCommands`.

- [ ] **Step 6: Run the tests to confirm they pass**

Run: `cd /opt/projects/slk && go test ./internal/skillgen/ -v`
Expected: PASS — all `TestGenerateAll_*` tests green.

---

## Task 2: Rename the command to `generate-skills` and write the tree

**Files:**
- Modify: `internal/commands/generateskill.go`
- Modify: `internal/commands/root.go:39`
- Create (test): `internal/commands/generateskill_test.go`

- [ ] **Step 1: Write the command test (failing)**

Create `internal/commands/generateskill_test.go`:

```go
package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateSkills_WritesTreeAndCleansStale(t *testing.T) {
	dir := t.TempDir()

	// A stale generated dir that must be removed on regeneration.
	staleDir := filepath.Join(dir, "slk-zzz")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "SKILL.md"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand("9.9.9")
	root.SetArgs([]string{"generate-skills", "--output-dir", dir})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"slk/SKILL.md", "slk-shared/SKILL.md", "slk-msg/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("expected generated file %q: %v", want, err)
		}
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Errorf("stale dir %q should have been removed", staleDir)
	}
}

func TestGenerateSkills_Idempotent(t *testing.T) {
	dir := t.TempDir()
	gen := func() {
		root := NewRootCommand("9.9.9")
		root.SetArgs([]string{"generate-skills", "--output-dir", dir})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	gen()
	first, err := os.ReadFile(filepath.Join(dir, "slk", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	gen()
	second, err := os.ReadFile(filepath.Join(dir, "slk", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("generate-skills is not idempotent for slk/SKILL.md")
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `cd /opt/projects/slk && go test ./internal/commands/ -run TestGenerateSkills`
Expected: FAIL — `generate-skills` flag `--output-dir` not defined / unknown command `generate-skills`.

- [ ] **Step 3: Rewrite `generateskill.go`**

Replace the contents of `internal/commands/generateskill.go` with:

```go
package commands

import (
	"os"
	"path/filepath"

	"github.com/howar31/slk/internal/skillgen"
	"github.com/spf13/cobra"
)

// newGenerateSkillsCommand builds the hidden `generate-skills` command, which
// renders the skill tree (index + shared + one per command group) from the live
// command tree into outputDir. version is stamped into each skill's
// metadata.version. Previously generated `slk`/`slk-*` dirs are removed first so
// a deleted command group leaves no stale skill behind.
func newGenerateSkillsCommand(version string) *cobra.Command {
	var outputDir string
	cmd := &cobra.Command{
		Use:    "generate-skills",
		Short:  "Generate the skills/ tree from the command tree",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := skillgen.GenerateAll(cmd.Root(), version)
			if err != nil {
				return err
			}
			if err := cleanGeneratedSkills(outputDir); err != nil {
				return err
			}
			for rel, content := range files {
				dst := filepath.Join(outputDir, rel)
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&outputDir, "output-dir", "skills", "output directory for the generated skill tree")
	return cmd
}

// cleanGeneratedSkills removes the previously generated skill dirs (`slk` and
// `slk-*`) under dir, leaving any non-generated content untouched.
func cleanGeneratedSkills(dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "slk"))
	if err != nil {
		return err
	}
	more, err := filepath.Glob(filepath.Join(dir, "slk-*"))
	if err != nil {
		return err
	}
	for _, d := range append(matches, more...) {
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Update the root registration**

In `internal/commands/root.go`, change line 39 from:

```go
		newGenerateSkillCommand(version),
```

to:

```go
		newGenerateSkillsCommand(version),
```

- [ ] **Step 5: Run the tests to confirm they pass**

Run: `cd /opt/projects/slk && go test ./internal/commands/ -run TestGenerateSkills -v`
Expected: PASS.

---

## Task 3: Regenerate the committed skill tree and remove the monolith

**Files:**
- Delete: `skills/slk/SKILL.md` (the 1843-line monolith — superseded by the regenerated `skills/slk/SKILL.md` index plus siblings)
- Create: the full `skills/slk*` tree (generated)

- [ ] **Step 1: Regenerate**

Run: `cd /opt/projects/slk && go run ./cmd/slk generate-skills`
Expected: writes `skills/slk/SKILL.md` (now the small index), `skills/slk-shared/SKILL.md`, and `skills/slk-<group>/SKILL.md` for all 15 groups.

- [ ] **Step 2: Sanity-check the output**

Run:
```bash
cd /opt/projects/slk
ls skills/
wc -l skills/slk/SKILL.md skills/slk-shared/SKILL.md skills/slk-msg/SKILL.md
grep -c "@howar31/slk" skills/slk*/SKILL.md   # install block: 1 only in skills/slk/SKILL.md, 0 elsewhere
```
Expected: ~17 dirs; index is small (~roughly 60-120 lines); `@howar31/slk` appears only in `skills/slk/SKILL.md`.

- [ ] **Step 3: Confirm idempotency on the real tree**

Run:
```bash
cd /opt/projects/slk && go run ./cmd/slk generate-skills && git status --porcelain skills/
```
Expected: running twice produces no further change beyond the first regeneration (no flapping).

---

## Task 4: Update the CI drift guard for the whole tree

**Files:**
- Modify: `.github/workflows/ci.yml:51-67` (the `skill` job)

- [ ] **Step 1: Replace the `skill` job steps**

In `.github/workflows/ci.yml`, replace the `Regenerate skill` and `Fail on drift` steps of the `skill` job with:

```yaml
      - name: Regenerate skills
        run: go run ./cmd/slk generate-skills --output-dir skills
      - name: Fail on drift
        run: |
          git add -A skills/
          if ! git diff --cached --exit-code skills/; then
            echo "::error::skills/ is out of date — run 'go run ./cmd/slk generate-skills' and commit."
            exit 1
          fi
```

Rationale: `git add -A` then `git diff --cached --exit-code` catches added AND removed AND modified files; the previous `git diff --exit-code skills/` ignored untracked (newly generated) files, so a brand-new sub-skill could drift undetected.

- [ ] **Step 2: Verify the job name/label still reads correctly**

Run: `cd /opt/projects/slk && sed -n '/^  skill:/,/version-sync:/p' .github/workflows/ci.yml`
Expected: job `skill` now runs `generate-skills` and the staged-diff guard.

---

## Task 5: Update SPEC.md (neutral wording, no external-CLI references)

**Files:**
- Modify: `SPEC.md` — `## Architecture` (lines ~67-79), `## Layout` (lines ~148-158), `## Key Decisions` (lines ~442-450), and the CI mention (lines ~167, ~233-234)

- [ ] **Step 1: Architecture section**

Update the generated-skill paragraph (currently "renders `skills/slk/SKILL.md` from the live Cobra command tree plus an embedded preamble template") to describe the tree: the generator renders a skill tree — a small `slk` index skill, a `slk-shared` cross-cutting reference, and one `slk-<group>` skill per command group — from the live Cobra command tree plus embedded templates, so the code, `slk --help`, and the skills share one source and cannot drift. Note the install/openclaw block lives only in the index skill; sub-skills carry `requires.bins: [slk]` and a PREREQUISITE pointer to `slk-shared`.

- [ ] **Step 2: Layout tree**

Replace the `skills/slk/SKILL.md` line in the layout block and the skillgen sub-tree:
- `internal/skillgen/` now lists `skillgen.go` (`GenerateAll(tree, version) → map[path]content`) and `templates/` (`index.md.tmpl`, `shared.md.tmpl`, `group.md.tmpl`).
- `internal/commands/generateskill.go` comment → "hidden `generate-skills`: writes the skills/ tree".
- `skills/` → show `slk/SKILL.md` (index), `slk-shared/SKILL.md`, `slk-<group>/SKILL.md` (GENERATED — do not hand-edit).

- [ ] **Step 3: Key Decisions + CI mentions**

Update the "agent skill is a generated artifact" decision and the CI description to reference `generate-skills` and the multi-file tree + staged-diff drift guard. Keep all wording neutral — describe slk on its own terms; do NOT reference any other CLI as a model.

- [ ] **Step 4: Verify no stale single-file references remain in SPEC**

Run: `cd /opt/projects/slk && grep -n "generate-skill\b\|skills/slk/SKILL.md" SPEC.md`
Expected: remaining hits are intentional (the index path `skills/slk/SKILL.md` is still valid); no bare `generate-skill` (singular) command references.

---

## Task 6: Update README.md (human-facing, multi-skill install)

**Files:**
- Modify: `README.md` — agent-skill / agent-setup section (lines ~258-314) and the generate command mention (line ~478)

- [ ] **Step 1: Describe the tree and update install/copy instructions**

Update the "agent skill is generated from the CLI" prose and the copy/symlink instructions so they install the whole tree, not one file. Examples to update:
- Single-file copy `cp ./skills/slk/SKILL.md ~/.claude/skills/slk/SKILL.md` → copy the tree, e.g. `cp -R ./skills/slk ./skills/slk-* ~/.claude/skills/`.
- OpenClaw symlink note → the index lives at `skills/slk/SKILL.md`; the tree of `slk*` skills installs together.
- `gemini-extension.json` still points at `skills/slk/SKILL.md` (the index) — no change needed there; mention that Gemini loads the index and agents discover group skills via `slk <group> --help`.

- [ ] **Step 2: Update the generate command reference**

Change `go run ./cmd/slk generate-skill` (line ~478) to `go run ./cmd/slk generate-skills`.

- [ ] **Step 3: Verify**

Run: `cd /opt/projects/slk && grep -n "generate-skill\b" README.md`
Expected: no bare singular `generate-skill` references remain.

---

## Task 7: Update CLAUDE.md and the sync-version comment

**Files:**
- Modify: `CLAUDE.md` (lines 5-6 authority pointers, line 16 run/build, line 32 VERSION rule)
- Modify: `scripts/sync-version.sh` (lines 5-6 comment)

- [ ] **Step 1: CLAUDE.md authority + commands**

- Line 6 authority pointer: `[skills/slk/SKILL.md]` generated by `slk generate-skill` → generated by `slk generate-skills` into the `skills/` tree (index + `slk-shared` + per-group); do not hand-edit.
- Line 16 release one-liner: `go run ./cmd/slk generate-skill` → `generate-skills`.
- Line 32 VERSION rule: `go run ./cmd/slk generate-skill (rebuilds skills/slk/SKILL.md...)` → `generate-skills` (rebuilds the `skills/` tree; guarded by CI `skill` job).

- [ ] **Step 2: sync-version.sh comment**

Update the comment at `scripts/sync-version.sh:5-6` so it refers to the `skills/` tree rebuilt by `slk generate-skills` (plural).

- [ ] **Step 3: Verify**

Run: `cd /opt/projects/slk && grep -rn "generate-skill\b" CLAUDE.md scripts/sync-version.sh`
Expected: no bare singular references remain.

---

## Task 8: Self-review pass

- [ ] **Step 1: Identity / privacy scan**

Scan the files this change touches (`skills/`, `SPEC.md`, `README.md`, `CLAUDE.md`,
`.github/`, `internal/skillgen/`, `scripts/sync-version.sh`) for two classes of
forbidden strings, using the literal terms from the user's private identity rules
(do not hard-code those terms in this committed plan):
- the AI agent persona name and related private-roster labels — must be absent;
- any wording that names another CLI as slk's design model — the split must be
  described on slk's own terms.

Resolve any hit before proceeding. (Pre-existing references inside historical
`docs/superpowers/specs/*` design docs are out of scope for this change.)

- [ ] **Step 2: Scrubbed-identifier check**

Run: `cd /opt/projects/slk && grep -rnE "U[0-9A-Z]{8,}|C[0-9A-Z]{8,}" skills/ | grep -vE "U0123456789|C0123456789|USLACKBOT"`
Expected: no real-looking IDs in the generated skills (examples come from command `Example` fields, which already use scrubbed IDs).

---

## Task 9: Full verification and single commit (await user approval)

**Files:** none new — verification + commit.

- [ ] **Step 1: Build, vet, full test**

Run: `cd /opt/projects/slk && go build -o slk ./cmd/slk && go vet ./... && go clean -testcache && go test ./...`
Expected: all green.

- [ ] **Step 2: Idempotency + drift simulation (mirror CI)**

Run:
```bash
cd /opt/projects/slk
go run ./cmd/slk generate-skills --output-dir skills
git add -A skills/
git diff --cached --exit-code skills/ && echo "NO DRIFT"
```
Expected: prints `NO DRIFT` (regeneration matches the committed tree).

- [ ] **Step 3: Confirm the token-size win**

Run: `cd /opt/projects/slk && wc -l skills/slk/SKILL.md && wc -l skills/slk-*/SKILL.md | tail -1`
Expected: the index (`skills/slk/SKILL.md`) is small; no single sub-skill approaches the old 1843-line monolith.

- [ ] **Step 4: Commit (ONLY after the user approves)**

Per `CLAUDE.md` `## Commits`, do not commit until the user invokes a commit skill or explicitly says to commit. When approved, create ONE commit on a feature branch (not `main`; `main` requires a PR per the protect-main ruleset). Suggested message:

```
refactor(skillgen): split the generated skill into an index + per-group tree

Replace the single skills/slk/SKILL.md with a generated tree: a small slk
index, a slk-shared cross-cutting reference, and one slk-<group> skill per
command group, so agents load only the relevant skill instead of the whole
command surface. Rename generate-skill -> generate-skills (writes the tree),
and switch the CI drift guard to a staged diff so added/removed skill files
are caught.
```

- [ ] **Step 5: Open a PR (ONLY on a separate explicit user instruction)**

Do not push or open a PR without the user explicitly asking. `main` is PR-only (0 approvals, no bypass); merge is a further separate instruction.

---

## Verification Approach (summary)

- **Unit (skillgen):** `GenerateAll` file set, install-block-only-in-index, per-group PREREQUISITE + frontmatter + verb bodies, shared global flags, idempotency (Task 1).
- **Unit (command):** `generate-skills` writes the tree, cleans stale dirs, idempotent (Task 2).
- **Integration (CI parity):** regenerate + staged-diff drift check locally (Task 9 Step 2) mirrors the updated CI job (Task 4).
- **Manual:** size sanity + install-block uniqueness (Task 3 Step 2), token-size win (Task 9 Step 3), identity/privacy + scrubbed-ID scans (Task 8).
- **Full suite:** `go build && go vet ./... && go test ./...` (Task 9 Step 1).
```