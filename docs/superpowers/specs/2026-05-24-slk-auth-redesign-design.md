# slk Auth Redesign — One Token Per Profile, Bot Flow, Scope Generation — Design

- **Date:** 2026-05-24
- **Status:** Design / approved decisions (pre-implementation)
- **Branch:** `feat/auth-bot-flow` (from `main` @ `e8af577`)
- **Companion:** `docs/superpowers/specs/2026-05-24-slk-bot-scope-design.md` (the bot-scope
  blueprint; its §9 is the canonical per-verb scope data this design consumes).

## 1. Background & goal

slk has been user-token-first (`xoxp-`). Bot-token (`xoxb-`) support exists only partially:
the `--as user|bot` flag and a `BotToken` profile field are present, but `auth login` mints
user tokens only, profiles co-mingle both token types, and no command declares which scopes it
needs. The session goal is to **complete the bot-token flow**, starting with auth.

This design covers the auth layer plus the scope-generation infrastructure that both the user
and bot flows depend on. It does **not** re-derive the bot capability matrix — that work is the
companion blueprint, whose §9 canonical table is consumed here verbatim.

### Problem with the current model

A profile holds both `UserToken` and `BotToken` (`internal/auth/store.go:17`). Every command
must then answer "which token?" via `--as`, and `auth login`/`set-token` prompt for both. This
couples two independent credentials into one profile and forces an identity choice on every
call.

## 2. Locked decisions

These were settled in the design discussion and are inputs, not open questions:

1. **One token per profile.** `Profile` holds a single `Token`. Scope (user/bot) is **derived
   from the token prefix at runtime, never stored** — `xoxp-` → user, `xoxb-` → bot. (Tokens are
   plaintext in memory after `Load`; encryption is at-rest only, so the prefix is readable at
   decision time. A token that cannot be decrypted is unusable and reports as such.)
2. **`--as` is repurposed from selector to identity intent**, with context-dependent meaning:
   - On **runtime commands**: an *assertion* — default empty (no check); `--as bot`/`--as user`
     requires the active token's derived scope to match, else a clean auth error.
   - On **`auth login`**: a *mint selector* — default **user**; `--as bot` is the explicit,
     deliberate opt-in to mint a bot token. Misuse resistance lives at credential-creation time.
3. **`set-token` takes a single `--token`** (and `--token -` for stdin), replacing `--user` /
   `--bot`. It validates the prefix is a supported type (`xoxp-`/`xoxb-`); `xoxc-`/`xoxd-` stay
   rejected (an existing non-goal).
4. **`auth status`** annotates each profile with a derived `[user]`/`[bot]` label and gains
   `--format json`.
5. **`auth login` mints bot tokens** and computes its default scope list at runtime by unioning
   per-command annotations (no hardcoded literal).
6. **Clean break, no migration code.** Legacy dual-token configs are not read; pre-release, the
   product has effectively no users to migrate. A stray legacy profile resolves to an empty
   token and reports "no token", which is acceptable.
7. **Scopes are dynamically generated from per-command annotations** (the command tree is the
   SSOT), feeding `auth login` defaults (runtime) and the README manifest (generated + CI-guarded).
   Locked cross-check oracle: **37 user / 35 bot** scopes (see §7; `canvases:read` kept — §7.4).
8. **`--as bot` / a bot token short-circuits user-only verbs** (`botCapable=false`) with a clean
   message and exit 3, instead of surfacing Slack's `not_allowed_token_type`.
9. **`users:read.email`** is added (closes the pre-existing `user by-email` gap; companion §9-C).

## 3. Data model

`internal/auth/store.go`:

```go
// Profile holds the credential for one Slack identity. The token is stored
// encrypted at rest (enc:v1:) and decrypted into memory by Load. Scope
// (user/bot) is derived from the prefix, never stored.
type Profile struct {
    Token string `toml:"token,omitempty"`
}
```

`UserToken` / `BotToken` are removed. `encryptProfile` and `decryptInPlace` iterate over the
single `&p.Token` field. `Config` is otherwise unchanged (`active`, `key_backend`, `profiles`).

The `Load`-never-writes invariant is preserved; there is no on-read migration.

## 4. Scope derivation — one helper, three consumers

A single primitive in `internal/auth` is the only place that maps a token prefix to a scope:

```go
// TokenScope returns "user" for xoxp- tokens, "bot" for xoxb- tokens, or ""
// when the prefix is unrecognized (slk supports only these two). Callers handle
// a still-encrypted token (IsEncrypted) separately — its prefix is unreadable.
func TokenScope(token string) string
```

Consumers:

1. **`auth status`** — renders the `[user]`/`[bot]`/`[unknown]`/`[encrypted]` label.
2. **`ResolveToken` assertion** — compares against `--as` when set.
3. **`set-token` validation** — rejects an unsupported prefix at write time.

This keeps the prefix→scope mapping as a single source of truth.

## 5. Token resolution & the `--as` assertion

