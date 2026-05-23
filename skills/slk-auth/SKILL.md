---
name: slk-auth
description: "Manage Slack credentials"
metadata:
  version: 0.7.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - slk
    cliHelp: "slk auth --help"
---

# slk auth

Manage Slack credentials

> **PREREQUISITE:** Read `../slk-shared/SKILL.md` for auth, global flags, security rules, and exit codes. If missing, run `slk generate-skills`.

| Command | Description |
|---------|-------------|
| `slk auth login` | Run the OAuth flow with your own Slack app credentials |
| `slk auth logout` | Remove a profile |
| `slk auth revoke` | Revoke the active token at Slack (server-side) |
| `slk auth set-token` | Store tokens for a profile |
| `slk auth status` | Show configured profiles |
| `slk auth switch` | Set the active profile |
| `slk auth test` | Verify the active token and show its live identity |

## slk auth login

Run the OAuth flow with your own Slack app credentials

```bash
slk auth login [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--client-id` | — | — | your Slack app client ID |
| `--client-secret` | — | — | your Slack app client secret |
| `--non-interactive` | — | — | never prompt; require values via flags |
| `--port` | — | `3000` | local callback port |
| `--profile` | — | `default` | profile name |
| `--scopes` | — | `channels:history,channels:read,channels:write,groups:history,groups:read,groups:write,im:history,im:read,im:write,mpim:history,mpim:read,mpim:write,chat:write,reactions:write,reactions:read,search:read,users:read,users:write,users.profile:read,users.profile:write,files:read,files:write,canvases:read,canvases:write,lists:read,lists:write,pins:read,pins:write,bookmarks:read,bookmarks:write,team:read,emoji:read,dnd:read,dnd:write,usergroups:read,usergroups:write` | comma-separated user scopes |

**Tips:** Run the OAuth flow with your own Slack app credentials. Pass values via flags for scripts/agents, or run with missing fields in a terminal to be prompted (the client secret is entered hidden).

## slk auth logout

Remove a profile

```bash
slk auth logout <profile>
```

## slk auth revoke

Revoke the active token at Slack (server-side)

**Slack API:** `auth.revoke`

```bash
slk auth revoke
```

**Tips:** Revoke the active token at Slack. This invalidates the token server-side; it does not remove the local profile (use `auth logout` for that).

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk auth set-token

Store tokens for a profile

```bash
slk auth set-token [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--bot` | — | — | bot token (xoxb-, or - to read stdin) |
| `--non-interactive` | — | — | never prompt; require values via flags |
| `--profile` | — | `default` | profile name |
| `--user` | — | — | user token (xoxp-, or - to read stdin) |

**Tips:** Store tokens for a profile. Pass values via flags for scripts/agents, or run with missing fields in a terminal to be prompted (token entry is hidden). Use --user - / --bot - to read a token from stdin.

## slk auth status

Show configured profiles

```bash
slk auth status [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--all` | — | — | verify every profile live, not just the active one |
| `--offline` | — | — | skip the live Slack check; list local info only |

**Tips:** Show configured profiles. By default the active profile is verified live against Slack (auth.test); pass --all to verify every profile, or --offline to list local info only without any network call.

## slk auth switch

Set the active profile

```bash
slk auth switch <profile>
```

## slk auth test

Verify the active token and show its live identity

**Slack API:** `auth.test`

```bash
slk auth test
```


