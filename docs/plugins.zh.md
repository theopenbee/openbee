# 插件

<sub><a href="plugins.md">English</a></sub>

引擎和聊天渠道都是插件。OpenBee 运行时即可安装、配置、启用和卸载它们，无需重启。

## 官方插件

| 类型 | ID | 接入 |
| --- | --- | --- |
| 引擎 | `claude` | Claude Code |
| 引擎 | `codex` | Codex |
| 引擎 | `pi` | pi |
| 渠道 | `feishu` | 飞书 / Lark |
| 渠道 | `dingtalk` | 钉钉 |
| 渠道 | `wecom` | 企业微信 |
| 渠道 | `weixin` | 微信 |
| 渠道 | `telegram` | Telegram |
| 渠道 | `discord` | Discord |
| 渠道 | `linear` | Linear |

## 在应用里管理

在桌面版或 Web 控制台中打开 **设置 → 插件 → 插件市场**，安装插件后在插件页填写配置即可。

## 用命令行管理

```bash
openbee plugin search              # 浏览插件目录
openbee plugin install claude      # 按 ID 安装
openbee plugin configure telegram  # 按提示填写配置，然后启用
openbee plugin list                # 查看所有插件及其运行状态
```

另有 `enable`、`disable`、`restart`、`update` 和 `remove`。`configure <id> --set key=value` 可以跳过问答直接配置。

## 引擎

引擎插件负责驱动 Agent CLI，所以请先安装对应的 CLI 并登录。新装的引擎会立即启用，找到 CLI 后状态显示为 ready。如果 CLI 不在 `PATH` 里，指定它的位置：

```bash
openbee plugin configure claude --set path=/path/to/claude
```

## 运行时

插件运行在 Bun 或 Node.js 上。两者都没有时，OpenBee 会把 Bun 下载到 `~/.openbee/runtime`。

## 开发插件

见[渠道插件协议](channel-plugin-protocol.md)和[引擎插件协议](engine-plugin-protocol.md)。
