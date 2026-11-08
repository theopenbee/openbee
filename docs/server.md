# Server and CLI

<sub><a href="server.zh.md">中文</a></sub>

Run OpenBee as a background service on a server or any always-on machine. You use it from the web console, the [desktop app](desktop.md) as a client, or a chat app through [plugins](plugins.md).

## Install

<details open>
<summary>📦 npm (recommended)</summary>

```bash
npm install -g @theopenbee/cli
```

The binary for your platform is downloaded automatically: Linux, macOS and Windows, amd64 and arm64.

</details>

<details>
<summary>🔧 Install script</summary>

```bash
curl -fsSL https://raw.githubusercontent.com/theopenbee/openbee/main/install.sh | bash
```

</details>

<details>
<summary>🍺 Homebrew (macOS) / Scoop (Windows)</summary>

```bash
brew install theopenbee/tap/openbee
```

```bash
scoop bucket add theopenbee https://github.com/theopenbee/scoop-bucket
scoop install theopenbee/openbee
```

</details>

<details>
<summary>📥 Binary download</summary>

Download the archive for your platform from [GitHub Releases](https://github.com/theopenbee/openbee/releases), extract it, and put the `openbee` executable on your `PATH`.

</details>

## Start

```bash
openbee server -d
```

This starts OpenBee in the background. To also start it at every login, run `openbee service install` instead: it registers the service and starts it right away.

Open <http://localhost:8080> and create the admin account. Then install an engine, see [Plugins](plugins.md).

## Config and data

On first start OpenBee writes `~/.openbee/config.yaml` with defaults and random secrets, and keeps its database at `~/.openbee/data/openbee.db`. Edit the file and run `openbee restart` to change the port or other settings. To use another file, pass `-c /path/to/config.yaml`.

## Everyday commands

| Command | What it does |
| --- | --- |
| `openbee status` / `stop` / `restart` | Check, stop or restart the background server |
| `openbee service install` / `uninstall` | Turn start-at-login on or off |
| `openbee backup` / `restore <file>` | Back up or restore the database, config and state |
| `openbee upgrade` | Upgrade to the latest version |
| `openbee reset-password` | Reset the admin password |
| `openbee plugin …` | Manage engines and channels, see [Plugins](plugins.md) |
