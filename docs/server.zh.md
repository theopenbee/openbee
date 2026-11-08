# 服务端与命令行

<sub><a href="server.md">English</a></sub>

把 OpenBee 作为后台服务跑在服务器或任何常开的机器上。你可以通过 Web 控制台、作为客户端的[桌面版](desktop.zh.md)，或经由[插件](plugins.zh.md)接入的聊天软件使用它。

## 安装

<details open>
<summary>📦 npm（推荐）</summary>

```bash
npm install -g @theopenbee/cli
```

安装时会自动下载对应平台的二进制文件，支持 Linux、macOS 和 Windows（amd64 与 arm64）。

</details>

<details>
<summary>🔧 安装脚本</summary>

```bash
curl -fsSL https://raw.githubusercontent.com/theopenbee/openbee/main/install.sh | bash
```

</details>

<details>
<summary>🍺 Homebrew（macOS）/ Scoop（Windows）</summary>

```bash
brew install theopenbee/tap/openbee
```

```bash
scoop bucket add theopenbee https://github.com/theopenbee/scoop-bucket
scoop install theopenbee/openbee
```

</details>

<details>
<summary>📥 下载二进制</summary>

从 [GitHub Releases](https://github.com/theopenbee/openbee/releases) 下载对应平台的压缩包，解压后把 `openbee` 可执行文件放进 `PATH`。

</details>

## 启动

```bash
openbee server -d
```

这会在后台启动 OpenBee。如果还想开机登录后自动启动，改用 `openbee service install`：它会注册服务并立即启动。

打开 <http://localhost:19876> 并创建管理员账号，然后安装引擎，见[插件](plugins.zh.md)。

## 配置与数据

首次启动时，OpenBee 会生成 `~/.openbee/config.yaml`（默认配置加随机密钥），数据库放在 `~/.openbee/data/openbee.db`。要改端口等设置，编辑这个文件后运行 `openbee restart`。要使用其他配置文件，加上 `-c /path/to/config.yaml`。

## 常用命令

| 命令 | 作用 |
| --- | --- |
| `openbee status` / `stop` / `restart` | 查看、停止或重启后台服务 |
| `openbee service install` / `uninstall` | 开启或关闭登录后自动启动 |
| `openbee backup` / `restore <file>` | 备份或恢复数据库、配置和状态 |
| `openbee upgrade` | 升级到最新版本 |
| `openbee reset-password` | 重置管理员密码 |
| `openbee plugin …` | 管理引擎和渠道，见[插件](plugins.zh.md) |
