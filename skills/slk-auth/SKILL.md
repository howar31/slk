---
name: slk-auth
description: "Manage Slack credentials"
metadata:
  version: 0.4.0
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
| `--client-id` | ✓ | — | your Slack app client ID |
| `--client-secret` | ✓ | — | your Slack app client secret |
| `--port` | — | `3000` | local callback port |
| `--profile` | — | `default` | profile name |
| `--scopes` | — | `channels:history,channels:read,channels:write,groups:history,groups:read,groups:write,im:history,im:read,im:write,mpim:history,mpim:read,mpim:write,chat:write,reactions:write,reactions:read,search:read,users:read,users:write,users.profile:read,users.profile:write,files:read,files:write,canvases:read,canvases:write,lists:read,lists:write,pins:read,pins:write,bookmarks:read,bookmarks:write,team:read,emoji:read,dnd:read,dnd:write,usergroups:read,usergroups:write` | comma-separated user scopes |
| `--workspace` | — | — | workspace label |

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
| `--bot` | — | — | bot token (xoxb-) |
| `--profile` | — | `default` | profile name |
| `--user` | — | — | user token (xoxp-) |
| `--workspace` | — | — | workspace label |

## slk auth status

Show configured profiles

```bash
slk auth status
```

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


