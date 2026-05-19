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
---

# slk — Agent-facing Slack CLI

## Syntax

`slk <group> <verb> [flags]`

Groups: `auth`, `msg`, `thread`, `search`, `canvas`, `list`, `channel`, `user`,
`api`.

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

## Security rules

- Never print tokens. They live in `~/.config/slk/config.toml` (mode 0600).
- Confirm with the user before any write command (`send`, `create`, `archive`,
  `delete`, etc.). Use `--dry-run` first to preview.

## Exit codes

`0` ok · `2` bad args · `3` auth error · `4` not found · `5` rate limited ·
`1` other.
