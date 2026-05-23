# slk API Coverage Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add ~63 curated verbs covering every OAuth-reachable Slack Web API method slk does not yet wrap (everything the final 36-scope user-token set authorizes, minus deprecated / Grid-only / xoxc-only / app-framework methods), and expand the default OAuth scope set 22 → 36 so they work out-of-box.

**Architecture:** One file per command group under `internal/commands/`, each with a paired `_test.go`. New verbs follow four archetypes (A1 simple-write, A2 single-read, A3 paginated-read, A4 two-step-upload). Seven new groups (`file`, `pin`, `bookmark`, `team`, `emoji`, `dnd`, `usergroup`) get new files; six existing groups (`msg`, `channel`, `user`, `canvas`, `list`, `search`) gain verbs; `auth` gains two verbs alongside the scope change. `SKILL.md` is regenerated from the command tree at the end.

**Tech Stack:** Go, cobra, stdlib `net/http` (incl. multipart for file/photo upload — NO new dependencies; NO TUI libs — TUI is reserved for human onboarding only, never command paths), `httptest` for tests.

**Integrator-only files (NOT for subagents):** `auth.go` (Task 0 scope + auth verbs), `root.go` (group registration), `SKILL.md`/`README.md`/`SPEC.md` (Task 10). Subagents must NOT touch SPEC.md/CLAUDE.md/README.md, must NOT run any commit or skill-generation helper, and must use scrubbed identifiers (`Alice`/`Bob`/`C0123456789`/`U0123456789`/`F01234567`).

---

## Final Scope Set (36)

Existing 22 + **9** (`reactions:read`, `files:write`, `users.profile:write`, `pins:read`, `pins:write`, `bookmarks:read`, `bookmarks:write`, `team:read`, `emoji:read`) + **5** (`users:write`, `dnd:read`, `dnd:write`, `usergroups:read`, `usergroups:write`).

`D` below = already in the original 22; `+` = added by Task 0.

## Final Command Inventory (63 verbs)

`W` = write (honor `--dry-run` + `--raw`). `R` = read.

### `msg` (existing file, +8)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `unreact` | reactions.remove | W | reactions:write D | A1 | channel, ts→timestamp, emoji→name |
| `unschedule` | chat.deleteScheduledMessage | W | chat:write D | A1 | channel, id→scheduled_message_id (Long: 5-min race) |
| `scheduled` | chat.scheduledMessages.list | R | chat:write D | A3 | channel (opt) |
| `permalink` | chat.getPermalink | R | none | A2 | channel, ts→message_ts |
| `ephemeral` | chat.postEphemeral | W | chat:write D | A1 | channel, user, text/text-file |
| `me` | chat.meMessage | W | chat:write D | A1 | channel, text |
| `reactions` | reactions.get | R | reactions:read + | A2 | channel, ts→timestamp |
| `reacted` | reactions.list | R | reactions:read + | A3 | user (opt) |

### `channel` (existing file, +11)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `info` | conversations.info | R | channels:read D | A2 | channel |
| `members` | conversations.members | R | channels:read D | A3 | channel |
| `join` | conversations.join | W | channels:write D | A1 | channel |
| `leave` | conversations.leave | W | channels:write D | A1 | channel |
| `purpose` | conversations.setPurpose | W | channels:write D | A1 | channel, purpose |
| `kick` | conversations.kick | W | channels:write D | A1 | channel, user |
| `rename` | conversations.rename | W | channels:write D | A1 | channel, name |
| `unarchive` | conversations.unarchive | W | channels:write D | A1 | channel |
| `open` | conversations.open | W | im:write D | A1 | users (parse+print channel.id) |
| `mark` | conversations.mark | W | channels:write D | A1 | channel, ts |
| `close` | conversations.close | W | im:write D | A1 | channel |

