# Gotchas

## Environment

- Inside an openbee worker, the agent's environment carries `OPENBEE_URL`, `OPENBEE_API_KEY` and `OPENBEE_MESSAGE_ID` for the real instance, and the real `openbee` binary may be first on PATH. `$T` runs every test command with a clean environment and the env's own binary, so these never leak in.
- `$T mcp` and `$T js` sign a bee token with the env's `bee.rpc.token_secret`, which the script always sets. Builds from before MCP have no `/mcp`; drive them with `$T ctl -- <ctl args>` instead.
- `server.env_secret` must be 64 hex chars (32 bytes), or startup fails with "key must be 32 bytes".
- The first admin can only be created with `POST /api/setup`. Setting `auth.password` in the config doesn't create a user.
- `pi -p` hangs if stdin is left open.
- macOS has no `timeout`; use `perl -e 'alarm N; exec @ARGV' cmd ...`. zsh doesn't word-split `$VAR`, so never store a command in a string variable.
- A mock HOME doesn't isolate `/tmp`. The script points the server's `TMPDIR` at the env, but a worker that writes `/tmp/...` literally still escapes. Check `/tmp` after down when workers generated files.
- Read the DB only through `$T db` or `$T watch`. On builds without WAL (before #190), any plain reader makes the server log `SQLITE_BUSY`, so the script opens those DBs with `immutable=1`. On WAL builds it uses `mode=ro`.

## Upgrade-compat check (channel moved to a plugin)

1. Insert a `bee_platform_messages` row whose `raw` uses the old built-in format (non-envelope JSON), with status `bee_processed` and an older `received_at`. `$T db` is read-only, so write with `sqlite3 <db path>`, and only while no message is in flight.
2. Reply to the row as bee: `$T mcp send_message '{"message_id": "<row id>", "content": "..."}'`.

## Platforms

- **Telegram:**
  - Privacy mode hides @mentions in groups. Turn Group Privacy off and re-add the bot.
  - Proactive sends stay in the forum topic the conversation came from.
  - Don't call `getUpdates` while an instance runs; it steals updates from the plugin's long poll.
  - The plugin manifest gives `auth_code` a random default. The script clears it unless you pass `--set auth_code=...`.
- **Discord:**
  - Check the bot with `GET /api/v10/users/@me` and its guilds with `/users/@me/guilds`.
  - Check MESSAGE CONTENT INTENT via `/applications/@me` `flags` (bit 18 or 19).
  - With the intent off, discord.js rejects login with a plain `Error("Used disallowed intents")`.
- **Feishu:** bad credentials give tenant token `code 10014`. Without chat/reaction read scopes, reactions and group replies can only be checked visually.
- **DingTalk:**
  - Bad credentials give HTTP 400 `invalidClientIdOrSecret` (the gateway returns 401 `authFailed`).
  - `sessionWebhook` expires 90 min after the user's last message. Simulate expiry with `json_set(raw,'$.sessionWebhookExpiredTime',1)` on the latest message.
  - CSV attachments can't be previewed in the app.
- **WeCom:**
  - Node's built-in (undici) WebSocket gets close 1006 from `openws.work.weixin.qq.com`; the `ws` package works.
  - Bad bot_id/secret gives subscribe ack errcode 853000.
  - Restarting while a reply streams leaves a stuck "thinking" bubble, so never restart mid-message.
- **WeChat:**
  - Each QR code lives about 2 minutes, with at most 3 per login, so confirm the user is ready first.
  - The bot sends at most 10 replies per user message; after that, sends fail with `ret=-2 errmsg=prepare failed`.
- **Linear:**
  - A test key often belongs to a real workspace whose issues already carry the `openbee` label. Never point a test instance at that label.
  - Call GraphQL from node; Python urllib may fail TLS verification on macOS.
  - Reactions created as `:eyes:` read back as `eyes`.
  - Deleted issues stay in Linear's trash for 30 days.

## npm

- Right after a publish, don't probe the new tarball URL before it exists; the 404 gets cached for a few minutes. Use `npm install --prefer-online` to skip the local packument cache.
- While the plugin catalog is unpublished, install by package (the script's default `npm:` spec), or pass `--env OPENBEE_PLUGIN_CATALOG=<local catalog.json>` to `up`.
