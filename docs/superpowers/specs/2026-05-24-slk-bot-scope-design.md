# slk Bot-Token Scope Blueprint — Design

- **Date:** 2026-05-24
- **Status:** Analysis / decision document (no code change mandated by this doc)
- **Author context:** Counterpart to the finalized user-token scope set.

> **Revision (2026-05-24, auth-redesign coordination).** A parallel auth-redesign effort
> (separate agent + branch) consumes this doc as machine-readable data. Two of its decisions
> change earlier assumptions here, reflected throughout and consolidated in **§9**:
> 1. `auth login` **will** mint bot tokens (`--as bot` → `scope=<bot scopes>` in the authorize
>    URL). The bot scope set therefore has **two** consumers, not one (§3, §7).
> 2. Both scope sets will be **dynamically generated** by unioning per-command annotations (the
>    command tree is the SSOT), not hand-maintained. §9 provides the canonical per-verb source
>    data and the reviewed final sets used as the generator's cross-check oracle.
>
> Net scope change this round: `users:read.email` is added to **both** sets (closes the
> `user by-email` gap), so the reviewed final sets are **37 user** / **35 bot** (see §9-E).

## 1. Background & goal

slk is user-token-first (`xoxp-`). The global `--as user|bot` flag already exists
(`internal/commands/context.go`, default `user`) and `ResolveToken` already selects
`profile.BotToken` when `--as bot` is passed (`internal/auth/token.go`). The bot-token
*mechanism* is therefore already in place; what is undefined is **which curated verbs actually
function under a bot token, and which bot scopes that requires**.

The user-token scope set is finalized at **36 scopes**, consumed in two places:

1. The README app manifest's `oauth_config.scopes.user` list (quick app creation).
2. The `slk auth login --scopes` default (the OAuth `user_scope=` parameter).

**Goal:** produce the bot-token counterpart — a per-command capability + suitability matrix,
verified against Slack's official method reference, culminating in a recommended
`oauth_config.scopes.bot` block and a go/no-go positioning. This is an analysis artifact; it
recommends, but does not itself perform, any implementation.

## 2. Method & sources

Every capability claim was verified against the Slack method reference at
`https://docs.slack.dev/reference/methods/<method>` (accessed 2026-05-24). Methods fetched
directly: `search.messages`, `dnd.setSnooze`, `dnd.info`, `files.sharedPublicURL`,
`chat.meMessage`, `chat.delete`, `canvases.create`, `canvases.access.set`, `slackLists.create`,
`usergroups.create`, `usergroups.list`, `users.profile.set`, `users.profile.get`,
`users.setPresence`, `users.getPresence`, `users.setPhoto`, `conversations.create`,
`conversations.invite`, `conversations.history`, `conversations.replies`, `conversations.mark`,
`conversations.join`, `team.profile.get`, `files.completeUploadExternal`. Sibling methods in the
same family (e.g. `dnd.endSnooze` ↔ `dnd.setSnooze`, `search.files`/`search.all` ↔
`search.messages`, `files.revokePublicURL` ↔ `files.sharedPublicURL`) inherit the verified
verdict and are marked accordingly. Well-established standard scopes (`reactions:*`, `pins:*`,
`bookmarks:*`, `emoji:read`, `team:read`, `users:read`, `files:read`) are taken as known.

Two-axis rubric per verb:

- **Capability** (objective, from docs):
  - ✅ works with a bot token given the listed bot scope
  - ⚠️ works, but with narrower effect (own-resource-only) or membership constraints
  - ❌ user-token-only — no bot equivalent exists
- **Suitability** (product judgment):
  - **Fit** — automation-natural
  - **Limited** — works but the bot identity changes the semantics
  - **N/A-bot** — acts on a human's personal state the bot does not have, or is ❌

## 3. Key architectural distinction: where the bot set is consumed

The bot scope set has **two** consumers (updated — the auth-redesign branch adds bot-token
support to `auth login`):

| | User scope set | Bot scope set |
|---|---|---|
| Manifest block | `oauth_config.scopes.user` | `oauth_config.scopes.bot` |
| `auth login` authorize param | `user_scope=` (default) | `scope=` (explicit `--as bot`) |
| How the token is obtained | OAuth flow (`auth login`) **or** install + paste | OAuth flow (`auth login --as bot`) **or** install → copy `xoxb-` → `slk auth set-token` |

Previously this doc stated the bot set fed the manifest only. That is now false: with the
auth-redesign change, `auth login --as bot` requests the bot scope set via the OAuth `scope=`
parameter (user stays the default; bot is explicit), so the bot token can be minted through the
browser flow just like the user token. Install + paste (`slk auth set-token` already accepts an
optional `xoxb-`) remains the alternate path.

**Implication:** both the README `oauth_config.scopes.bot` manifest block **and** the
`auth login` bot default are generated from the same per-verb annotations (§9). They must stay in
sync — which is exactly why §2 of the auth effort makes them generated + CI-guarded rather than
hand-maintained.

## 4. Per-command capability & suitability matrix

Legend — Cap: ✅ / ⚠️ / ❌. Suit: Fit / Limited / N/A-bot. "Bot scope" is the scope a bot token
needs; "—" means no bot scope exists (user-only).

