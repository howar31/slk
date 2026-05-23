#!/usr/bin/env bash
# Fan the VERSION single source of truth into the committed plain-text files
# that carry a literal copy of it: gemini-extension.json and SECURITY.md.
#
# skills/slk/SKILL.md is NOT handled here. It is a generated artifact, rebuilt
# wholesale from the Cobra command tree by `slk generate-skill` (which stamps the
# version as one field along the way); run that separately and let CI's `skill`
# job guard it. This script is purely the version fan-out for the two files that
# nothing else generates.
#
# Also intentionally NOT here (they derive automatically): the binary's version
# (//go:embed at compile time), and the git tag plus the published npm version
# (derived in CI at release time; npm/package.json's committed 0.0.0 is a
# placeholder).
set -euo pipefail

cd "$(dirname "$0")/.."

version="$(cat VERSION)"
minor="$(cut -d. -f1,2 VERSION)" # e.g. 0.3 — the supported patch series

# gemini-extension.json — manifest version (SemVer; drives the
# `gemini extensions update` comparison, which a literal "latest" defeats).
tmp="$(mktemp)"
jq --arg v "$version" '.version = $v' gemini-extension.json >"$tmp"
mv "$tmp" gemini-extension.json

# SECURITY.md — the supported-versions table tracks the current patch series.
tmp="$(mktemp)"
sed -E \
	-e "s/^\| [0-9]+\.[0-9]+\.x /| ${minor}.x /" \
	-e "s/^\| < [0-9]+\.[0-9]+ /| < ${minor} /" \
	SECURITY.md >"$tmp"
mv "$tmp" SECURITY.md

echo "Fanned VERSION ${version} into gemini-extension.json and SECURITY.md."
