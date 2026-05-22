---
name: slk
description: "slk CLI: read/send Slack messages, manage canvases, lists, channels from the terminal with token-efficient output."
metadata:
  version: 0.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - slk
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

## Syntax

`slk <group> <verb> [flags]`

Groups: `auth`, `msg`, `thread`, `search`, `canvas`, `list`, `channel`, `user`,
`api`, `version`.

For the authoritative command set, ask the binary — its help is auto-generated
from the registered commands and never drifts from this doc:

- `slk --help` — all groups.
- `slk <group> --help` — a group's verbs (e.g. `slk msg --help`).
- `slk <group> <verb> --help` — a verb's flags (e.g. `slk msg send --help`).

The examples below are only the common subset; consult `--help` for anything
not shown here.

## Global flags

| Flag | Description |
|------|-------------|
| `--format` | `concise` (default), `json`, `jsonl`, `table` |
| `--as` | `user` (default) or `bot` |
| `--profile` | config profile to use |
| `--raw` | return the raw Slack API response |
| `--dry-run` | validate a write without calling the API |
| `--no-resolve` | do not resolve IDs to names |

## Common commands

```bash
slk msg read --channel C123 --limit 20
slk msg send --channel C123 --text "hello"
slk thread reply --channel C123 --thread 1779191572.0 --text "hi"
slk search channels
slk canvas create --title "Plan" --markdown "# Heading"
slk list create --title "Backlog"
slk channel invite --channel C123 --users U1,U2
slk api <method> --params '{"k":"v"}'   # escape hatch for any method
```

`slk api` reaches any Web API method. `--params` is a flat JSON object sent as
form fields, so nested values must be pre-serialized JSON strings; use `--json`
to send a raw JSON request body instead.

## Multi-line content

Shell `"\n"` is literal. For multi-line markdown/text pass a file or stdin:

```
slk canvas update --id F0… --action prepend --markdown-file entry.md
cat entry.md | slk msg send --channel C0… --text-file -
```

File-flag pairs: `--markdown` / `--markdown-file` (canvas), `--text` / `--text-file` (msg/thread).

## Drafts

`msg draft` creates one-shot; list/delete only work in the Slack client UI
(public API doesn't grant draft management to xoxp tokens). The output URL
opens the channel where the draft is attached.

## Security rules

- Never print tokens. They live in `~/.config/slk/config.toml` (mode 0600).
- Confirm with the user before any write command (`send`, `create`, `archive`,
  `delete`, etc.). Use `--dry-run` first to preview.

## Exit codes

`0` ok · `3` auth error · `4` not found · `5` rate limited · `1` other (includes
bad args / usage errors).