`ResolveToken` keeps its precedence (env `SLK_TOKEN` → `--profile`/`SLK_PROFILE` → `cfg.Active`)
but its third parameter changes from a *selector* to an *assertion*:

```go
func ResolveToken(cfg *Config, profileName, assertScope, envToken string) (string, error)
```

- Resolve the single profile token (or env token).
- If `assertScope != ""` **and** the resolved token's `TokenScope` is known **and** differs →
  return an `*AuthError` (exit 3): `profile %q holds a <scope> token but --as <assertScope> was requested`.
- An unknown prefix (`TokenScope == ""`) does not fail the assertion — only a *known mismatch*
  does. A still-encrypted token errors as today (cannot decrypt).
- The env token is validated against the assertion the same way (consistency); an unknown-prefix
  env token passes.

### Command-level guardrail (botCapable)

Separate from the prefix assertion: when the **effective** token scope is `bot` and the command
is annotated `botCapable=false`, refuse before hitting the network with a clean message
("`<verb>` is user-token-only; the active profile holds a bot token") and exit 3. This keys off
the *actual* token scope, so it fires even without `--as bot` (e.g. a bot profile is active and
the user runs `search messages`). Wiring (shared pre-execution check vs. `buildClient` variant
that receives the command's annotations) is a plan-level detail; the contract is: user-only verbs
fail fast and legibly under a bot token.

## 6. Command surface changes

### 6.1 `set-token`
- Flags: `--token` (`xoxp-`/`xoxb-`, or `-` for stdin), `--profile`, `--non-interactive`. Drop
  `--user` / `--bot`.
- Interactive prompt: "Paste token (xoxp- or xoxb-, hidden): ".
- Validate prefix via `TokenScope`; empty or unsupported prefix → error. A profile must resolve
  to a non-empty supported token (unchanged "refuse empty profile" behavior).
- Shared `promptCtx` resolution is retained.

### 6.2 `auth status`
- Takes `*GlobalFlags` (currently it does not) so it can read `--format`.
- The per-profile `(user=… bot=…)` column becomes a derived `[scope]` label.
- The live-check path returns **structured data**, not a pre-formatted string; text and JSON
  renderers consume it. (`liveIdentity`/`identitySuffix` are refactored to yield a struct.)
- `--format json` shape:

```json
{
  "encryption": { "backend": "file", "key_location": "~/.config/slk/.encryption_key" },
  "active": "work",
  "profiles": [
    { "name": "work", "active": true, "scope": "bot", "checked": true,
      "identity": { "team": "Acme", "team_id": "T0123456789",
                    "user": "slk-bot", "user_id": "U0123456789",
                    "url": "https://acme.slack.com" } },
    { "name": "personal", "active": false, "scope": "user", "checked": false }
  ]
}
```
  - `scope`: `user|bot|unknown|encrypted|none` (1:1 with the text label; one field covers both
    scope and the "unreadable" states — pragmatic over a separate flag).
  - `checked`: whether a live `auth.test` ran for this row (governed by `--offline`/`--all`/
    default-active-only). On success → `identity`; on failure → `error` (e.g.
    `"invalid token: invalid_auth"` / `"offline"`); when not checked → neither.
  - `concise` (default) and `json` are the two branches; `jsonl`/`table` fall back to concise
    (the encryption header does not fit per-row streaming).
- Text sample:
```
encryption: file backend (key at ~/.config/slk/.encryption_key)

* work     [bot]       — Acme (T0123456789) — slk-bot (U0123456789) @ https://acme.slack.com
  personal [user]      — Acme (T0123456789) — Alice (U0123456789) @ https://acme.slack.com
  stale    [encrypted] — (local: token could not be decrypted)
```

### 6.3 `auth login`
- `--as` (the global persistent flag) selects which token to mint: empty/`user` → request
  `user_scope=`; `bot` → request `scope=`. Store the corresponding token from the exchange into
  the profile's single `Token` (`ExchangeCode` already parses both; only the selected one is
  stored). If `--as bot` yields no bot token (the app has no bot user), error clearly.
- **Default scope list is computed at runtime** by unioning the per-command annotations across
  the command tree: `--as user` → union of `userScopes`; `--as bot` → union of `botScopes`. The
  `--scopes` flag still overrides verbatim when set. The hardcoded 36-scope literal is removed.
- Cobra cannot express a dynamic default statically; the union is computed in `RunE` when
  `--scopes` was not changed (the help text describes it as "generated from supported commands").

## 7. Scope SSOT & generation

### 7.1 Annotation schema (the SSOT)
Each command's `Annotations` map carries, alongside the existing `slackMethod`:

| Key | Meaning |
|---|---|
| `userScopes` | comma-separated user-token scopes; empty = no scope needed / local verb |
| `botScopes` | comma-separated bot-token scopes; empty = user-only **or** no scope needed |
| `botCapable` | `"true"` if the verb works at all under a bot token, else `"false"` |

