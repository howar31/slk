#!/usr/bin/env bash
# Fan the VERSION single source of truth into the committed plain-text file
# that carries a literal copy of it: SECURITY.md.
#
# The skills/ tree is NOT handled here. It is a generated artifact, rebuilt
# wholesale from the Cobra command tree by `slk generate-skills` (which stamps the
# version into every skill along the way); run that separately and let CI's `skill`
# job guard it. This script is purely the version fan-out for SECURITY.md, the
# one file that nothing else generates.
#
# Also intentionally NOT here (they derive automatically): the binary's version
# (//go:embed at compile time), and the git tag plus the published npm version
# (derived in CI at release time; npm/package.json's committed 0.0.0 is a
# placeholder).
set -euo pipefail

cd "$(dirname "$0")/.."

version="$(cat VERSION)"
minor="$(cut -d. -f1,2 VERSION)" # e.g. 0.3 — the supported patch series

# SECURITY.md — the supported-versions table tracks the current patch series.
tmp="$(mktemp)"
sed -E \
	-e "s/^\| [0-9]+\.[0-9]+\.x /| ${minor}.x /" \
	-e "s/^\| < [0-9]+\.[0-9]+ /| < ${minor} /" \
	SECURITY.md >"$tmp"
mv "$tmp" SECURITY.md

echo "Fanned VERSION ${version} into SECURITY.md."