### auth
| Verb | Method | User scope | Bot scope | Cap | Suit | Note |
|---|---|---|---|---|---|---|
| `auth test` | auth.test | (any token) | (any token) | ✅ | Fit | Useful to verify the `xoxb-` token / identity |
| `auth revoke` | auth.revoke | (any token) | (any token) | ✅ | Fit | |
| `auth status` | auth.test | (any token) | (any token) | ✅ | Fit | |
| `auth set-token` / `switch` / `logout` | local config | — | — | ✅ | Fit | No API; already accepts a `xoxb-` token |
| `auth login` | OAuth `user_scope=` | — | ❌ | ❌ | N/A-bot | Mints a **user** token only; not a bot-token path |

### msg
| Verb | Method | User scope | Bot scope | Cap | Suit | Note |
|---|---|---|---|---|---|---|
| `msg read` | conversations.history | channels:history… | channels:history, groups:history, im:history, mpim:history | ⚠️ | Limited | Bot sees only channels it is a **member** of; DM history limited to the **bot's own** DMs |
| `msg send` | chat.postMessage | chat:write | chat:write | ✅ | Fit | |
| `msg update` | chat.update | chat:write | chat:write | ⚠️ | Limited | Bot may edit only messages **it** posted |
| `msg delete` | chat.delete | chat:write | chat:write | ⚠️ | Limited | Bot may delete only messages **it** posted (verified) |
| `msg react` / `unreact` | reactions.add / remove | reactions:write | reactions:write | ✅ | Fit | |
| `msg reactions` / `reacted` | reactions.get / list | reactions:read | reactions:read | ✅ | Fit | |
| `msg schedule` / `unschedule` / `scheduled` | chat.scheduleMessage / deleteScheduledMessage / scheduledMessages.list | chat:write | chat:write | ✅ | Fit | |
| `msg permalink` | chat.getPermalink | (read) | (read) | ✅ | Fit | |
| `msg ephemeral` | chat.postEphemeral | chat:write | chat:write | ✅ | Fit | Bot posting an ephemeral is natural |
| `msg me` | chat.meMessage | chat:write | chat:write | ✅ | Fit | Verified both token types |
| `msg draft` | drafts.create | (xoxp works) | — | ❌ | N/A-bot | `drafts.create` is not in the public method reference; drafts are a **per-user compose** artifact (no bot Drafts panel). Excluded |

### channel
| Verb | Method | User scope | Bot scope | Cap | Suit | Note |
|---|---|---|---|---|---|---|
| `channel list` | conversations.list | channels:read… | channels:read, groups:read, im:read, mpim:read | ✅ | Fit | |
| `channel info` / `members` | conversations.info / members | channels:read | channels:read (+ groups/im/mpim:read) | ✅ | Fit | |
| `channel create` | conversations.create | channels:write… | channels:manage, groups:write, im:write, mpim:write | ✅ | Fit | Bot uses `channels:manage` (verified) |
| `channel archive` / `unarchive` / `rename` / `topic` / `purpose` / `kick` / `leave` | conversations.archive / unarchive / rename / setTopic / setPurpose / kick / leave | channels:write… | channels:manage (+ groups:write, im:write, mpim:write) | ✅ | Fit | All covered by `channels:manage` |
| `channel invite` | conversations.invite | channels:write… | channels:manage | ✅ | Fit | Granular `channels:write.invites` also accepted; `channels:manage` subsumes it |
| `channel join` | conversations.join | channels:write | **channels:join** | ✅ | Fit | Bot scope **differs** from user (`channels:join` vs `channels:write`) — verified |
| `channel open` / `close` | conversations.open / close | im:write, mpim:write | im:write, mpim:write | ✅ | Fit | |
| `channel mark` | conversations.mark | channels:write… | channels:manage… | ✅ | Fit | Marks the **bot's** read cursor |

> Private-channel caveat: for any private channel (`groups:*`) read or management verb, the bot
> must first be a member of that channel.

### thread
| Verb | Method | User scope | Bot scope | Cap | Suit | Note |
|---|---|---|---|---|---|---|
| `thread read` | conversations.replies | channels:history… | channels:history, groups:history, im:history, mpim:history | ⚠️ | Limited | Same membership constraint as `msg read` |
| `thread reply` | chat.postMessage | chat:write | chat:write | ✅ | Fit | |

### user
| Verb | Method | User scope | Bot scope | Cap | Suit | Note |
|---|---|---|---|---|---|---|
| `user list` / `info` | users.list / info | users:read | users:read | ✅ | Fit | |
| `user channels` | users.conversations | users:read | users:read | ✅ | Fit | |
| `user by-email` | users.lookupByEmail | users:read.email | users:read.email | ✅ | Fit | `users:read.email` added to **both** sets this round (§9-C); resolved |
| `user presence` | users.getPresence | users:read | users:read | ✅ | Fit | Reading a human's presence is fine for a bot (verified) |
| `user profile` | users.profile.get | users.profile:read | users.profile:read | ✅ | Fit | Verified |
| `user set-profile` | users.profile.set | users.profile:write | — | ❌ | N/A-bot | Editing a personal profile is user-token-only (verified) |
| `user set-photo` / `delete-photo` | users.setPhoto / deletePhoto | users.profile:write | — | ❌ | N/A-bot | User-token-only (verified `setPhoto`) |
| `user set-presence` | users.setPresence | users:write | users:write | ✅ | N/A-bot | Technically works (verified both), but a bot/app has no human presence to broadcast — **pointless**; the only verb backing `users:write` |

