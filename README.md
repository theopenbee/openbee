<img src="docs/images/logo-full.svg" alt="OpenBee" width="220" />

# OpenBee — Build smarter AI teams.

**OpenBee** is an around-the-clock digital worker solution, dedicated to making AI Agents your 7×24 always-on assistant.

To learn more about the project, visit [docs.theopenbee.com](https://docs.theopenbee.com).

[![npm version](https://img.shields.io/npm/v/@theopenbee/cli?color=F7C948&logo=npm&label=npm)](https://www.npmjs.com/package/@theopenbee/cli)
[![npm downloads](https://img.shields.io/npm/dt/@theopenbee/cli?color=F7C948&logo=npm&label=downloads)](https://www.npmjs.com/package/@theopenbee/cli)
[![License](https://img.shields.io/github/license/theopenbee/openbee?color=F7C948)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-F7C948)](https://github.com/theopenbee/openbee/releases)
[![GitHub Stars](https://img.shields.io/github/stars/theopenbee/openbee?style=social)](https://github.com/theopenbee/openbee)

<sub><a href="README.zh.md">中文</a> &nbsp;·&nbsp; <a href="https://docs.theopenbee.com">📖 Docs</a> &nbsp;·&nbsp; <a href="https://x.com/0xtyz">0xtyz</a></sub>

## 📸 Screenshots

<img src="docs/images/openbee-dashboard.png" alt="OpenBee Dashboard" />

## 🚀 Quick Start

### Step 1: Install

<details open>
<summary>📦 npm (Recommended)</summary>

```bash
npm install -g @theopenbee/cli
```

The platform-specific binary is downloaded automatically. Supports Linux / macOS / Windows (amd64 & arm64).

</details>

<details>
<summary>🔧 One-click Script</summary>

```bash
curl -fsSL https://raw.githubusercontent.com/theopenbee/openbee/main/install.sh | bash
```

</details>

<details>
<summary>🍺 brew (macOS) / scoop (Windows)</summary>

**macOS (Homebrew):**
```bash
brew install theopenbee/tap/openbee
```

**Windows (Scoop):**
```bash
scoop bucket add theopenbee https://github.com/theopenbee/scoop-bucket
scoop install theopenbee/openbee
```

</details>

<details>
<summary>📥 Manual Binary Download</summary>

Visit [GitHub Releases](https://github.com/theopenbee/openbee/releases), download the archive for your platform, extract it, and place the `openbee` executable in your PATH.

</details>

### Step 2: Generate a config file

```bash
openbee config
```

The wizard will guide you through:
- Agent executable path
- Advanced options (can be skipped to use defaults)

The config file is written to `config.yaml` in the current directory by default. Use `-o` to specify a custom path:

```bash
openbee config -o /path/to/config.yaml
```

### Step 3: Start the service

```bash
openbee server -d
```

### Step 4: Start using

- Open the Web Console (default [http://localhost:8080](http://localhost:8080)) to manage Workers and view task status
- Send messages directly in any configured platform (a channel plugin such as WeChat, Lark, DingTalk, WeCom, Telegram or Linear) to interact with OpenBee

### Channel plugins

Chat platforms are added as plugins without rebuilding OpenBee, and a running OpenBee installs, configures, enables and removes them without restarting:

```bash
openbee plugin search              # browse the plugin catalog and pick plugins to install
openbee plugin configure telegram  # answer the plugin's questions, then enable it
openbee plugin list                # see every plugin and whether it is running
```

`openbee plugin install <id>`, `enable`, `disable`, `restart`, `update` and `remove` cover the rest; `configure --set key=value` works without prompts. Plugins run on Bun or Node.js; if neither is installed, OpenBee downloads Bun into `~/.openbee/runtime`. See the [channel plugin protocol](docs/channel-plugin-protocol.md).

## 🖥️ Desktop app

A native desktop client for macOS, Windows and Linux, built with [MyGo](https://github.com/egoist/mygo). It connects to an OpenBee server you already run.

1. Download the package for your platform from [GitHub Releases](https://github.com/theopenbee/openbee/releases): `openbee-desktop-<version>-darwin-universal.dmg` (or `.zip`) for macOS, `openbee-desktop-<version>-windows-<arch>-setup.exe` for Windows, `openbee-desktop-<version>-linux-<arch>.deb` (or `.tar.gz`) for Linux.
2. Open it and enter your OpenBee server address (default `http://localhost:8080`).
3. Sign in with your OpenBee account.

Release builds are not code signed yet, so the first launch needs a manual approval:

- **macOS:** right-click the app and choose Open (on macOS 15 and later, use Open Anyway in System Settings > Privacy & Security), or run `xattr -dr com.apple.quarantine /Applications/OpenBee.app`.
- **Windows:** when SmartScreen shows "Windows protected your PC", click More info, then Run anyway.

To build it from source (Go 1.27 is downloaded automatically):

```bash
make desktop       # current platform, output in desktop/build/
make desktop-all   # macOS, Windows and Linux
```

## 🌟 Star History

<div align="center">

[![Star History Chart](https://api.star-history.com/svg?repos=theopenbee/openbee&type=Date)](https://star-history.com/#theopenbee/openbee&Date)

</div>

## 🤝 Community

- 🐛 **Bug reports / Feature requests** → [GitHub Issues](https://github.com/theopenbee/openbee/issues)
- 🤝 **Contributing** → Please read the [Contributing Guide](CONTRIBUTING.md). You must agree to the [Contributor License Agreement (CLA)](CLA.md) before submitting.