### `user` (existing file, +7)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `by-email` | users.lookupByEmail | R | users:read D | A2 | email |
| `presence` | users.getPresence | R | users:read D | A2 | user (opt) |
| `channels` | users.conversations | R | users:read D | A3 | user (opt), types |
| `set-profile` | users.profile.set | W | users.profile:write + | A1 | name+value OR profile (JSON) |
| `set-photo` | users.setPhoto | W | users.profile:write + | A4-img | file (path, multipart `image`) |
| `delete-photo` | users.deletePhoto | W | users.profile:write + | A1 | (none) |
| `set-presence` | users.setPresence | W | users:write + | A1 | presence (auto/away) |

### `canvas` (existing file, +3)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `delete` | canvases.delete | W | canvases:write D | A1 | id→canvas_id |
| `share` | canvases.access.set | W | canvases:write D | A1 | id→canvas_id, access-level→access_level, users→user_ids OR channels→channel_ids |
| `unshare` | canvases.access.delete | W | canvases:write D | A1 | id→canvas_id, users→user_ids OR channels→channel_ids |

### `list` (existing file, +2)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `delete-item` | slackLists.items.delete | W | lists:write D | A1 | id→list_id, row-id→id (Long: whole-list delete still UI-only) |
| `update` | slackLists.update | W | lists:write D | A1 | id, name, description (opt), todo-mode→todo_mode (opt) |

### `search` (existing file, +2)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `files` | search.files | R | search:read D | A2 | query (parse files.matches[]) |
| `all` | search.all | R | search:read D | A2 | query (parse messages.matches + files.matches) |

### `auth` (existing file, +2 — INTEGRATOR DOES THIS)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `test` | auth.test | R | none | A2 | (none) → print `team (team_id) — user (user_id) @ url` |
| `revoke` | auth.revoke | W | none | A1 | (none) → revokes token at Slack; honor dry-run |

### `file` (NEW file, 6)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `list` | files.list | R | files:read D | A3 | channel, user, types (page-based) |
| `info` | files.info | R | files:read D | A2 | file |
| `upload` | files.getUploadURLExternal + files.completeUploadExternal | W | files:write + | A4 | file (path), channel (opt), title (opt) |
| `delete` | files.delete | W | files:write + | A1 | file |
| `public` | files.sharedPublicURL | W | files:write + | A1 | file (⚠️ Long: makes file public-by-link) |
| `revoke-public` | files.revokePublicURL | W | files:write + | A1 | file |

### `pin` (NEW file, 3)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `add` | pins.add | W | pins:write + | A1 | channel, ts→timestamp |
| `remove` | pins.remove | W | pins:write + | A1 | channel, ts→timestamp |
| `list` | pins.list | R | pins:read + | A3 | channel (parse items[]) |

### `bookmark` (NEW file, 4)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `add` | bookmarks.add | W | bookmarks:write + | A1 | channel→channel_id, title, link, type (default `link`) |
| `edit` | bookmarks.edit | W | bookmarks:write + | A1 | channel→channel_id, id→bookmark_id, title (opt), link (opt) |
| `remove` | bookmarks.remove | W | bookmarks:write + | A1 | channel→channel_id, id→bookmark_id |
| `list` | bookmarks.list | R | bookmarks:read + | A3 | channel→channel_id (parse bookmarks[]) |

### `team` (NEW file, 2)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `info` | team.info | R | team:read + | A2 | team (opt) → `name (id) — domain.slack.com` |
| `profile` | team.profile.get | R | users.profile:read D | A2 | (none) → list custom profile field labels |

### `emoji` (NEW file, 1)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `list` | emoji.list | R | emoji:read + | A3 | (none) → map name→url, sorted; keep `alias:` with note |

### `dnd` (NEW file, 5)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `info` | dnd.info | R | dnd:read + | A2 | user (opt) → `dnd_enabled`, snooze fields |
| `team` | dnd.teamInfo | R | dnd:read + | A3 | users (comma) |
| `snooze` | dnd.setSnooze | W | dnd:write + | A1 | minutes→num_minutes |
| `end-snooze` | dnd.endSnooze | W | dnd:write + | A1 | (none) |
| `end` | dnd.endDnd | W | dnd:write + | A1 | (none) |