### search
| Verb | Method | User scope | Bot scope | Cap | Suit | Note |
|---|---|---|---|---|---|---|
| `search messages` | search.messages | search:read | — | ❌ | N/A-bot | Search API is user-token-only (verified) |
| `search files` / `all` | search.files / search.all | search:read | — | ❌ | N/A-bot | Same Search API restriction |
| `search channels` | conversations.list | channels:read… | channels:read… | ✅ | Fit | **Not** the Search API — a local filter over `conversations.list` |
| `search users` | users.list | users:read | users:read | ✅ | Fit | Local filter over `users.list` |

### file
| Verb | Method | User scope | Bot scope | Cap | Suit | Note |
|---|---|---|---|---|---|---|
| `file list` / `info` | files.list / info | files:read | files:read | ✅ | Fit | |
| `file upload` | files.getUploadURLExternal → completeUploadExternal | files:write | files:write | ✅ | Fit | Verified `completeUploadExternal` |
| `file delete` | files.delete | files:write | files:write | ⚠️ | Limited | Bot may delete only its **own** files |
| `file public` / `revoke-public` | files.sharedPublicURL / revokePublicURL | files:write | — | ❌ | N/A-bot | Public-URL sharing is user-token-only (verified `sharedPublicURL`) |

### canvas
| Verb | Method | User scope | Bot scope | Cap | Suit | Note |
|---|---|---|---|---|---|---|
| `canvas create` | canvases.create | canvases:write | canvases:write | ✅ | Fit | Verified |
| `canvas read` | files.info | files:read | files:read | ✅ | Fit | A canvas is a file |
| `canvas update` / `delete` | canvases.edit / delete | canvases:write | canvases:write | ✅ | Fit | |
| `canvas share` / `unshare` | canvases.access.set / delete | canvases:write | canvases:write | ✅ | Fit | Verified `access.set` |
| `canvas list` | search.files | search:read | — | ❌ | N/A-bot | Canvas listing is built on the Search API → user-only |

### list (Slack Lists)
| Verb | Method | User scope | Bot scope | Cap | Suit | Note |
|---|---|---|---|---|---|---|
| `list create` | slackLists.create | lists:write | lists:write | ✅ | Fit | Verified |
| `list read` | slackLists.items.list | lists:read | lists:read | ✅ | Fit | |
| `list add-item` / `update-item` / `delete-item` | slackLists.items.create / update / delete | lists:write | lists:write | ✅ | Fit | |
| `list update` | slackLists.update | lists:write | lists:write | ✅ | Fit | |

### pin / bookmark / team / emoji / dnd
| Verb | Method | User scope | Bot scope | Cap | Suit | Note |
|---|---|---|---|---|---|---|
| `pin add` / `remove` | pins.add / remove | pins:write | pins:write | ✅ | Fit | |
| `pin list` | pins.list | pins:read | pins:read | ✅ | Fit | |
| `bookmark add` / `edit` / `remove` | bookmarks.add / edit / remove | bookmarks:write | bookmarks:write | ✅ | Fit | |
| `bookmark list` | bookmarks.list | bookmarks:read | bookmarks:read | ✅ | Fit | |
| `team info` | team.info | team:read | team:read | ✅ | Fit | |
| `team profile` | team.profile.get | users.profile:read | users.profile:read | ✅ | Fit | Verified |
| `emoji list` | emoji.list | emoji:read | emoji:read | ✅ | Fit | |
| `dnd info` | dnd.info | dnd:read | dnd:read | ✅ | Fit | Verified |
| `dnd team` | dnd.teamInfo | dnd:read | dnd:read | ✅ | Fit | |
| `dnd snooze` / `end-snooze` / `end` | dnd.setSnooze / endSnooze / endDnd | dnd:write | — | ❌ | N/A-bot | Setting DND is user-token-only (verified `setSnooze`); a bot has no DND state |

### usergroup (paid plans only)
| Verb | Method | User scope | Bot scope | Cap | Suit | Note |
|---|---|---|---|---|---|---|
| `usergroup list` / `users` | usergroups.list / users.list | usergroups:read | usergroups:read | ✅ | Fit | Requires a paid workspace |
| `usergroup create` / `update` / `enable` / `disable` / `set-users` | usergroups.create / update / enable / disable / users.update | usergroups:write | usergroups:write | ✅ | Fit | Verified `create`; requires a paid workspace |

## 5. Derived bot scope set (Approach A — capability upper bound)

Approach A (chosen): include **every bot-capable scope**; express suitability as annotation, not
omission — mirroring the user manifest's "list the full set, trim to taste" philosophy.

**Diff from the user set (post-email-fix baseline of 37 — see §9-E):**

- **Drop 3** (no bot equivalent): `search:read`, `users.profile:write`, `dnd:write`.
- **Replace 1**: `channels:write` → `channels:manage` (the bot channel-management scope —
  verified bot-only via the scope reference, §9 note).
- **Add 1**: `channels:join` (the bot scope for `conversations.join`).
- Remaining scopes (including `users:read.email`) carry over unchanged.

Net: 37 − 3 − 1 (channels:write) + 1 (channels:manage) + 1 (channels:join) = **35 bot scopes**.
The JSON below has been updated to include `users:read.email`. §9 is the authoritative,
machine-consumable source; this block is the human-readable manifest artifact.

