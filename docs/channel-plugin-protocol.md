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
| `id` | Channel id and session key prefix. Matches `^[a-z][a-z0-9_-]{1,31}$`. Cannot be `local`. |
| `name`, `description` | A string or an object keyed by language (`en`, `zh`). Lookups fall back to `en`. |
| `command` | Program and arguments, run with the plugin directory as working directory. `node` runs on the JavaScript runtime openbee picks (see [Runtime](#runtime)). Otherwise an absolute path is used as-is; a path starting with `.` or containing a separator is relative to the plugin directory; anything else is looked up on `PATH`. |
| `setup` | Optional. `{"command": [...], "label": ...}`: an interactive step `openbee plugin configure` runs before asking for `config[]`, for platforms that log in with a QR code or a browser instead of a pasted token. `command` follows the same rules as the top-level `command`; `label` names the step in prompts. See [Setup](#setup). |
| `version` | Optional. `package.json` `version` wins when present. |
| `config[]` | Settings openbee stores for the plugin and validates before starting it. `type` is `string`, `secret`, `bool`, `int` or `list` (list of strings). `required`, `default`, `label` and `help` are optional. `random_default: true` (only for `string`) makes openbee fill in a random value when the field has no value yet, which suits access codes. `enabled` and `bot_name` are reserved keys and cannot be declared. |

One plugin provides exactly one channel.

## Installation

```sh
openbee plugin search [keyword]
openbee plugin install <catalog id | npm-spec | directory>...
openbee plugin configure <id> [--set key=value]... [--bot-name name] [--enable]
openbee plugin list
openbee plugin enable|disable|restart <id>
openbee plugin update [id...]
openbee plugin remove <id> [--yes]
```

These commands talk to the running openbee, signing a short-lived token with `bee.rpc.token_secret` from `config.yaml`. They refuse to run inside openbee's own agents.

- A catalog id installs the version listed in the plugin catalog. A bare name that is not in the catalog is rejected; write `npm:<name>` to install that npm package.
- A directory argument is registered in place and loaded from that directory on every start.
- Anything else is installed into `~/.openbee/plugins` by the package manager of the [runtime](#runtime): `npm install --save` on Node.js, `bun add` on Bun (any spec npm accepts: `name@version`, `github:owner/repo`, a tarball URL).
- Installed plugins are recorded in `~/.openbee/plugins/plugins.json`. Each plugin gets a private writable directory `~/.openbee/plugins/data/<id>/`.
- A running openbee installs, upgrades and removes plugins without restarting; see [Configuration](#configuration).

Plugins run with the user's permissions. Only install plugins you trust.

## Runtime

A `command` (or `setup.command`) starting with `node` runs on the first runtime found:

1. `bun` on `PATH`.
2. The Bun openbee installed in `~/.openbee/runtime/bun-1.4.4`.
3. `node` on `PATH`.
4. None of these: openbee downloads Bun 1.4.4 for the current OS and CPU into `~/.openbee/runtime/bun-1.4.4` and uses it.

`openbee plugin install` and `remove` pick a package manager the same way, except that step 3 needs `npm` next to `node`: `bun add`/`bun remove` on Bun, `npm install`/`npm uninstall` on Node.js. Plugins should therefore work on both Node.js and Bun.

The local Bun is private to openbee: `PATH` and shell profiles are not changed, and Bun's package and transpiler caches stay in its directory. It is downloaded from GitHub releases, falling back to npmmirror, and checked against a SHA-256 built into openbee. Set `OPENBEE_BUN_MIRROR` to download from another mirror with the same layout. An openbee release that pins a newer Bun installs it the next time it needs a local Bun and removes the older versions.

## Configuration

openbee stores each plugin's settings in its database, encrypted with `server.env_secret`. `config.yaml` no longer holds plugin settings; a leftover `bee.platforms` section is ignored. A running openbee manages plugins through its HTTP API (`/api/plugins`, permissions `plugins:read` and `plugins:write`):

| Request | Effect |
|---|---|
| `POST /api/plugins/install` `{"spec": "..."}` | Installs or upgrades in the background and returns an operation; poll `GET /api/plugins/operations/<id>`. An upgrade stops the running plugin, installs, and starts it again. |
| `PUT /api/plugins/<id>/settings` `{"bot_name": "...", "config": {...}}` | Validates against `config[]` and saves. A `secret` field sent empty keeps its stored value. A `random_default` field that has no value gets a random one. Restarts only this plugin when it is enabled. |
| `POST /api/plugins/<id>/enable`, `/disable`, `/restart` | Starts or stops only this plugin. |
| `DELETE /api/plugins/<id>` | Stops the plugin, removes the package, and deletes its settings and `~/.openbee/plugins/data/<id>/`. |
| `GET /api/plugins`, `GET /api/plugins/<id>` | Lists plugins with their state (`not_configured`, `disabled`, `starting`, `running`, `retrying`, `failed`) and last error. Secret values are masked. |
| `GET /api/plugins/catalog` | Reads the plugin catalog `@theopenbee/plugins` from npm (npmmirror as a fallback, cached for 10 minutes). `OPENBEE_PLUGIN_CATALOG` can point to a local `catalog.json` file or URL. |

When openbee starts it starts every enabled plugin. Before it sends `initialize.config` it applies manifest defaults, checks required fields and types, and leaves out `bot_name`. A plugin that fails to start is marked `failed`; other channels are not affected. `bot_name` behaves as for built-in channels: an `@bot_name` mention is stripped from incoming text, and no worker may use that name.

## Setup

`openbee plugin configure <id>` runs the setup command of a plugin whose manifest declares `setup`, on the machine where openbee runs, before asking for the `config[]` fields, and saves the result as the plugin's settings.

- The command runs in the plugin directory with stdin and stderr attached to the user's terminal, so it can show a QR code and ask questions there. `OPENBEE_LANGUAGE` holds the user's language (`en` or `zh`).
- On success it exits 0 and writes one JSON object to stdout, for example `{"token": "...", "user_id": "..."}`. Every key must be a declared `config[]` field and every value must match its type; `null` values are ignored. `openbee plugin configure` saves the values and does not ask for those fields again.
- On failure (non-zero exit or invalid output) openbee shows the error and asks for every field by hand.

## Process rules

- **stdout carries protocol messages only.** Anything else written there is logged and discarded. Write logs to stderr; every stderr line is copied into the openbee log with component `plugin:<id>`.
- **Exit when stdin closes.** EOF on stdin means openbee is gone (it may have crashed or been killed). A plugin that keeps running would answer users twice after openbee restarts.
- Secrets arrive only in `initialize`. They are never passed in arguments or environment variables. The environment is inherited from openbee.
- openbee restarts a plugin that exits unexpectedly. The delay starts at 1s and doubles up to 60s; after 10 minutes of healthy uptime it resets.
- openbee does not restart a plugin that answers `initialize` with error `-32001`, reports a different `protocol_version`, or whose command cannot be found, until its settings change or it is restarted through the API.
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
