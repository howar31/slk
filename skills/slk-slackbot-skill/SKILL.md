---
name: slk-slackbot-skill
description: "Manage Slackbot skills"
metadata:
  version: 0.9.1
  openclaw:
    category: "productivity"
    requires:
      bins:
        - slk
    cliHelp: "slk slackbot-skill --help"
---

# slk slackbot-skill

Manage Slackbot skills

> **PREREQUISITE:** Read `../slk-shared/SKILL.md` for auth, global flags, security rules, and exit codes. If missing, run `slk generate-skills`.

| Command | Description |
|---------|-------------|
| `slk slackbot-skill create` | Create a Slackbot skill from markdown or from a canvas |
| `slk slackbot-skill delete` | Delete a Slackbot skill and its canvas |
| `slk slackbot-skill disable` | Turn a skill off in the caller's Slackbot library |
| `slk slackbot-skill enable` | Turn a skill on in the caller's Slackbot library |
| `slk slackbot-skill info` | Read one Slackbot skill with its instructions |
| `slk slackbot-skill list` | List Slackbot skills |
| `slk slackbot-skill update` | Update a Slackbot skill's instructions or metadata |

## slk slackbot-skill create

Create a Slackbot skill from markdown or from a canvas

**Slack API:** `slackbot.skills.create`

```bash
slk slackbot-skill create [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--canvas` | — | — | ID of a canvas you own to turn into a skill |
| `--category` | — | — | skill category |
| `--description` | — | — | when Slackbot should use the skill (max 400 characters) |
| `--example-prompt` | — | — | example prompt (max 200 characters; {skill} becomes the skill mention) |
| `--instructions` | — | — | skill instructions in markdown |
| `--instructions-file` | — | — | path to a markdown file (use - for stdin) |
| `--mcp-servers` | — | — | JSON array of apps the skill needs |
| `--name` | ✓ | — | display name (max 255 characters) |

**Tips:** Creates a skill from markdown (--instructions or --instructions-file) or from a canvas the caller owns (--canvas); pass exactly one. The skill is added to the caller's own Slackbot library. --category is one of: communication, customer_support, data_and_analytics, engineering, product_management, productivity, professional_development, project_management, sales, system, team_management, other. --mcp-servers is a JSON array of the apps the skill needs.

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk slackbot-skill delete

Delete a Slackbot skill and its canvas

**Slack API:** `slackbot.skills.delete`

```bash
slk slackbot-skill delete [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | skill ID |

**Tips:** Deletes the skill and its underlying canvas, and removes it from every library, skill set, and the org catalog. To turn a skill off only for yourself, use slackbot-skill disable.

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk slackbot-skill disable

Turn a skill off in the caller's Slackbot library

**Slack API:** `slackbot.skills.library.remove`

```bash
slk slackbot-skill disable [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | skill ID |

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk slackbot-skill enable

Turn a skill on in the caller's Slackbot library

**Slack API:** `slackbot.skills.library.add`

```bash
slk slackbot-skill enable [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | skill ID |

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk slackbot-skill info

Read one Slackbot skill with its instructions

**Slack API:** `slackbot.skills.info`

```bash
slk slackbot-skill info [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | skill ID (a canvas file ID or a builtin: reference) |

## slk slackbot-skill list

List Slackbot skills

**Slack API:** `slackbot.skills.list`

```bash
slk slackbot-skill list [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--category` | — | — | only skills in this category |
| `--filter` | — | `library` | library, owned, or catalog |

**Tips:** Lists skills from the caller's point of view. --filter is library (enabled for the caller, default), owned (created by the caller), or catalog (the organization's official skills). Instructions are not included; use slackbot-skill info.

## slk slackbot-skill update

Update a Slackbot skill's instructions or metadata

**Slack API:** `slackbot.skills.update`

```bash
slk slackbot-skill update [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--category` | — | — | new category (empty clears) |
| `--description` | — | — | new description (empty clears) |
| `--example-prompt` | — | — | new example prompt (empty clears) |
| `--id` | ✓ | — | skill ID |
| `--instructions` | — | — | new instructions in markdown (replaces the whole document) |
| `--instructions-file` | — | — | path to a markdown file (use - for stdin) |
| `--mcp-servers` | — | — | JSON array of apps the skill needs (replaces the list) |
| `--name` | — | — | new display name |

**Tips:** Changes only the fields passed. Instructions replace the whole document; --mcp-servers replaces the whole list ('[]' clears it). An empty --description, --category, or --example-prompt clears that field.

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.