```json
{
  "display_information": {
    "name": "slk"
  },
  "oauth_config": {
    "scopes": {
      "bot": [
        "channels:history",
        "channels:read",
        "channels:manage",
        "channels:join",
        "groups:history",
        "groups:read",
        "groups:write",
        "im:history",
        "im:read",
        "im:write",
        "mpim:history",
        "mpim:read",
        "mpim:write",
        "chat:write",
        "reactions:read",
        "reactions:write",
        "users:read",
        "users:read.email",
        "users:write",
        "users.profile:read",
        "files:read",
        "files:write",
        "canvases:read",
        "canvases:write",
        "lists:read",
        "lists:write",
        "pins:read",
        "pins:write",
        "bookmarks:read",
        "bookmarks:write",
        "team:read",
        "emoji:read",
        "dnd:read",
        "usergroups:read",
        "usergroups:write"
      ]
    }
  },
  "settings": {
    "org_deploy_enabled": false,
    "socket_mode_enabled": false,
    "token_rotation_enabled": false
  }
}
```

**Trim candidates (annotated, kept in Approach A):**

- `users:write` — backs only `user set-presence`, which is pointless for a bot. Drop unless you
  specifically automate presence. Removing it takes the set to 33.
- `usergroups:read` / `usergroups:write` — only function on paid workspaces; drop on free plans.
- `dnd:read`, `users.profile:read` — read-only convenience; keep or drop per need.

## 6. Capability boundary summary

- **❌ user-only / not usefully available on a bot (14 verbs):** `search messages`,
  `search files`, `search all`, `canvas list` (all Search API); `user set-profile`,
  `user set-photo`, `user delete-photo` (profile write); `dnd snooze`, `dnd end-snooze`,
  `dnd end` (DND write); `file public`, `file revoke-public` (public URL); `msg draft`
  (undocumented + per-user concept); `auth login` (mints a user token). Plus `user set-presence`
  works but is pointless.
