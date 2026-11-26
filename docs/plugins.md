# Plugins

<sub><a href="plugins.zh.md">中文</a></sub>

Engines, chat channels and worker avatars are plugins. A running OpenBee installs, configures, enables and removes them without a restart.

## Official plugins

| Type | ID | Connects |
| --- | --- | --- |
| Engine | `claude` | Claude Code |
| Engine | `codex` | Codex |
| Engine | `pi` | pi |
| Engine | `opencode` | OpenCode |
| Channel | `feishu` | Lark / Feishu |
| Channel | `dingtalk` | DingTalk |
| Channel | `wecom` | WeCom |
| Channel | `weixin` | WeChat |
| Channel | `telegram` | Telegram |
| Channel | `discord` | Discord |
| Channel | `linear` | Linear |
| Avatars | `mfers` | 300 mfers by sartoshi (CC0) |

## From the app

In the desktop app or the web console, open **Settings → Plugins → Marketplace**, install a plugin, then fill in its settings on the plugin page.

## From the command line

```bash
openbee plugin search              # browse the catalog
openbee plugin install claude      # install by ID
openbee plugin configure telegram  # answer the plugin's questions, then enable it
openbee plugin list                # every plugin and whether it is running
```

`enable`, `disable`, `restart`, `update` and `remove` cover the rest. `configure <id> --set key=value` skips the questions.

## Engines

An engine plugin drives an agent CLI, so install the CLI and sign in to it first. A new engine is enabled right away and shows as ready once the CLI is found. If the CLI is not on `PATH`, point to it:

```bash
openbee plugin configure claude --set path=/path/to/claude
```

## Avatars

An avatar plugin is enabled when it is installed. New workers then get a random avatar that no other worker uses yet; existing workers keep their initials. To change a worker's avatar, edit the worker and pick another one, a random one, or initials. Disable or remove the plugin and workers show their initials again.

## Runtime

Channel and engine plugins run on Bun or Node.js. If neither is installed, OpenBee downloads Bun into `~/.openbee/runtime`. Avatar plugins are just images and need no runtime.

## Write your own

See the [channel plugin protocol](channel-plugin-protocol.md), the [engine plugin protocol](engine-plugin-protocol.md) and the [avatar plugin protocol](avatar-plugin-protocol.md).
