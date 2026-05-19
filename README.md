# slk

Agent-facing Slack CLI. Read and send Slack messages, manage canvases, lists,
and channels — with token-efficient output designed for AI agents.

## Install

```bash
brew install howar31/tap/slk
```

Or download a binary from the Releases page.

## Auth

`slk` uses your own Slack app. Create one at <https://api.slack.com/apps>, add
user scopes, then either:

```bash
# Paste a token directly
slk auth set-token --profile work --workspace acme --user xoxp-... --bot xoxb-...

# Or run the OAuth flow
slk auth login --profile work --client-id ... --client-secret ...
```

## Usage

```bash
slk msg read --channel C0123456789 --limit 20
slk msg send --channel C0123456789 --text "hello"
slk search channels
slk canvas create --title "Plan" --markdown "# Heading"
slk list create --title "Backlog"
slk channel archive --channel C123
slk api conversations.info --params '{"channel":"C123"}'
```

Global flags: `--format concise|json|jsonl|table`, `--as user|bot`,
`--profile`, `--raw`, `--dry-run`, `--no-resolve`.

## License

MIT
