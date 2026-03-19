# 🐝 OpenBee — Digital Employee (OPC) Solution

**OpenBee** is a digital employee solution that runs on your personal computer or server. You can communicate with it via Lark / DingTalk / WeCom to create workers, assign tasks, and much more — let your imagination run wild!

## Installation

**Option 1: npm install (recommended)**

```bash
npm install -g @theopenbee/cli
```

The platform-specific binary is downloaded automatically during installation. Supports Linux / macOS / Windows (amd64 & arm64).

**Option 2: Manual binary download**

Visit the [GitHub Releases](https://github.com/theopenbee/openbee/releases) page, download the archive for your platform, extract it, and place the `openbee` executable in your PATH.

## Quick Start

**Step 1: Generate a config file interactively**

```bash
openbee config
```

The wizard will guide you through:

- Server port / host
- Claude executable path
- MCP API Key (can be randomly generated)
- IM platform(s) to enable (Lark / DingTalk / WeCom) and their credentials
- Advanced options (can be skipped to use defaults)

The config file is written to `config.yaml` in the current directory by default. Use `-o` to specify a custom path:

```bash
openbee config -o /path/to/config.yaml
```

**Step 2: Start the service**

```bash
openbee start
```

**Step 3: Start using**

- Open the Web Console (default [http://localhost:8080](http://localhost:8080)) to manage Workers and view task status
- Send messages directly in any configured IM platform (Lark / DingTalk / WeCom) to interact with OpenBee

## How It Works

OpenBee consists of four core layers:

**1. Platform Integration Layer**
Connects to IM platforms such as Lark, DingTalk, and WeCom to receive user messages in real time and reply with results in the same conversation.

**2. Message Ingestion & Task Scheduling**
Debounces and aggregates incoming messages, then routes them to the appropriate Worker for execution. Scheduled tasks are also supported for automatic, time-based triggering.

**3. Workers (Digital Employees)**
Each Worker is an intelligent agent powered by Claude AI, equipped with persistent memory, tool invocation (MCP), and multi-step task planning. Workers can be created, configured, and assigned tasks to complete independently — just like real employees.

**4. Web Console**
Provides a visual interface for Worker management, task execution history, and real-time logs, making monitoring and debugging straightforward.

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=theopenbee/openbee&type=Date)](https://star-history.com/#theopenbee/openbee&Date)

## Community

- **Bug reports / Feature requests**: [GitHub Issues](https://github.com/theopenbee/openbee/issues)
- **Contributing**: Please read the [Contributing Guide](CONTRIBUTING.md). You must agree to the [Contributor License Agreement (CLA)](CLA.md) before submitting.