### `usergroup` (NEW file, 7)
| Verb | Method | RW | Scope | Arch | flag→param |
|---|---|---|---|---|---|
| `list` | usergroups.list | R | usergroups:read + | A3 | (parse usergroups[] → name/handle/id) |
| `create` | usergroups.create | W | usergroups:write + | A1 | name, handle (opt), description (opt) |
| `update` | usergroups.update | W | usergroups:write + | A1 | usergroup, name (opt), handle (opt) |
| `enable` | usergroups.enable | W | usergroups:write + | A1 | usergroup |
| `disable` | usergroups.disable | W | usergroups:write + | A1 | usergroup |
| `users` | usergroups.users.list | R | usergroups:read + | A3 | usergroup |
| `set-users` | usergroups.users.update | W | usergroups:write + | A1 | usergroup, users |

### Deferred / excluded (state in PR description; do NOT implement)
- `canvases.sections.lookup` — covered by `canvas read --with-sections`.
- `files.remote.*` — niche external-file references; reachable via `slk api`.
- `bots.info`, `team.preferences.list` — low value / likely admin.
- **OUT** (prior decisions): `admin.*` (Grid), `drafts.list/delete/update` + `search.modules.*` (xoxc/xoxd), `reminders.*`/`stars.*` (deprecated), `views/dialog/workflows/functions/calls/rtm/apps/assistant.*` (app framework), `conversations.*SharedInvite*` (Slack Connect), `chat.unfurl`/streaming, `users.identity` (needs identity.basic).

---

## Archetypes (copy and adapt per verb)

### A1 — Simple write
Model: `channel.go` → `newChannelArchiveCommand`. `Annotations:{slackMethod, write:"true"}`; build `params` from flags (apply flag→param renames); `if g.DryRun { print "[dry-run] <method> %v"; return }`; `client.Call`; `if g.Raw { print raw; return }`; else a fixed confirmation line. `MarkFlagRequired` per required flag.

### A2 — Single-object read
Model: `user.go` → `newUserInfoCommand`. `Annotations:{slackMethod}`; `client.Call`; `if g.Raw { print raw; return }`; unmarshal one object; `output.Emit(out, g.Format, []T{hit})` (use `searchHit` or a small typed struct with `Concise()`).

### A3 — Paginated / list read
Model: `channel.go` → `fetchChannelsWith` + `search.go` → `newSearchChannelsCommand`. Page via `client.CallAll(method, params, 10)` (cursor) or a manual loop; `files.list` uses page-based (`page`/`pages`) not cursor. Accumulate `[]searchHit`; `output.Emit`. No `--raw` on multi-page reads (match `// --raw is not offered here`). Provide an httptest-able `parseXxx` helper.

### A4 — Two-step external upload (`file upload`, bespoke)
1. `client.Call("files.getUploadURLExternal", {filename, length}, nil)` → parse `upload_url` + `file_id`.
2. `http.Post(upload_url, "application/octet-stream", bytes.NewReader(data))` — raw POST to the returned URL, NOT via `api.Client`.
3. `client.Call("files.completeUploadExternal", {files:"[{\"id\":\"<file_id>\",\"title\":\"<title>\"}]", channel_id?}, nil)`.
Honor `--dry-run` (skip all 3) and `--raw` (print complete response). Read file via `os.ReadFile`; `length=len(data)`.

### A4-img — `user set-photo`
multipart/form-data POST to `users.setPhoto` with field `image` (file bytes) via `api.Client` — needs a multipart body helper. If `api.Client.Call` cannot send multipart, add a thin `client.CallMultipart(method, fields, fileField, filename, data)` in `internal/api` (integrator reviews any api change). Honor dry-run/raw.

---

## Tasks

### Task 0 — Expand default scopes (INTEGRATOR) ✅ first
**Files:** `internal/commands/auth.go:201`, `internal/commands/auth_test.go`.
- [ ] Test `TestAuthLogin_DefaultScopesIncludeExpanded` asserts `--scopes` default contains all 14 new scopes.
- [ ] Run → FAIL.
- [ ] Append `,reactions:read,files:write,users.profile:write,pins:read,pins:write,bookmarks:read,bookmarks:write,team:read,emoji:read,users:write,dnd:read,dnd:write,usergroups:read,usergroups:write`.
- [ ] Run → PASS.

