# 插件

<sub><a href="plugins.md">English</a></sub>

引擎、聊天渠道和员工头像都是插件。OpenBee 运行时即可安装、配置、启用和卸载它们，无需重启。

## 官方插件

| 类型 | ID | 接入 |
| --- | --- | --- |
| 引擎 | `claude` | Claude Code |
| 引擎 | `codex` | Codex |
| 引擎 | `pi` | pi |
| 引擎 | `opencode` | OpenCode |
| 渠道 | `feishu` | 飞书 / Lark |
| 渠道 | `dingtalk` | 钉钉 |
| 渠道 | `wecom` | 企业微信 |
| 渠道 | `weixin` | 微信 |
| 渠道 | `telegram` | Telegram |
| 渠道 | `discord` | Discord |
| 渠道 | `linear` | Linear |
| 头像 | `mfers` | 300 个 mfer，sartoshi 作品（CC0） |

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

## 头像

头像插件装好即启用。之后新建的员工会随机分配一个还没人用的头像，已有员工保持文字头像。要换头像，编辑员工，选一个、随机一个或改回文字头像即可。停用或卸载插件后，员工改回显示文字头像。

## 运行时

渠道和引擎插件运行在 Bun 或 Node.js 上。两者都没有时，OpenBee 会把 Bun 下载到 `~/.openbee/runtime`。头像插件只有图片，不需要运行时。

## 开发插件

见[渠道插件协议](channel-plugin-protocol.md)、[引擎插件协议](engine-plugin-protocol.md)和[头像插件协议](avatar-plugin-protocol.md)。
