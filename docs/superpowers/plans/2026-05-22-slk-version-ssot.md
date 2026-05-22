# slk Version SSOT (VERSION file) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a committed `VERSION` file the single source of truth for the slk version, embedded into the binary at compile time, so `slk --version`, the release tag, and (later) the generated skill all derive from one value.

**Architecture:** A root-level `VERSION` file holds the canonical semver. A new root package file `version.go` (`package slk`) embeds it via `//go:embed VERSION` and exposes `slk.Version`. `cmd/slk/main.go` uses `slk.Version` instead of an `-ldflags`-injected variable. The release workflow asserts the pushed tag matches `VERSION`.

**Tech Stack:** Go 1.25, `//go:embed`, Cobra (unchanged), GitHub Actions, goreleaser.

---

## File structure

- Create: `VERSION` — the canonical version string (one line, no leading `v`).
- Create: `version.go` (module root, `package slk`) — embeds and exposes `Version`.
- Create: `version_test.go` (module root, `package slk`) — asserts `Version` tracks the file and is semver.
- Modify: `cmd/slk/main.go` — use `slk.Version`; drop the `dev` var.
- Modify: `.goreleaser.yaml` — drop the now-unused `-X main.version` ldflag.
- Modify: `.github/workflows/release.yml` — add a `tag == VERSION` guard.
- Modify: `CLAUDE.md`, `SPEC.md` — update build commands that reference `-ldflags`.

---

### Task 1: VERSION file and embedding package

**Files:**
- Create: `VERSION`
- Create: `version.go`
- Test: `version_test.go`

- [ ] **Step 1: Write the failing test**

Create `version_test.go`:

```go
package slk

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestVersion_MatchesVersionFile(t *testing.T) {
	data, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	want := strings.TrimSpace(string(data))
	if Version != want {
		t.Errorf("Version = %q, want %q", Version, want)
	}
}

func TestVersion_IsSemver(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(Version) {
		t.Errorf("Version %q is not semver", Version)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestVersion -v`
Expected: FAIL — `undefined: Version` (and `VERSION` file missing).

- [ ] **Step 3: Create the VERSION file**

Create `VERSION` with exactly this content (current released version; one trailing newline):

```
0.2.1
```

- [ ] **Step 4: Create the embedding package**

Create `version.go`:

```go
// Package slk exposes build-wide metadata for the slk CLI.
package slk

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionFile string

// Version is the canonical slk version, read from the committed VERSION file.
// It is the single source of truth for the running version; the release tag
// must match it (enforced in the release workflow).
var Version = strings.TrimSpace(versionFile)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test . -run TestVersion -v`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add VERSION version.go version_test.go
git commit -m "feat(version): add VERSION file as the version source of truth"
```

---

### Task 2: Wire main.go to the embedded version

**Files:**
- Modify: `cmd/slk/main.go`

- [ ] **Step 1: Replace the build-time var with the embedded version**

In `cmd/slk/main.go`, remove the `version` var and its comment, add the root
package import, and pass `slk.Version`. Result:

```go
// Command slk is an agent-facing Slack CLI.
package main

import (
	"errors"
	"fmt"
	"os"

	slk "github.com/howar31/slk"
	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/auth"
	"github.com/howar31/slk/internal/commands"
)

func main() {
	root := commands.NewRootCommand(slk.Version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "slk:", err)
		var apiErr *api.APIError
		if errors.As(err, &apiErr) {
			os.Exit(apiErr.ExitCode())
		}
		var authErr *auth.AuthError
		if errors.As(err, &authErr) {
			os.Exit(authErr.ExitCode())
		}
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build and run to verify the version comes from VERSION**

Run:
```bash
go build -o /tmp/slk ./cmd/slk && /tmp/slk version
```
Expected: `slk 0.2.1`

- [ ] **Step 3: Verify the full test suite still passes**

Run: `go test ./...`
Expected: PASS (no package references a removed `main.version`).

- [ ] **Step 4: Commit**

```bash
git add cmd/slk/main.go
git commit -m "feat(version): read version from embedded VERSION instead of ldflags"
```

---

### Task 3: Drop the unused ldflag from goreleaser and build docs

**Files:**
- Modify: `.goreleaser.yaml`
- Modify: `CLAUDE.md`
- Modify: `SPEC.md`

- [ ] **Step 1: Remove `-X main.version` from goreleaser**

In `.goreleaser.yaml`, the build `ldflags` line currently reads:

```yaml
      - -s -w -X main.version={{.Version}}
```

Change it to (the version now comes from the embedded VERSION file, which
goreleaser checks out at the tagged commit):

```yaml
      - -s -w
```

- [ ] **Step 2: Update the build command in CLAUDE.md**

In `CLAUDE.md`, under `## Run / build`, change:

```bash
go build -ldflags "-X main.version=0.1.0" -o slk ./cmd/slk
```

to:

```bash
go build -o slk ./cmd/slk   # version comes from the committed VERSION file
```

- [ ] **Step 3: Update the build command in SPEC.md**

In `SPEC.md`, under `## Verification`, change the Build line:

```
- Build: `go build -ldflags "-X main.version=<v>" -o slk ./cmd/slk`
```

to:

```
- Build: `go build -o slk ./cmd/slk` (version is read from the embedded `VERSION` file)
```

- [ ] **Step 4: Verify goreleaser config is still valid**

Run: `goreleaser check`
Expected: `1 configuration file(s) validated`

- [ ] **Step 5: Commit**

```bash
git add .goreleaser.yaml CLAUDE.md SPEC.md
git commit -m "chore(version): drop main.version ldflag now that VERSION is embedded"
```

---

### Task 4: Release workflow asserts tag matches VERSION

**Files:**
- Modify: `.github/workflows/release.yml`

- [ ] **Step 1: Add a tag/VERSION guard step**

In `.github/workflows/release.yml`, in the main release job, add this step
immediately after the `actions/checkout` step (before the goreleaser run):

```yaml
      - name: Verify tag matches VERSION
        run: |
          file_version="$(cat VERSION)"
          tag_version="${GITHUB_REF_NAME#v}"
          if [ "$file_version" != "$tag_version" ]; then
            echo "::error::tag $GITHUB_REF_NAME does not match VERSION ($file_version)"
            exit 1
          fi
```

- [ ] **Step 2: Verify the workflow YAML parses**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml')); print('ok')"`
Expected: `ok`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci(release): fail the release if the tag disagrees with VERSION"
```

---

## Release procedure (documentation note, not a code step)

From now on, cutting a release is:

1. Edit `VERSION` to the new semver (e.g. `0.3.0`), commit on a branch, merge via PR.
2. `git tag v0.3.0 && git push origin v0.3.0` (tag must equal `VERSION`).

The existing tag-driven GHA release fires as before; the new guard rejects a
mismatch. `slk --version` and the npm smoke test both report the embedded
`VERSION`, which equals the tag.

---

## Self-review

- **Spec coverage:** Implements design §8 (Version SSOT) — `VERSION` file (Task 1),
  `go:embed` + drop ldflags (Tasks 1–3), tag==VERSION guard (Task 4). The
  `generate-skill` consumption of the version is in Plan 2.
- **Placeholder scan:** none — all code and commands are concrete.
- **Type consistency:** `slk.Version` (exported, root package) used consistently
  in `version.go` and `cmd/slk/main.go`.
- **Note:** `internal/commands/version.go`'s `isDevBuild` is unchanged; with an
  embedded version it is effectively never `"dev"` (only the empty-string guard
  remains), which is harmless and needs no edit.
