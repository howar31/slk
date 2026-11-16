---
name: slk-list
description: "Create and manage Slack Lists"
metadata:
  version: 0.10.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - slk
    cliHelp: "slk list --help"
---

# slk list

Create and manage Slack Lists

> **PREREQUISITE:** Read `../slk-shared/SKILL.md` for auth, global flags, security rules, and exit codes. If missing, run `slk generate-skills`.

| Command | Description |
|---------|-------------|
| `slk list add-item` | Add an item to a List |
| `slk list create` | Create a new List |
| `slk list delete-item` | Delete one item from a List |
| `slk list delete-items` | Delete several items from a List |
| `slk list export` | Export a List to a CSV or JSON file |
| `slk list item-info` | Read one item of a List |
| `slk list read` | Read items in a List |
| `slk list share` | Set access on a List for users or channels |
| `slk list unshare` | Remove access on a List for users or channels |
| `slk list update` | Update metadata of a List |
| `slk list update-item` | Update an item in a List |

## slk list add-item

Add an item to a List

**Slack API:** `slackLists.items.create`

```bash
slk list add-item [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--fields` | — | `[]` | initial fields as JSON array (see Long help for shape) |
| `--id` | ✓ | — | list ID |

**Tips:** Add an item to a List.

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk list create

Create a new List

**Slack API:** `slackLists.create`

```bash
slk list create [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--title` | ✓ | — | list title |

**Tips:** Create a Slack List. Lists cannot be deleted via the public API (slackLists.delete does not exist) — remove them in the Slack UI.

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk list delete-item

Delete one item from a List

**Slack API:** `slackLists.items.delete`

```bash
slk list delete-item [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | list ID |
| `--row-id` | ✓ | — | row (item) ID to delete |

**Tips:** Deletes one List item. The whole-list delete API (slackLists.delete) does not exist — remove a List in the Slack UI.

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk list delete-items

Delete several items from a List

**Slack API:** `slackLists.items.deleteMultiple`

```bash
slk list delete-items [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | list ID |
| `--row-ids` | ✓ | — | comma-separated row (item) IDs to delete |

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk list export

Export a List to a CSV or JSON file

**Slack API:** `slackLists.download.start`

```bash
slk list export [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--file-format` | — | `csv` | export format: csv or json |
| `--force` | — | — | overwrite an existing file |
| `--id` | ✓ | — | list ID |
| `--include-archived` | — | — | include archived items |
| `--include-attachments` | — | — | include attachment metadata (json only) |
| `--include-threads` | — | — | include each item's thread (json only) |
| `--out` | — | — | output file or directory (default: <list ID>.<format> in the current directory) |
| `--timeout` | — | `1m0s` | how long to wait for the export job |

**Tips:** Starts an export job (slackLists.download.start), waits for it (slackLists.download.get), and saves the file. --file-format is csv or json; --include-threads and --include-attachments apply to json only. With --raw, prints the slackLists.download.get response, which carries download_url, and saves nothing. An existing file is kept unless --force is set.

## slk list item-info

Read one item of a List

**Slack API:** `slackLists.items.info`

```bash
slk list item-info [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | list ID |
| `--row-id` | ✓ | — | row (item) ID |

## slk list read

Read items in a List

**Slack API:** `slackLists.items.list`

```bash
slk list read [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | list ID |

## slk list share

Set access on a List for users or channels

**Slack API:** `slackLists.access.set`

```bash
slk list share [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--access-level` | ✓ | — | access level: read, write, or owner |
| `--channels` | — | — | comma-separated channel IDs to grant access |
| `--id` | ✓ | — | list ID |
| `--users` | — | — | comma-separated user IDs to grant access |

**Tips:** Sets access on a List. --access-level is read, write, or owner; owner applies to --users only.

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk list unshare

Remove access on a List for users or channels

**Slack API:** `slackLists.access.delete`

```bash
slk list unshare [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--channels` | — | — | comma-separated channel IDs to remove access |
| `--id` | ✓ | — | list ID |
| `--users` | — | — | comma-separated user IDs to remove access |

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk list update

Update metadata of a List

**Slack API:** `slackLists.update`

```bash
slk list update [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--description` | — | — | new description for the list |
| `--id` | ✓ | — | list ID |
| `--name` | — | — | new name for the list |
| `--todo-mode` | — | — | todo mode string (e.g. "on" or "off") |

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.

## slk list update-item

Update an item in a List

**Slack API:** `slackLists.items.update`

```bash
slk list update-item [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--fields` | — | `[]` | updated cells as JSON array (see Long help for shape) |
| `--id` | ✓ | — | list ID |
| `--row-id` | — | — | row to update; injected as cells[].row_id when a cell omits it |

**Tips:** Update an item in a List.

> [!CAUTION]
> Write command — confirm with the user before executing; preview with `--dry-run`.