### Task A — `auth test` + `auth revoke` (INTEGRATOR)
**Files:** `auth.go`, `auth_test.go`. Add both verbs (A2/A1), register in `newAuthCommand`. httptest for `auth test`; dry-run for `auth revoke`.

### Tasks 1–13 — one per group (SUBAGENT each, parallel; new-file groups are lowest-risk)
Each task: implement the group's verbs per the inventory + archetypes; add `_test.go` with (a) flag-registration for every verb, (b) dry-run for every write verb, (c) httptest parser test for every read verb that parses. Register new-file groups' constructor name for the integrator to wire in `root.go`.

- [ ] Task 1 `msg` (+8) — modify `msg.go`/`msg_test.go`
- [ ] Task 2 `channel` (+11) — modify `channel.go`/`channel_test.go`
- [ ] Task 3 `user` (+7) — modify `user.go`/`user_test.go`
- [ ] Task 4 `canvas` (+3) — modify `canvas.go`/`canvas_test.go`
- [ ] Task 5 `list`+`search` (+2/+2) — modify `list.go`/`search.go` + tests
- [ ] Task 6 `file` (NEW, 6) — create `file.go`/`file_test.go`
- [ ] Task 7 `pin` (NEW, 3) — create `pin.go`/`pin_test.go`
- [ ] Task 8 `bookmark` (NEW, 4) — create `bookmark.go`/`bookmark_test.go`
- [ ] Task 9 `team` (NEW, 2) — create `team.go`/`team_test.go`
- [ ] Task 10 `emoji` (NEW, 1) — create `emoji.go`/`emoji_test.go`
- [ ] Task 11 `dnd` (NEW, 5) — create `dnd.go`/`dnd_test.go`
- [ ] Task 12 `usergroup` (NEW, 7) — create `usergroup.go`/`usergroup_test.go`

### Task 14 — Register groups + build + full test (INTEGRATOR)
**Files:** `internal/commands/root.go`.
- [ ] Add `newFileCommand`, `newPinCommand`, `newBookmarkCommand`, `newTeamCommand`, `newEmojiCommand`, `newDndCommand`, `newUsergroupCommand` to `root.AddCommand`.
- [ ] `go build -o slk ./cmd/slk` → OK. `go clean -testcache && go test ./...` → PASS.
- [ ] `./slk --help` shows 7 new groups.

### Task 15 — Regenerate SKILL.md + sync docs (INTEGRATOR)
**Files:** `skills/slk/SKILL.md`, `README.md`, `SPEC.md`.
- [ ] `go run ./cmd/slk generate-skill`; verify no-drift; `git diff --stat` shows new verbs.
- [ ] README `Known Slack-side limitations`: note `list delete-item` (whole-list delete still UI-only) and `file public` (public-by-link) only if they add a real caveat; do NOT enumerate every command.
- [ ] SPEC: default-scope count 22→36; record deferrals (`canvases.sections.lookup`, `files.remote.*`, `bots.info`, `users.setPresence` now INCLUDED; keep list current). Neutral wording, no identity terms.
- [ ] Final `go clean -testcache && go test ./...` → PASS.

---

## Self-Review
- **Coverage:** every method authorized by the 36 scopes (minus the explicit deferral list) maps to a verb in Tasks 1–13; scope = Task 0; auth verbs = Task A; registration = Task 14; skill+docs = Task 15.
- **Placeholders:** archetypes carry reference code; per-verb rows give method + flag→param + output expectation. Error handling = return `err` (archetype).
- **Type consistency:** reads reuse `searchHit{Name,ID,Extra}` or a small `Concise()` struct; writes use `params map[string]string` + `client.Call`; flag→param renames tabulated once per verb.
- **Guardrails:** scrubbed test IDs; writes honor `--raw`+`--dry-run`; `slackMethod`/`write` annotations; no new deps; no TUI in command paths; subagents avoid SPEC/CLAUDE/README + commit/skill helpers.
