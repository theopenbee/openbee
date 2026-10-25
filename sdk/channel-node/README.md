# @theopenbee/channel-sdk

Node.js SDK for building openbee channel plugins. It implements the stdio JSON-RPC protocol described in [`docs/channel-plugin-protocol.md`](https://github.com/theopenbee/openbee/blob/main/docs/channel-plugin-protocol.md), so a plugin only has to connect to its chat platform.

Requires Node.js 20 or newer.

## Usage

```js
import { defineChannel, ConfigError } from "@theopenbee/channel-sdk";

defineChannel({
  async start(ctx) {
    if (!ctx.config.token) throw new ConfigError("token is required");
    const client = await connect(ctx.config.token);
    client.on("message", (m) =>
      ctx.dispatch({ chat_id: m.channelId, sender_id: m.authorId, message_id: m.id, text: m.text }),
    );
    return { stop: () => client.close() };
  },
  async send(msg, ctx, signal) {
    await deliver(msg, { signal });
  },
});
```

## What the SDK handles

- Line-delimited JSON-RPC framing on stdin/stdout, request ids, `$/cancelRequest` (exposed as the `AbortSignal` passed to `send`).
- `initialize`: builds the context and calls `start`. Throwing `ConfigError` reports a config problem (`-32001`) and openbee will not restart the plugin; any other error exits the process so openbee restarts it.
- `shutdown` and stdin EOF: calls `stop` and exits. A plugin must never outlive openbee.
- `console.log`, `console.info` and `console.debug` are redirected to stderr so they cannot corrupt the protocol stream; stderr lines end up in the openbee log.
- Uncaught exceptions and unhandled rejections are written to stderr and exit with code 1.

## Context

| Field | Description |
|---|---|
| `channelId` | Channel id from the manifest |
| `config` | Resolved config from `bee.platforms.<id>` (defaults applied, `enabled` and `bot_name` removed) |
| `botName` | The configured bot display name |
| `language` | openbee UI language (`en` or `zh`) |
| `dataDir` | Private writable directory for this plugin |
| `mediaDir` | Directory where inbound attachments must be stored |
| `dispatch(msg)` | Hand an inbound message to openbee |
| `saveMedia(data, ext)` | Write an attachment into `mediaDir` and return its path |
| `log.debug/info/warn/error(message, fields?)` | Structured logs in the openbee log |

## Development

```sh
pnpm install
pnpm build
pnpm test
```
