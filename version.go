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
