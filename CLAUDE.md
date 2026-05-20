# slk — agent rules

## Authority
- Architecture, layout, conventions, decisions → **[SPEC.md](SPEC.md)** (SSOT).
- Human-facing usage → [README.md](README.md).
- Agent-facing skill prompt → [skill/SKILL.md](skill/SKILL.md).
- Original design + plan → `docs/superpowers/specs/2026-05-19-slack-cli-design.md` and `docs/superpowers/plans/2026-05-19-slk-slack-cli.md`.

This file is index-only; do not duplicate SPEC.md content here.

## Run / build
```bash
go build -ldflags "-X main.version=0.1.0" -o slk ./cmd/slk
go test ./...                               # uncached → prefix with go clean -testcache
go test ./internal/commands/ -run TestX -v  # focused
goreleaser release --clean                  # darwin/linux × amd64/arm64
```

## Conventions
- Code comments + test names: English. See SPEC.md `## Conventions`.
- Doc language: SPEC.md / CLAUDE.md / this file in English; README.md mixes per audience.
- One file per command group under `internal/commands/`, paired `_test.go` with dry-run + flag-registration checks.
- Response-parsing helpers (`parseListCreateID`, `messageDisplay`, `injectRowID`, `fetchChannelsWith`, …) live alongside the command and are unit-tested via `httptest.NewServer` against `api.Client` with `BaseURL` overridden.
- Write verbs MUST honor `--raw` (return raw Slack JSON when set) and `--dry-run` (print the about-to-fire call and return without hitting the API).
- Exit codes: `0` ok · `3` auth · `4` not found · `5` rate-limited · `1` other. Mappings in `internal/api/errors.go`.

## Workflow rules
- Never commit automatically. Always present a summary and wait for explicit approval. Conventional commits (`feat:` / `fix:` / `refactor:` / `docs:` / `chore:`).
- `main` is the public branch (`github.com/howar31/slk`). Do not push without explicit approval.
- Test fixtures and example identifiers use the scrubbed convention: `Alice` / `Bob` / `C0123456789` / `U0123456789` / `F01234567`. Do NOT introduce real names or real channel/user IDs into committed code or docs.
- When dispatching subagents for code work, explicitly tell them: "Do NOT run any commit-helper or documentation skill. Do NOT create SPEC.md, CLAUDE.md, or top-level README.md."
- Tokens live in `~/.config/slk/config.toml` (mode 0600). Never print or log token strings.

## Slack-side traps (documented in README + SPEC)
- `chat.deleteScheduledMessage` may return ok=true for schedules within ~5 min of `post_at` yet still fire.
- `chat.delete` on a self-DM returns `internal_error`; UI-only deletion.
- `slackLists.delete` is not in the public API — lists must be cleaned via the Slack UI.
- `slk api --params` nested objects must be pre-serialized JSON strings (form-urlencoded transport).
- Slackbot is special-cased: Slack returns `is_bot=false` for it, so user-list filters key off the literal ID `USLACKBOT`.