- **⚠️ work with reduced effect (5 verbs):** `msg read`, `thread read` (membership-bound;
  DM = bot's own only); `msg update`, `msg delete`, `file delete` (own resources only).
- **✅ full automation fit (~75 verbs):** all messaging-out, channel management, reactions,
  pins, bookmarks, files upload/list/info, canvas create/read/update/delete/share, Slack Lists,
  usergroups (paid), and all read verbs for team / emoji / users / DND status.

**Resolved this round:** `user by-email` needs `users:read.email`, which was absent from the
legacy hand-maintained user-36 set. The auth-redesign coordination adds it to **both** the user
and bot sets (§9-C), so `user by-email` is `botCapable=true` and works in both modes. Final sets
are 37 user / 35 bot (§9-E).

## 7. Go / no-go recommendation

**Recommendation: GO, with positioning "bot = the automation subset of user mode, not a parallel
replacement."**

Rationale: a bot token is an excellent fit for the write-heavy automation core (post / schedule /
react, channel + canvas + list + file management, status reads). But three pillars of slk's
user-first surface degrade or vanish on a bot: **Search** (entirely gone — the single biggest
loss), **personal state** (profile, photo, presence, DND writes — gone or meaningless), and
**broad read** (history is restricted to channels the bot has joined; DMs to the bot's own).

The blueprint's value is to make that boundary explicit so a bot operator is not surprised. The
recommended deliverables, if this is greenlit for implementation (a **separate** plan):

1. Add the `scopes.bot` block (§5) to the README manifest, with a short "bot mode is a subset"
   note and the acquisition path (§3).
2. Document the ❌/⚠️ boundary (§6) where the README already documents Slack-side traps.
3. Optional guardrail: have `--as bot` short-circuit the ❌ verbs with a clear "user-token-only"
   message (exit code 3) instead of surfacing a raw Slack `not_allowed_token_type` error.
4. **In progress (auth-redesign branch).** `auth login --as bot` requests `scope=<bot scopes>`
   and captures the returned `xoxb-`, so the OAuth path mints a bot token too — no longer
   install-and-paste only. This greenlights what was an optional item; the bot scope set now
   feeds this flow's default (§3, §9-E).

## 8. Out of scope

- Implementation of the above (manifest edit, guardrails, docs, any `auth login` change) — a
  separate spec/plan if greenlit.
- Enterprise Grid / admin.* scopes — slk is single-workspace, public-OAuth only (per SPEC).
- `xoxc`/`xoxd` Slack-client token types (drafts lifecycle, etc.) — explicitly excluded by slk's
  design.

## 9. Auth-redesign coordination — canonical data

This section is the **machine-consumable source of truth** for the parallel auth-redesign branch.
Everything here is doc-only; command code, the generator, README, and auth files are owned by that
branch.

### 9-A. Canonical per-verb table

Conventions:

- `userScopes` / `botScopes`: comma-separated scope names. **Empty string** = the method needs
  no scope (works with any valid token, e.g. `auth test`, `chat.getPermalink`) **or** the verb is
  local / has no API call. For `botCapable=false`, `botScopes` is empty (no bot equivalent).
- `botCapable`: `true` if the verb functions at all under a bot token (even if reduced — see §6
  ⚠️ verbs), `false` if user-token-only.
- **Channel-scope rule** (verified via the scope reference, not the method pages): `channels:write`
  is **user-token-only**; `channels:manage` and `channels:join` are **bot-token-only**. So every
  public-channel write/management method maps user `channels:write` → bot `channels:manage`, and
  `conversations.join` maps user `channels:write` → bot `channels:join`. `groups:write` /
  `im:write` / `mpim:write` are identical across token types. Method-page "bot token scopes" lists
  that include `channels:write` are a column-merge artifact; the scope reference is authoritative.
- `canvases:read` is annotated defensively on `canvas read` (which calls `files.info`); see §9-E.

```json
[
  {"verb": "auth test", "slackMethod": "auth.test", "userScopes": "", "botScopes": "", "botCapable": true},
  {"verb": "auth revoke", "slackMethod": "auth.revoke", "userScopes": "", "botScopes": "", "botCapable": true},
  {"verb": "auth status", "slackMethod": "auth.test", "userScopes": "", "botScopes": "", "botCapable": true},
  {"verb": "auth set-token", "slackMethod": "", "userScopes": "", "botScopes": "", "botCapable": true},
  {"verb": "auth switch", "slackMethod": "", "userScopes": "", "botScopes": "", "botCapable": true},
  {"verb": "auth logout", "slackMethod": "", "userScopes": "", "botScopes": "", "botCapable": true},
  {"verb": "auth login", "slackMethod": "oauth.v2.access", "userScopes": "", "botScopes": "", "botCapable": true},

  {"verb": "msg read", "slackMethod": "conversations.history", "userScopes": "channels:history,groups:history,im:history,mpim:history", "botScopes": "channels:history,groups:history,im:history,mpim:history", "botCapable": true},
  {"verb": "msg send", "slackMethod": "chat.postMessage", "userScopes": "chat:write", "botScopes": "chat:write", "botCapable": true},
  {"verb": "msg update", "slackMethod": "chat.update", "userScopes": "chat:write", "botScopes": "chat:write", "botCapable": true},
  {"verb": "msg delete", "slackMethod": "chat.delete", "userScopes": "chat:write", "botScopes": "chat:write", "botCapable": true},
  {"verb": "msg react", "slackMethod": "reactions.add", "userScopes": "reactions:write", "botScopes": "reactions:write", "botCapable": true},
  {"verb": "msg unreact", "slackMethod": "reactions.remove", "userScopes": "reactions:write", "botScopes": "reactions:write", "botCapable": true},
  {"verb": "msg schedule", "slackMethod": "chat.scheduleMessage", "userScopes": "chat:write", "botScopes": "chat:write", "botCapable": true},
  {"verb": "msg unschedule", "slackMethod": "chat.deleteScheduledMessage", "userScopes": "chat:write", "botScopes": "chat:write", "botCapable": true},
  {"verb": "msg scheduled", "slackMethod": "chat.scheduledMessages.list", "userScopes": "", "botScopes": "", "botCapable": true},
  {"verb": "msg permalink", "slackMethod": "chat.getPermalink", "userScopes": "", "botScopes": "", "botCapable": true},
  {"verb": "msg ephemeral", "slackMethod": "chat.postEphemeral", "userScopes": "chat:write", "botScopes": "chat:write", "botCapable": true},
  {"verb": "msg me", "slackMethod": "chat.meMessage", "userScopes": "chat:write", "botScopes": "chat:write", "botCapable": true},
  {"verb": "msg reactions", "slackMethod": "reactions.get", "userScopes": "reactions:read", "botScopes": "reactions:read", "botCapable": true},
  {"verb": "msg reacted", "slackMethod": "reactions.list", "userScopes": "reactions:read", "botScopes": "reactions:read", "botCapable": true},
  {"verb": "msg draft", "slackMethod": "drafts.create", "userScopes": "", "botScopes": "", "botCapable": false},

  {"verb": "channel list", "slackMethod": "conversations.list", "userScopes": "channels:read,groups:read,im:read,mpim:read", "botScopes": "channels:read,groups:read,im:read,mpim:read", "botCapable": true},
  {"verb": "channel create", "slackMethod": "conversations.create", "userScopes": "channels:write,groups:write,im:write,mpim:write", "botScopes": "channels:manage,groups:write,im:write,mpim:write", "botCapable": true},
  {"verb": "channel archive", "slackMethod": "conversations.archive", "userScopes": "channels:write,groups:write,im:write,mpim:write", "botScopes": "channels:manage,groups:write,im:write,mpim:write", "botCapable": true},
  {"verb": "channel invite", "slackMethod": "conversations.invite", "userScopes": "channels:write,groups:write,im:write,mpim:write", "botScopes": "channels:manage,groups:write,im:write,mpim:write", "botCapable": true},
  {"verb": "channel topic", "slackMethod": "conversations.setTopic", "userScopes": "channels:write,groups:write,im:write,mpim:write", "botScopes": "channels:manage,groups:write,im:write,mpim:write", "botCapable": true},
  {"verb": "channel info", "slackMethod": "conversations.info", "userScopes": "channels:read,groups:read,im:read,mpim:read", "botScopes": "channels:read,groups:read,im:read,mpim:read", "botCapable": true},
  {"verb": "channel members", "slackMethod": "conversations.members", "userScopes": "channels:read,groups:read,im:read,mpim:read", "botScopes": "channels:read,groups:read,im:read,mpim:read", "botCapable": true},
  {"verb": "channel join", "slackMethod": "conversations.join", "userScopes": "channels:write", "botScopes": "channels:join", "botCapable": true},
  {"verb": "channel leave", "slackMethod": "conversations.leave", "userScopes": "channels:write,groups:write,im:write,mpim:write", "botScopes": "channels:manage,groups:write,im:write,mpim:write", "botCapable": true},
  {"verb": "channel purpose", "slackMethod": "conversations.setPurpose", "userScopes": "channels:write,groups:write,im:write,mpim:write", "botScopes": "channels:manage,groups:write,im:write,mpim:write", "botCapable": true},
  {"verb": "channel kick", "slackMethod": "conversations.kick", "userScopes": "channels:write,groups:write,im:write,mpim:write", "botScopes": "channels:manage,groups:write,im:write,mpim:write", "botCapable": true},
  {"verb": "channel rename", "slackMethod": "conversations.rename", "userScopes": "channels:write,groups:write,im:write,mpim:write", "botScopes": "channels:manage,groups:write,im:write,mpim:write", "botCapable": true},
  {"verb": "channel unarchive", "slackMethod": "conversations.unarchive", "userScopes": "channels:write,groups:write,im:write,mpim:write", "botScopes": "channels:manage,groups:write,im:write,mpim:write", "botCapable": true},
  {"verb": "channel open", "slackMethod": "conversations.open", "userScopes": "im:write,mpim:write", "botScopes": "im:write,mpim:write", "botCapable": true},
  {"verb": "channel mark", "slackMethod": "conversations.mark", "userScopes": "channels:write,groups:write,im:write,mpim:write", "botScopes": "channels:manage,groups:write,im:write,mpim:write", "botCapable": true},
  {"verb": "channel close", "slackMethod": "conversations.close", "userScopes": "im:write,mpim:write", "botScopes": "im:write,mpim:write", "botCapable": true},

  {"verb": "thread read", "slackMethod": "conversations.replies", "userScopes": "channels:history,groups:history,im:history,mpim:history", "botScopes": "channels:history,groups:history,im:history,mpim:history", "botCapable": true},
  {"verb": "thread reply", "slackMethod": "chat.postMessage", "userScopes": "chat:write", "botScopes": "chat:write", "botCapable": true},

  {"verb": "user list", "slackMethod": "users.list", "userScopes": "users:read", "botScopes": "users:read", "botCapable": true},
  {"verb": "user info", "slackMethod": "users.info", "userScopes": "users:read", "botScopes": "users:read", "botCapable": true},
  {"verb": "user by-email", "slackMethod": "users.lookupByEmail", "userScopes": "users:read.email", "botScopes": "users:read.email", "botCapable": true},
  {"verb": "user presence", "slackMethod": "users.getPresence", "userScopes": "users:read", "botScopes": "users:read", "botCapable": true},
  {"verb": "user channels", "slackMethod": "users.conversations", "userScopes": "channels:read,groups:read,im:read,mpim:read", "botScopes": "channels:read,groups:read,im:read,mpim:read", "botCapable": true},
  {"verb": "user set-profile", "slackMethod": "users.profile.set", "userScopes": "users.profile:write", "botScopes": "", "botCapable": false},
  {"verb": "user set-photo", "slackMethod": "users.setPhoto", "userScopes": "users.profile:write", "botScopes": "", "botCapable": false},
  {"verb": "user delete-photo", "slackMethod": "users.deletePhoto", "userScopes": "users.profile:write", "botScopes": "", "botCapable": false},
  {"verb": "user set-presence", "slackMethod": "users.setPresence", "userScopes": "users:write", "botScopes": "users:write", "botCapable": true},
  {"verb": "user profile", "slackMethod": "users.profile.get", "userScopes": "users.profile:read", "botScopes": "users.profile:read", "botCapable": true},

  {"verb": "search messages", "slackMethod": "search.messages", "userScopes": "search:read", "botScopes": "", "botCapable": false},
  {"verb": "search channels", "slackMethod": "conversations.list", "userScopes": "channels:read,groups:read,im:read,mpim:read", "botScopes": "channels:read,groups:read,im:read,mpim:read", "botCapable": true},
  {"verb": "search users", "slackMethod": "users.list", "userScopes": "users:read", "botScopes": "users:read", "botCapable": true},
  {"verb": "search files", "slackMethod": "search.files", "userScopes": "search:read", "botScopes": "", "botCapable": false},
  {"verb": "search all", "slackMethod": "search.all", "userScopes": "search:read", "botScopes": "", "botCapable": false},

  {"verb": "file list", "slackMethod": "files.list", "userScopes": "files:read", "botScopes": "files:read", "botCapable": true},
  {"verb": "file info", "slackMethod": "files.info", "userScopes": "files:read", "botScopes": "files:read", "botCapable": true},
  {"verb": "file upload", "slackMethod": "files.getUploadURLExternal,files.completeUploadExternal", "userScopes": "files:write", "botScopes": "files:write", "botCapable": true},
  {"verb": "file delete", "slackMethod": "files.delete", "userScopes": "files:write", "botScopes": "files:write", "botCapable": true},
  {"verb": "file public", "slackMethod": "files.sharedPublicURL", "userScopes": "files:write", "botScopes": "", "botCapable": false},
  {"verb": "file revoke-public", "slackMethod": "files.revokePublicURL", "userScopes": "files:write", "botScopes": "", "botCapable": false},

  {"verb": "canvas create", "slackMethod": "canvases.create", "userScopes": "canvases:write", "botScopes": "canvases:write", "botCapable": true},
  {"verb": "canvas read", "slackMethod": "files.info", "userScopes": "files:read,canvases:read", "botScopes": "files:read,canvases:read", "botCapable": true},
  {"verb": "canvas update", "slackMethod": "canvases.edit", "userScopes": "canvases:write", "botScopes": "canvases:write", "botCapable": true},
  {"verb": "canvas delete", "slackMethod": "canvases.delete", "userScopes": "canvases:write", "botScopes": "canvases:write", "botCapable": true},
  {"verb": "canvas share", "slackMethod": "canvases.access.set", "userScopes": "canvases:write", "botScopes": "canvases:write", "botCapable": true},
  {"verb": "canvas unshare", "slackMethod": "canvases.access.delete", "userScopes": "canvases:write", "botScopes": "canvases:write", "botCapable": true},
  {"verb": "canvas list", "slackMethod": "search.files", "userScopes": "search:read", "botScopes": "", "botCapable": false},

  {"verb": "list create", "slackMethod": "slackLists.create", "userScopes": "lists:write", "botScopes": "lists:write", "botCapable": true},
  {"verb": "list read", "slackMethod": "slackLists.items.list", "userScopes": "lists:read", "botScopes": "lists:read", "botCapable": true},
  {"verb": "list add-item", "slackMethod": "slackLists.items.create", "userScopes": "lists:write", "botScopes": "lists:write", "botCapable": true},
  {"verb": "list update-item", "slackMethod": "slackLists.items.update", "userScopes": "lists:write", "botScopes": "lists:write", "botCapable": true},
  {"verb": "list delete-item", "slackMethod": "slackLists.items.delete", "userScopes": "lists:write", "botScopes": "lists:write", "botCapable": true},
  {"verb": "list update", "slackMethod": "slackLists.update", "userScopes": "lists:write", "botScopes": "lists:write", "botCapable": true},

  {"verb": "pin add", "slackMethod": "pins.add", "userScopes": "pins:write", "botScopes": "pins:write", "botCapable": true},
  {"verb": "pin remove", "slackMethod": "pins.remove", "userScopes": "pins:write", "botScopes": "pins:write", "botCapable": true},
  {"verb": "pin list", "slackMethod": "pins.list", "userScopes": "pins:read", "botScopes": "pins:read", "botCapable": true},

  {"verb": "bookmark add", "slackMethod": "bookmarks.add", "userScopes": "bookmarks:write", "botScopes": "bookmarks:write", "botCapable": true},
  {"verb": "bookmark edit", "slackMethod": "bookmarks.edit", "userScopes": "bookmarks:write", "botScopes": "bookmarks:write", "botCapable": true},
  {"verb": "bookmark remove", "slackMethod": "bookmarks.remove", "userScopes": "bookmarks:write", "botScopes": "bookmarks:write", "botCapable": true},
  {"verb": "bookmark list", "slackMethod": "bookmarks.list", "userScopes": "bookmarks:read", "botScopes": "bookmarks:read", "botCapable": true},

  {"verb": "team info", "slackMethod": "team.info", "userScopes": "team:read", "botScopes": "team:read", "botCapable": true},
  {"verb": "team profile", "slackMethod": "team.profile.get", "userScopes": "users.profile:read", "botScopes": "users.profile:read", "botCapable": true},

  {"verb": "emoji list", "slackMethod": "emoji.list", "userScopes": "emoji:read", "botScopes": "emoji:read", "botCapable": true},

  {"verb": "dnd info", "slackMethod": "dnd.info", "userScopes": "dnd:read", "botScopes": "dnd:read", "botCapable": true},
  {"verb": "dnd team", "slackMethod": "dnd.teamInfo", "userScopes": "dnd:read", "botScopes": "dnd:read", "botCapable": true},
  {"verb": "dnd snooze", "slackMethod": "dnd.setSnooze", "userScopes": "dnd:write", "botScopes": "", "botCapable": false},
  {"verb": "dnd end-snooze", "slackMethod": "dnd.endSnooze", "userScopes": "dnd:write", "botScopes": "", "botCapable": false},
  {"verb": "dnd end", "slackMethod": "dnd.endDnd", "userScopes": "dnd:write", "botScopes": "", "botCapable": false},

  {"verb": "usergroup list", "slackMethod": "usergroups.list", "userScopes": "usergroups:read", "botScopes": "usergroups:read", "botCapable": true},
  {"verb": "usergroup create", "slackMethod": "usergroups.create", "userScopes": "usergroups:write", "botScopes": "usergroups:write", "botCapable": true},
  {"verb": "usergroup update", "slackMethod": "usergroups.update", "userScopes": "usergroups:write", "botScopes": "usergroups:write", "botCapable": true},
  {"verb": "usergroup enable", "slackMethod": "usergroups.enable", "userScopes": "usergroups:write", "botScopes": "usergroups:write", "botCapable": true},
  {"verb": "usergroup disable", "slackMethod": "usergroups.disable", "userScopes": "usergroups:write", "botScopes": "usergroups:write", "botCapable": true},
  {"verb": "usergroup users", "slackMethod": "usergroups.users.list", "userScopes": "usergroups:read", "botScopes": "usergroups:read", "botCapable": true},
  {"verb": "usergroup set-users", "slackMethod": "usergroups.users.update", "userScopes": "usergroups:write", "botScopes": "usergroups:write", "botCapable": true}
]
```

Notes on specific rows:

- `chat.getPermalink`, `chat.scheduledMessages.list` — verified "no scopes required"; both
  `botCapable=true` with empty scopes.
- `auth login` — the OAuth consumer, not a scoped verb; it *requests* the union. `botCapable=true`
  reflects that it now mints bot tokens (`--as bot`). It contributes no scope to the union.
- `msg draft` — `drafts.create` is not in the public method reference and drafts are a per-user
  compose artifact. `botCapable=false`; left out of both sets. The companion lifecycle endpoints
  need `xoxc`/`xoxd`, out of slk's design.
- Management-verb bot scopes for `setTopic`/`setPurpose`/`kick`/`rename`/`unarchive` are applied
  via the verified channel-scope rule (directly confirmed on `create`, `archive`, `leave`,
  `mark`); all public/private channel write/management methods share the same scope family.
- `channel open`/`close` are **not** channel-management verbs in slk: `open` requires `--users`
  and opens a DM/MPIM via `conversations.open`'s `users` param; `close` closes a DM/MPIM. Both
  therefore carry only `im:write,mpim:write` (identical for user and bot — no `channels:*`
  substitution). The `conversations.open`/`close` scope reference additionally lists the channel
  family (the method can resume a conversation by id), but slk never opens a public/private
  channel with it, so the family is omitted as an over-grant.

### 9-B. Every verb where `userScopes` ≠ `botScopes`

**Correction to the handoff assumption: it is NOT just `channel create` + `channel join`.** Two
classes:

**B1 — Substitution (botCapable=true, non-empty but different bot scope) — 11 verbs.** The single
uniform rule `channels:write` (user) → `channels:manage` (bot) applies to every public-channel
write/management verb; `channel join` additionally maps to `channels:join`:

| Verb | userScopes (channel part) | botScopes (channel part) |
|---|---|---|
| `channel create`, `channel archive`, `channel unarchive`, `channel rename`, `channel topic`, `channel purpose`, `channel kick`, `channel invite`, `channel leave`, `channel mark` | `channels:write` | `channels:manage` |
| `channel join` | `channels:write` | `channels:join` |

(`groups:write` / `im:write` / `mpim:write` are unchanged; only the public-channel scope differs.
`channel open`/`close` are **excluded** — they operate only on DMs/MPIMs and carry
`im:write,mpim:write` identically for user and bot, so they are not a mismatch.)

**B2 — User-only (botCapable=false, botScopes empty) — 12 verbs.** Here `userScopes` is non-empty
but `botScopes` is empty:

| Verb | userScopes | reason |
|---|---|---|
| `user set-profile`, `user set-photo`, `user delete-photo` | `users.profile:write` | profile write is user-token-only |
| `search messages`, `search files`, `search all`, `canvas list` | `search:read` | Search API is user-token-only |
| `file public`, `file revoke-public` | `files:write` | public-URL sharing is user-token-only |
| `dnd snooze`, `dnd end-snooze`, `dnd end` | `dnd:write` | DND write is user-token-only |

(`msg draft` also differs in capability — `botCapable=false` — but both scope fields are empty, so
it is not a scope mismatch.)

### 9-C. `users:read.email` fix (confirmed)

`user by-email` (`users.lookupByEmail`) lists `users:read.email` in **both** `userScopes` and
`botScopes`, with `botCapable=true`. The gap noted in §6 is closed this round; the scope is in
both final sets below.

### 9-D. Recommended set = Approach A (capability upper bound)

Consume the sets verbatim. Trims (`users:write` → only powers the pointless `user set-presence`;
`usergroups:*` → paid plans only; `dnd:read` / `users.profile:read` / `canvases:read` →
read-only convenience) are flagged as **notes, never omissions**. The union of the §9-A
annotations *is* the set; nothing is hand-removed.

### 9-E. Final sets (cross-check oracle)

These are the unions of §9-A (`userScopes` over all verbs → user set; `botScopes` over verbs with
`botCapable=true` → bot set). Assert the generator output against these exact lists.

**User set — 37 scopes** (= legacy hand-maintained 36 + `users:read.email`):

```
bookmarks:read, bookmarks:write, canvases:read, canvases:write, channels:history, channels:read,
channels:write, chat:write, dnd:read, dnd:write, emoji:read, files:read, files:write,
groups:history, groups:read, groups:write, im:history, im:read, im:write, lists:read, lists:write,
mpim:history, mpim:read, mpim:write, pins:read, pins:write, reactions:read, reactions:write,
search:read, team:read, usergroups:read, usergroups:write, users.profile:read,
users.profile:write, users:read, users:read.email, users:write
```

**Bot set — 35 scopes** (= user 37 − `search:read`, `users.profile:write`, `dnd:write`,
`channels:write` + `channels:manage`, `channels:join`):

```
bookmarks:read, bookmarks:write, canvases:read, canvases:write, channels:history, channels:join,
channels:manage, channels:read, chat:write, dnd:read, emoji:read, files:read, files:write,
groups:history, groups:read, groups:write, im:history, im:read, im:write, lists:read, lists:write,
mpim:history, mpim:read, mpim:write, pins:read, pins:write, reactions:read, reactions:write,
team:read, usergroups:read, usergroups:write, users.profile:read, users:read, users:read.email,
users:write
```

**One scope to verify before locking the oracle — `canvases:read`.** No curated verb calls a
method *documented* to require it: `canvas read` calls `files.info` (`files:read`) then downloads
`url_private`; `canvas list` calls `search.files` (`search:read`). It survives in both sets only
because §9-A annotates `canvas read` with `files:read,canvases:read` **defensively** (the legacy
36 granted it, and removing it risks breaking canvas reads if Slack gates canvas-type file access
behind `canvases:read`). **Decision for the auth branch:** verify whether `files.info` on a
canvas file needs `canvases:read`; if not, drop it from `canvas read`'s annotation, taking the
sets to **36 user / 34 bot**. Until verified, keep it (over-granting one read scope is safer than
breaking `canvas read`). This is the only count-affecting open item.
