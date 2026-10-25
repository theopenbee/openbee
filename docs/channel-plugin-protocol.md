# Channel plugin protocol (v1)

A channel plugin connects openbee to a chat platform that is not built in. It runs as a child process of openbee and talks to it over stdin/stdout. Plugins can be written in any language.

## Package layout

A plugin is a directory with an `openbee.plugin.json` manifest at its root:

```json
{
  "protocol_version": 1,
  "id": "discord",
  "name": { "en": "Discord", "zh": "Discord" },
  "description": { "en": "Discord bot channel" },
  "command": ["node", "dist/index.js"],
  "version": "0.1.0",
  "config": [
    { "key": "token", "type": "secret", "required": true, "label": { "en": "Bot token", "zh": "机器人 Token" } },
    { "key": "allowed_guilds", "type": "list" },
    { "key": "max_media_size", "type": "int", "default": 10485760 }
  ]
}
```

| Field | Rules |
|---|---|
| `protocol_version` | Must be `1`. |
| `id` | Channel id, session key prefix and `config.yaml` key. Matches `^[a-z][a-z0-9_-]{1,31}$`. Cannot be `local`. |
| `name`, `description` | A string or an object keyed by language (`en`, `zh`). Lookups fall back to `en`. |
| `command` | Program and arguments, run with the plugin directory as working directory. An absolute path is used as-is; a path starting with `.` or containing a separator is relative to the plugin directory; anything else is looked up on `PATH`. |
| `setup` | Optional. `{"command": [...], "label": ...}`: an interactive step `openbee config` runs before asking for `config[]`, for platforms that log in with a QR code or a browser instead of a pasted token. `command` follows the same rules as the top-level `command`; `label` names the step in prompts. See [Setup](#setup). |
| `version` | Optional. `package.json` `version` wins when present. |
| `config[]` | Settings openbee asks for in `openbee config` and validates at startup. `type` is `string`, `secret`, `bool`, `int` or `list` (list of strings). `required`, `default`, `label` and `help` are optional. `random_default: true` (only for `string`) makes `openbee config` suggest a random value when the field is empty, which suits access codes. `enabled` and `bot_name` are reserved keys and cannot be declared. |

One plugin provides exactly one channel.

## Installation

```sh
openbee plugin install <npm-spec | directory>
openbee plugin list
openbee plugin remove <id>
```

- A directory argument is registered in place and loaded from that directory on every start.
- Anything else is passed to `npm install --save` inside `~/.openbee/plugins` (any spec npm accepts: `name@version`, `github:owner/repo`, a tarball URL).
- Installed plugins are recorded in `~/.openbee/plugins/plugins.json`. Each plugin gets a private writable directory `~/.openbee/plugins/data/<id>/`.
- Installing or removing a plugin takes effect after openbee restarts.

Plugins run with the user's permissions. Only install plugins you trust.

## Configuration

Plugin channels are configured next to the built-in ones:

```yaml
bee:
  platforms:
    discord:
      enabled: true
      bot_name: OpenBee
      token: "..."
      allowed_guilds: ["123"]
```

At startup openbee, for every section with `enabled: true` that is not a built-in channel:

1. Finds the installed plugin with that id.
2. Applies manifest defaults, checks required fields and types, and removes `enabled` and `bot_name`.
3. Starts the plugin and sends the result as `initialize.config`.

If any step fails, openbee logs an error and skips only that channel. `bot_name` behaves as for built-in channels: an `@bot_name` mention is stripped from incoming text, and no worker may use that name.

## Setup

When the user enables a plugin whose manifest declares `setup`, `openbee config` runs the setup command before asking for the `config[]` fields. If the section already has a value for every required field, it first asks whether to run the setup again.

- The command runs in the plugin directory with stdin and stderr attached to the user's terminal, so it can show a QR code and ask questions there. `OPENBEE_LANGUAGE` holds the user's language (`en` or `zh`).
- On success it exits 0 and writes one JSON object to stdout, for example `{"token": "...", "user_id": "..."}`. Every key must be a declared `config[]` field and every value must match its type; `null` values are ignored. openbee stores the values in the channel's section and does not ask for those fields again.
- On failure (non-zero exit or invalid output) openbee shows the error and asks for every field by hand.

## Process rules

- **stdout carries protocol messages only.** Anything else written there is logged and discarded. Write logs to stderr; every stderr line is copied into the openbee log with component `plugin:<id>`.
- **Exit when stdin closes.** EOF on stdin means openbee is gone (it may have crashed or been killed). A plugin that keeps running would answer users twice after openbee restarts.
- Secrets arrive only in `initialize`. They are never passed in arguments or environment variables. The environment is inherited from openbee.
- openbee restarts a plugin that exits unexpectedly. The delay starts at 1s and doubles up to 60s; after 10 minutes of healthy uptime it resets.
- openbee does not restart a plugin that answers `initialize` with error `-32001`, reports a different `protocol_version`, or whose command cannot be found.
- On shutdown openbee sends `shutdown` and waits up to 5s, then closes stdin, waits 2s more and kills the process group.

## Transport

UTF-8 JSON-RPC 2.0, one message per line (`\n`-terminated). Lines longer than 8 MiB are dropped. Both sides ignore unknown fields, so new optional fields can be added within version 1. openbee answers any request a plugin sends with error `-32601`.

### openbee → plugin

#### `initialize` (request, 30s)

```json
{
  "protocol_version": 1,
  "openbee_version": "0.0.46",
  "channel_id": "discord",
  "language": "zh",
  "bot_name": "OpenBee",
  "config": { "token": "...", "allowed_guilds": ["123"], "max_media_size": 10485760 },
  "data_dir": "/home/u/.openbee/plugins/data/discord",
  "media_dir": "/home/u/.openbee/media/inbound"
}
```

Reply `{"protocol_version": 1}` once the plugin is connected to its platform. After replying the plugin may send `inbound` notifications.

Errors:

| Code | Meaning | openbee reaction |
|---|---|---|
| `-32001` | Config is invalid (missing or rejected credentials, …). | Log, never restart. |
| any other | Start failed. | Log, restart with backoff. |

#### `send` (request)

```json
{
  "session_key": "discord:123:456",
  "chat_id": "123",
  "session_scope": "456",
  "proactive": false,
  "reply_to": { "message_id": "m1", "raw": "<raw from the inbound message>" },
  "content": "text",
  "media": { "path": "/abs/path/report.pdf", "name": "report.pdf" }
}
```

- At least one of `content` and `media` is present. When both are present, the plugin decides how to deliver them (for example text first, then the file).
- `chat_id` and `session_scope` are split from the session key by openbee, so the plugin never has to parse it.
- `reply_to` is omitted when there is nothing to reply to. `reply_to.message_id` is the plugin's own `message_id` (without the channel prefix); `reply_to.raw` is the plugin's own `raw`, returned unchanged. If the channel id used to belong to a built-in channel, messages stored before the switch carry that channel's raw payload instead, without `message_id`.
- `proactive: true` means openbee is starting the conversation (scheduled task results, notifications). The plugin should post to `chat_id` without quoting a message.
- Reply `{}` or `{"platform_message_id": "..."}`. Error messages are passed to the caller (for example a worker running `openbee ctl message send`).
- The deadline follows the caller and is capped at 120s; for replies sent through `openbee ctl` it is about 30s. When the caller gives up, openbee sends `$/cancelRequest`.

#### `$/cancelRequest` (notification)

`{"id": <request id>}`. The plugin should stop working on that request if it can; whatever it already posted stays posted.

#### `shutdown` (request, 5s)

Disconnect from the platform, reply `{}`, then exit.

### plugin → openbee

#### `inbound` (notification)

```json
{
  "chat_id": "123",
  "sender_id": "456",
  "session_scope": "456",
  "message_id": "m1",
  "text": "@OpenBee summarize this",
  "attachments": [{ "type": "image", "path": "/home/u/.openbee/media/inbound/x.png", "name": "x.png" }],
  "raw": "<any string the plugin wants back when replying>",
  "context": { "guild_id": "9" },
  "time": 1761300000000
}
```

| Field | Rules |
|---|---|
| `chat_id` | Required. Must not contain `:`. Used for proactive sends. |
| `sender_id` | Required. |
| `session_scope` | Optional, defaults to `sender_id`. The session key is `<id>:<chat_id>:<session_scope>`; the scope may contain `:` (for example `thread:user`). |
| `message_id` | Optional. Used for deduplication (stored as `<id>:<message_id>`) and returned in `send.reply_to`. |
| `text`, `attachments` | At least one must be non-empty. |
| `attachments[].path` | Must be an absolute path inside `media_dir`; otherwise openbee keeps only the file name. `type` is `image`, `audio`, `video`, `document` or `sticker` (anything else becomes `document`). |
| `raw` | Optional opaque string, returned in `send.reply_to.raw`. |
| `context` | Optional JSON object, passed to workers as `platform_context` in the task metadata. |
| `time` | Optional Unix time in milliseconds; openbee uses its own clock when absent. |

Invalid messages are logged and dropped. openbee renders attachments into the message text itself, so plugins do not need to know openbee's attachment markup.

#### `migrate_chat` (notification)

```json
{ "from_chat_id": "-123", "to_chat_id": "-100123" }
```

Tells openbee that a chat got a new id (for example a Telegram group upgraded to a supergroup). openbee moves pending and running tasks from session keys starting with `<id>:<from_chat_id>:` to `<id>:<to_chat_id>:`, so scheduled results keep reaching the chat. Both ids follow the `chat_id` rules; invalid notifications are logged and dropped.

#### `log` (notification)

`{"level": "debug" | "info" | "warn" | "error", "message": "...", "fields": {...}}` is written to the openbee log with component `plugin:<id>`.

## Error codes

| Code | Used for |
|---|---|
| `-32001` | `initialize`: config invalid. |
| `-32601` | Unknown method. |
| `-32603` | `initialize`: unexpected start failure (Node SDK). |
| `-32000` | `send` failed (Node SDK). |
| `-32800` | `send` cancelled (Node SDK). |