Values are transcribed from the companion blueprint's **§9-A canonical JSON** (one row per verb).
Notable non-uniform rows (companion §9-B): `channels:write` (user) → `channels:manage` (bot) for
every public-channel write/management verb; `channel join` → `channels:join`; `channel
open`/`close` stay `im:write,mpim:write` for both. `canvas read` is annotated
`files:read,canvases:read` (canvases:read kept — §7.4).

### 7.2 Generator
Hook the existing cobra-tree walk (`internal/skillgen`, invoked by `slk generate-skills`) to also
emit the scope artifacts:

- **README app manifest** — regenerate the fenced manifest block (currently `oauth_config.scopes.user`
  only, README ~line 184) to contain **both** `user:` (37) and `bot:` (35) arrays, the "mother
  set" superset for quick-start. Wrap the block in `<!-- BEGIN GENERATED MANIFEST -->` /
  `<!-- END GENERATED MANIFEST -->` markers so the generator replaces a well-defined region.
- **`auth login` defaults** — **not** a generated file; computed at runtime from the same
  annotations (per §6.3). This keeps the login default from ever going stale, with zero new
  artifact.

### 7.3 CI guard
Add a staleness guard analogous to the existing `skill` and `version-sync` CI jobs: regenerate
the manifest and `git diff --exit-code`; a stale README (someone changed a command's scope
annotation without regenerating) fails CI. This is what makes "edit a command → scopes
auto-maintained" actually hold rather than merely be possible.

### 7.4 Locked oracle
A unit test asserts the generated unions equal the companion's §9-E sets **verbatim**:

- **User — 37 scopes**: the legacy 36 + `users:read.email`.
- **Bot — 35 scopes**: user 37 − `search:read`, `users.profile:write`, `dnd:write`,
  − `channels:write` + `channels:manage`, + `channels:join`.

`canvases:read` is **kept** in both sets. Verification (this design): per Slack's reference,
`files.info` requires only `files:read` and `canvases:read` is documented as gating only
`canvases.sections.lookup` (which slk never calls) — but `canvas read` also downloads the
canvas's `url_private` content, which is exactly what `canvases:read` ("Access contents of
canvases") most plausibly gates and which the method-level scope list does not cover. slk's
legacy set always granted it, so there is no negative evidence it can be dropped. Keeping one
read scope is the safe choice; dropping to 36/34 requires an empirical test (read a canvas with a
`files:read`-but-not-`canvases:read` token) and is deferred.

## 8. Migration

Clean break. The `Profile` struct no longer has `user_token`/`bot_token` TOML keys, so a legacy
config's tokens are simply not read; such a profile resolves to an empty `Token` and reports "no
token" through the normal path. No migration command, no on-read rewrite. Justified by
pre-release status (effectively no installed base).

## 9. Verification approach

- **Unit (auth package):** `TokenScope` prefix mapping; `ResolveToken` precedence + assertion
  matrix (rewrite of `precedence_test.go` for the single-token model and assert-not-select
  semantics); single-field encrypt/decrypt round-trip; clean-break (legacy keys ignored).
- **Unit (commands):** `set-token` single-`--token` + stdin + prefix-rejection; `auth status`
  text label + `--format json` shape (via the structured live-check seam, no network);
  `auth login` user/bot authorize-URL parameter selection and token storage (existing
  `waitForCode`/`exchangeCode` seams); the `botCapable=false` guardrail (dry-run/seamed).
- **Unit (skillgen/scopes):** generated user/bot unions equal the §7.4 oracle (37/35); manifest
  region round-trips; CI staleness guard.
- **Manual / real-token (user-run, per the test-execution split):** `auth login --as bot` against
  a real app with a bot user; a user-only verb under a bot token returning exit 3; the deferred
  `canvases:read` empirical check if/when leaning to 36/34.

## 10. Out of scope / non-goals

- Re-deriving the bot capability matrix (companion blueprint owns it).
- `xoxc`/`xoxd` token types; Enterprise-Grid `admin.*` scopes (existing non-goals).
- Hosted OAuth redirect / removing `client_secret` from the client (slk is backend-less; team
  distribution guidance is README-level, not a code change here).
- Trimming the bot set below Approach A (kept as the upper bound; trims are user choice).

## 11. Coordination record — companion blueprint absorbed

The companion's §7 implementation deliverables are **not** independent work; they are subsumed
here because we chose dynamic generation:

| Companion §7 item | Where it lives in this design |
|---|---|
| 1. README manifest `scopes.bot` | §7.2 generator (generated, not hand-edited) |
| 2. README ❌/⚠️ boundary narrative | a docs task in this plan, sequenced after the data is generated; content derives from `botCapable`/`botScopes` |
| 3. `--as bot` short-circuit user-only verbs | §5 command guardrail |
| 4. `auth login --as bot` requests `scope=` | §6.3 |

Consequently there is nothing in the companion to dispatch as separate/parallel implementation;
all of it is in this plan. Parallel subagents are appropriate **during execution** for mechanical,
independent chunks (e.g. transcribing §9-A annotations per command group under the §7.1 schema).
