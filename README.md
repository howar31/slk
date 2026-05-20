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

### Multi-line content

Bash double-quoted `"\n"` is a literal backslash-n, not a newline. To pass
real multi-line content to `canvas create/update` or any `msg`/`thread`
send, use the file or stdin alternative:

```bash
# Read markdown from a file
slk canvas create --title "Weekly" --markdown-file weekly.md

# Read text from stdin (use "-" as the path)
cat weekly.md | slk canvas update --id F0… --action prepend --markdown-file -
```

`--markdown-file` is available on `canvas create` / `canvas update`;
`--text-file` is available on `msg send` / `msg draft` / `msg schedule` /
`msg update` / `thread reply`.

### Drafts

`msg draft` creates a draft via Slack's `drafts.create` endpoint, which
accepts user tokens. The companion lifecycle endpoints (`drafts.list`,
`drafts.delete`, `drafts.update`) require a Slack-client token type that
is not available to OAuth user tokens. To list, edit, or delete drafts,
use the Slack desktop or web client's "Drafts & Sent" panel. The URL
emitted by `msg draft` opens the channel where the draft lives.

## MCP vs CLI: token cost

**CLI 明顯比較友善**，主要差距在四個面向：

### 1. Tool schema 常駐成本
- **MCP**：13 個 Slack 工具的 JSONSchema 全部進 context（即使用 deferred ToolSearch，呼叫過的 schema 都會留下）。粗估 5–10K tokens。
- **CLI**：對 LLM 來說只是 `Bash` 一個工具。`slk --help` 是純文字、且只有需要時才讀。

### 2. 回傳大小（這一輪最有感）
本 session 的對比：

| 動作 | MCP 預期回傳 | slk 實際回傳 |
|---|---|---|
| send message | 整個 message object（channel/user/ts/blocks/team…），約 200–400 tokens | `sent 1779236987.634179` ≈ 7 tokens |
| read 3 messages | 每筆完整 metadata，約 1–2K tokens | concise JSON 三筆 ≈ 150 tokens |
| search users | 完整 user object（profile 含 avatar URLs、time_zone…）每筆 500+ tokens | `{name,id,extra}` 每筆 ≈ 20 tokens |
| delete | `{"ok":true, ...}` 含完整 channel/ts echo | `delete ok` 2 tokens |

slk 的 `--format concise` + 預設 `--no-resolve` 解析（ID→名稱）讓模型不需要再追問 user 名字，省第二次 round trip。

### 3. 可預先過濾
- **CLI**：可串 `jq`、`grep`、`head`，bytes 在進 context 前就被砍掉。
- **MCP**：整包結果一定落地，模型才能讀。

### 4. Opt-in 詳細模式
slk 的 `--raw` 與 `--format json` 是「需要才開」；MCP 通常是「全送」。當你只是要確認某個動作成功，CLI 給 2–10 tokens 的確認句即可，MCP 給整包 JSON。

### 例外（MCP 較佳的場景）
- 需要 **嚴格 schema 驗證** 或下游程式直接消費結構化欄位時，MCP 的型別契約對 LLM tool-call 比較穩。
- 純對話介面、模型不被允許執行 shell 時，MCP 是唯一選擇。

### 結論
日常使用，CLI 約可省下 **單次呼叫 5–20x、整段會話 2–5x** 的 token；MCP 的優勢主要在「沒有 shell 環境」或「需要結構化 schema 契約」。slk 已經把 `concise / --no-resolve / --raw` 三層粒度做好，本質上就是為了在 agent 場景把 token 開銷壓到最小，這個方向是對的。

## License

MIT
