# 渠道插件化设计

- 日期：2026-10-24
- 状态：已确认（2026-10-25）；示例插件为 Discord 参考实现
- 范围：只做聊天渠道（platform）插件化；Agent 引擎插件化本期不做

## 1. 背景与目的

openbee 现在的渠道（飞书、钉钉、企微、微信、Telegram、Linear）全部编译进主程序。渠道接口本身已经很小：

- `platform.Platform`：`ID()` / `Receiver()` / `Sender()`
- `PlatformReceiverAdapter.Start(ctx, dispatch)`：收消息，回调 `dispatch(InboundMessage)`
- `PlatformSenderAdapter.Send(ctx, OutboundMessage)`：发消息

问题出在接入成本和耦合上。以 Linear 渠道（PR #142）为例，渠道目录之外还改了 10 个文件：`app.go`、`config.go`、`config.yaml.tmpl`、CLI 配置向导、i18n 三处、`rpc/tools.go`（加了一段 Linear 特判）等。目前散落在渠道包之外的渠道知识有：

| 位置 | 内容 |
|---|---|
| `internal/app/app.go` `buildPlatforms` | 每个渠道一个 `if xx.Enabled` 分支，手动注册上下文提取器 |
| `internal/app/app.go` `msgingest.WithPlatformBotNames` | 手写的渠道 → bot 名映射 |
| `internal/infra/config/config.go` | 每个渠道一个强类型配置结构，`PlatformsConfig.BotNames()` 手写列表 |
| `internal/domain/worker/manager.go` | 通过 `bc.Platforms.BotNames()` 拿 bot 名，防止 worker 与 bot 重名 |
| `internal/rpc/tools.go` `toolSendMessage` | `if platform == linear` 时文字和附件合并成一次发送 |
| `cmd/openbee/internal/cli/configcmd/*` | 每个渠道一个向导函数、一组 `configValues` 字段、模板片段 |
| `internal/infra/i18n` | 向导里每个渠道的提示文案 |

渠道是长尾需求（Slack、Discord、QQ、WhatsApp、Matrix、邮件……），不可能都进主仓库。目的：

1. 让新渠道可以**不改主程序、不发版**就接入，并且可以用任何语言写（多数 IM 的官方 SDK 首选 Node/TS）。
2. 渠道出问题时不拖垮主进程和其他渠道。

参考：magpie 用 Bun 宿主进程 + stdin/stdout 每行一条 JSON 的方式承载重插件；渠道插件需要网络、WebSocket、定时器，同样需要完整运行时，因此采用独立子进程方案。

## 2. 目标、非目标、反目标

### 目标

- **G1（第 0 步）**：主程序运行时对渠道“无知”——装配、bot 名、上下文提取、发送路由都从一份通用的渠道列表推导，不再按渠道名写分支。
- **G2（第 1 步）**：外部渠道插件。插件是一个独立子进程，通过 stdio 上的 JSON-RPC 与主程序通信；对主程序其余部分来说，它和内置渠道没有区别。
- **G3**：插件的安装、列出、移除有 CLI 命令；插件渠道的配置写在 `config.yaml` 里，和内置渠道同一个位置，`openbee config` 向导能根据插件声明的配置项提问。
- **G4**：提供一个 Node/TS SDK，以及一个真实渠道的示例插件，端到端验证协议。
- **G5**：协议有一份独立的参考文档，第三方只看文档就能用任何语言实现插件。

### 非目标（本期不做）

- Agent 引擎插件化。
- 把现有内置渠道迁移成外部插件。它们继续留在 Go 里。
- 热加载。安装或改配置后需要重启 openbee（与现在改配置的行为一致）。
- Web 端的插件管理页面。
- 插件市场、注册表、自动更新。
- 消息中间件、UI 页面/面板之类的其他插件类型。
- 兼容 OpenClaw 的渠道插件（理由见 §9）。
- 沙箱。插件以当前用户权限运行，只安装可信代码。
- 插件的交互式登录流程（例如微信那种扫码登录）。见 §8 的预留。
- 插件调用主程序的业务接口（例如 Telegram 群升级超级群时迁移会话键，内置渠道是直接拿 `TaskStore` 做的）。

### 反目标（技术上能跑但算失败的情况）

- 现有用户的 `config.yaml` 需要手改才能继续用。
- 一个插件崩溃、卡死或配置错误，导致 openbee 启动失败或其他渠道不可用。
- 插件作者被锁死在某一种语言或运行时上。
- 协议暴露了主程序内部结构（DB 行、内部 struct），导致主程序重构时第三方插件被动跟着改。
- 重新运行 `openbee config` 时把用户已经写好的插件配置弄丢。

## 3. 约束

- openbee 是单个 Go 二进制，通过 npm（`@theopenbee/cli`，按平台分包）和 `install.sh` 分发。npm 用户一定有 Node；`install.sh` 用户不一定有。
- 数据目录固定在 `~/.openbee`（随 `HOME` 变化，测试实例靠隔离 `HOME`）；`config.yaml` 路径由 `-c` 指定。
- 需支持 macOS / Linux / Windows（现有 `internal/ai/process_unix.go` / `process_windows.go` 已有进程组处理）。
- 去重集合与 DB 唯一索引 `idx_platform_messages_platform_msg_id` 都是**跨渠道全局**的，只按 `platform_msg_id` 区分。
- 会话键约定为 `<渠道ID>:<chatID>[:<更多>]`。`delivery.Resolver` 在没有历史消息时用 `strings.Cut(sessionKey, ":")` 推断渠道；`platform.SessionChatID` 用 `SplitN(key, ":", 3)` 取 chatID。
- 入站媒体统一落在 `~/.openbee/media/inbound/`，消息正文里用 `media.Service.BuildPlaceholder` 生成的 `<media:image path="..." name="...">` 占位符。
- 项目规范：代码中不写注释（`//go:build` 之类除外）；代码内文本用英文；CHANGELOG 每条不超过 10 个英文单词。

## 4. 已经定下来的

- 先做渠道插件化，Agent 插件化不做。（用户决定）
- 走两步：先做第 0 步纯 Go 重构，再做外部插件。（讨论中提出，用户认可方向）
- 外部插件 = 主程序里的“代理 Platform” + 子进程，stdio JSON-RPC，协议与语言无关。（同上）
- 内置的飞书、钉钉、企微、微信、Telegram 继续内置在 Go 里。（同上）

## 5. 第 0 步：运行时与渠道解耦

目标是把 §1 表格里“运行时”那几处收敛成一份通用列表。**内置渠道的配置结构和 CLI 向导本期保持不变**（理由见 §9 的 D1）。

### 5.1 通用渠道描述

在 `internal/platform` 里引入一个运行时描述（命名由实现者定）：

```go
type Channel struct {
	Platform       Platform
	BotName        string
	ExtractContext func(raw string) string
}
```

`app.go` 里的构建逻辑改成：内置渠道和插件渠道都产出 `[]Channel`，之后只有一个通用循环，从这份列表推导出：

- `sendersByPlatform`（仍然包一层 `store.NewLoggingPlatformSenderAdapter`）
- receiver 的 runner
- `msgingest.WithPlatformBotNames` 的映射
- `platform.RegisterExtractor` 的注册
- worker manager 需要的 bot 名列表（`worker.NewManager` 改为接收 bot 名列表，不再自己读 `bc.Platforms.BotNames()`）

注意保持现有行为：今天 `PlatformsConfig.BotNames()` 和 `WithPlatformBotNames` 的映射**不看 `enabled`**，未启用渠道的 bot 名也会生效（worker 不能和它重名，@它也会被剥掉）。bot 名列表因此要从“配置里出现的所有渠道段落”收集，而不是只从启用的渠道收集；插件渠道同理。

`local` 渠道保持现状，单独处理。

### 5.2 去掉 Linear 特判

`rpc/tools.go` 里“文字 + 附件合并为一次发送”的特判改成一个可选接口，由 sender 自己声明，例如：

```go
type CombinedSender interface {
	SendsTextWithMedia() bool
}
```

`toolSendMessage` 检查 sender 是否实现该接口：是则一次发送，否则保持现在“先文字、后附件”两次发送。Linear sender 和插件代理 sender 都实现它。注意 sender 外面包了一层 logging adapter，接口要能穿透这层（实现者决定是让 adapter 透传，还是在 `Channel` 上加字段）。

### 5.3 第 0 步完成的标准

- `app.go`、`msgingest`、`worker`、`rpc` 里不再出现具体渠道 ID 或渠道包的分支逻辑（`buildPlatforms` 里构造内置渠道的那张表除外）。
- 行为零变化：现有测试全过，E2E 冒烟（至少一个真实 IM 渠道收发 + 主动推送 + 附件）通过。

## 6. 第 1 步：外部渠道插件

### 6.1 插件包与清单

一个插件 = 一个目录，根目录放清单文件 `openbee.plugin.json`。v1 里**一个插件只提供一个渠道**。

```json
{
  "protocol_version": 1,
  "id": "discord",
  "name": { "en": "Discord", "zh": "Discord" },
  "description": { "en": "Discord bot channel", "zh": "Discord 机器人渠道" },
  "command": ["node", "dist/index.js"],
  "config": [
    {
      "key": "token",
      "type": "secret",
      "required": true,
      "label": { "en": "Bot token", "zh": "机器人 Token" },
      "help": { "en": "Discord Developer Portal → Bot → Token", "zh": "Discord 开发者后台 → Bot → Token" }
    },
    { "key": "allowed_guilds", "type": "list", "label": { "en": "Allowed server IDs", "zh": "允许的服务器 ID" } },
    { "key": "max_media_size", "type": "int", "default": 26214400, "label": { "en": "Max media size (bytes)", "zh": "附件大小上限（字节）" } }
  ]
}
```

- `id`：渠道 ID，也是会话键前缀和 `config.yaml` 里的键。规则 `^[a-z][a-z0-9_-]{1,31}$`；不能和内置渠道（`feishu`、`dingtalk`、`wecom`、`telegram`、`weixin`、`linear`、`local`）重名。
- `command`：启动命令，工作目录为插件根目录。第一个元素按 PATH 查找（如 `node`）或是相对插件根目录的可执行文件（便于 Go/Rust 编译产物）。
- `config`：配置项声明。`type` 取 `string` / `secret` / `bool` / `int` / `list`（字符串列表）。`label` / `help` 可以是字符串，也可以是 `{en, zh}`，按当前语言取，缺省回退到 `en`，再回退到 `key`。
- `enabled`、`bot_name` 是主程序保留的配置键，插件不能声明。
- 版本号：有 `package.json` 时取其 `version`，否则取清单里的 `version` 字段。

### 6.2 安装与目录

```
~/.openbee/plugins/
  package.json          # 私有 npm 项目，首次 npm 安装时创建
  node_modules/...      # npm 安装的插件
  plugins.json          # 已安装插件登记表：id → { source, path, version }
  data/<id>/            # 每个插件私有的可写目录
```

- **npm 来源**：`openbee plugin install <spec>`，`spec` 是任何 `npm install` 认识的写法（包名、`@scope/name@1.2.3`、`github:owner/repo`、tarball）。主程序在 `~/.openbee/plugins` 里执行 `npm install <spec>`，再到对应包目录读清单。npm 不在 PATH 时给出明确报错。
- **本地目录**：参数是已存在的目录时，就地登记（不复制），每次启动都按目录当前内容加载，方便开发。
- 安装时校验：清单存在且合法、`protocol_version` 受支持、`id` 合法且不和内置渠道冲突、不和已装的其他插件冲突（同一个包重装视为升级）。
- 安装成功后打印：信任提示（插件以当前用户权限运行，只安装可信代码）、下一步提示（运行 `openbee config` 或编辑 `config.yaml`，然后重启）。

### 6.3 CLI

- `openbee plugin install <spec|path>`：安装或升级。
- `openbee plugin list`：列出 id、名称、版本、来源、路径，以及在当前 `config.yaml` 里是否启用。
- `openbee plugin remove <id>`：卸载并移出登记表；不改 `config.yaml`，如果仍处于启用状态则给出警告。

文案走 i18n（中英文）。

### 6.4 配置

插件渠道的配置和内置渠道放在同一层：

```yaml
bee:
  platforms:
    feishu:            # 内置渠道，格式不变
      enabled: true
      app_id: ...
    discord:           # 插件渠道
      enabled: true
      bot_name: OpenBee
      token: "..."
      allowed_guilds: ["123"]
```

- `PlatformsConfig` 保留现有强类型字段，再加一个 inline map 收集其余键（yaml.v3 的 `,inline` map），存放插件渠道配置。现有配置文件无需改动。
- 启动时，对每个 `enabled: true` 的插件配置：找已安装插件 → 套用清单默认值 → 校验必填项和类型 → 去掉 `enabled` / `bot_name` 后作为 `config` 传给插件。
- 找不到插件、校验失败：打 error 日志，**跳过该渠道，其余照常启动**。
- `openbee config` 向导：
  - 渠道多选列表里加入已安装的插件（显示清单里的名称）。
  - 选中的插件按清单 `config` 逐项提问：`secret` 用密码输入，`bool` 用确认，`int` 校验整数，`list` 用逗号分隔输入；最后复用现有的 bot 名提问。
  - 模板渲染插件段落时要**保留**未安装插件的已有配置，原样写回，避免丢数据。

### 6.5 进程模型与监管

- openbee 启动时，为每个启用的插件渠道拉起一个子进程。子进程放进独立进程组（复用 `ai.ConfigureCmd` 的做法，或者把它抽到公共包），退出时整组清理。
- **插件必须在 stdin 关闭（EOF）时退出。** 这是协议的硬性要求，SDK 默认实现。openbee 被 SIGKILL 或崩溃时不会走优雅关闭，管道关闭是唯一可靠的信号；否则孤儿插件会继续连着同一个 bot，下次启动后用户收到双份回复。进程组只用于正常停止时清理插件派生的子进程。Windows 上 `ai.ConfigureCmd` 是空实现，停止靠 `taskkill /T`；是否额外用 Job Object 由实现者定。
- 环境变量继承主进程（便于代理设置）；**配置和密钥只通过 `initialize` 传递**，不放在环境变量或命令行参数里，避免出现在 `ps` 里。
- stdout 只走协议；stderr 每一行写入 openbee 日志，`component` 为 `plugin:<id>`。
- 崩溃重启：指数退避，1s 起、上限 60s；连续稳定运行 10 分钟后退避清零。
- 以下情况**不重启**，只记 error 日志：`initialize` 返回配置错误、协议版本不匹配、启动命令不存在。
- 插件不在运行时，`send` 立即返回错误（`channel <id> is not running`），不排队。
- 关闭：主进程退出时发 `shutdown`，最多等 5s，然后关闭 stdin 并杀掉整个进程组。现在的 `App.Run` 取消 context 后只等 HTTP 关闭（最多 15s）就关 DB 返回，**不等 runner**；需要给插件监管加等待（例如 WaitGroup），与 HTTP 关闭并行进行，整体落在 daemon 停止时 SIGTERM 到 SIGKILL 的 15s 窗口内。

### 6.6 协议

传输：stdio，UTF-8，**每行一条 JSON-RPC 2.0 消息**（NDJSON，与 MCP 的 stdio 传输相同）。双方都可以发请求和通知。单行上限 8 MiB；超限或无法解析的行记 warn 日志后丢弃。未知字段双方都忽略，以便在 v1 内向后兼容地加字段。

#### 主程序 → 插件

**`initialize`**（请求，超时 30s）

```json
{
  "protocol_version": 1,
  "openbee_version": "0.0.46",
  "channel_id": "discord",
  "language": "zh",
  "bot_name": "OpenBee",
  "config": { "token": "...", "allowed_guilds": ["123"], "max_media_size": 26214400 },
  "data_dir": "/home/u/.openbee/plugins/data/discord",
  "media_dir": "/home/u/.openbee/media/inbound"
}
```

返回 `{ "protocol_version": 1 }`。插件返回成功后开始连接平台并收消息。配置有问题时返回错误码 `-32001`（config invalid），主程序不会重启它。

**`send`**（请求）

超时：跟随调用方的 context，主程序侧再加 120s 上限。实际情况是 bee 和 worker 的回复都经过 `openbee ctl message send`，它的 HTTP 客户端超时 30s（`internal/ctlclient/client.go`），RPC 处理函数把请求 context 传给了 `Send`，所以这条路径上实际只有约 30s。调用方放弃时，主程序向插件发 `$/cancelRequest` 通知（`{"id": <请求 id>}`，同 LSP），SDK 把它转成 `AbortSignal` 交给插件，尽力中止。已经发出去的消息无法撤回，调用方重试可能导致重复，这一点与内置渠道今天的行为一致，本期不处理。

```json
{
  "session_key": "discord:123:456",
  "chat_id": "123",
  "session_scope": "456",
  "proactive": false,
  "reply_to": { "message_id": "m1", "raw": "<插件入站时给的 raw，原样带回>" },
  "content": "文字内容",
  "media": { "path": "/abs/path/report.pdf", "name": "report.pdf" }
}
```

- `content` 和 `media` 至少有一个。两者都有时，插件自己决定合并发送还是先文字后附件（SDK 默认先文字后附件）。
- `chat_id` 和 `session_scope` 由主程序从会话键拆出，插件不需要自己解析会话键（Telegram 论坛话题那种“chat + thread + sender”的结构可以编码进 `session_scope`）。
- `proactive: true` 时没有可用的 `reply_to.raw`（或者只有该会话最近一条的 raw），插件按 `chat_id` 主动发送。
- `reply_to.message_id` 是插件入站时给的 `message_id`（不带渠道前缀）。回复路径上 `delivery.Resolver` 只从 DB 拿得到 platform、session_key、raw 三个字段，所以 `message_id` 也放在 raw 信封里（见下文）。
- 返回 `{}`，可选带 `platform_message_id`。失败返回 JSON-RPC 错误，错误信息会原样回到调用方（例如 worker 的 `openbee ctl message send`）。

**`shutdown`**（请求，超时 5s）：插件断开平台连接，返回后可以退出。

#### 插件 → 主程序

**`inbound`**（通知）

```json
{
  "chat_id": "123",
  "sender_id": "456",
  "session_scope": "456",
  "message_id": "m1",
  "text": "@OpenBee 帮我看看这个",
  "attachments": [ { "type": "image", "path": "/home/u/.openbee/media/inbound/x.png", "name": "x.png" } ],
  "raw": "<任意字符串，插件自己用来回复>",
  "context": { "guild_id": "9", "channel_name": "general" },
  "time": 1761300000000
}
```

主程序收到后转成 `platform.InboundMessage`，交给 `msgingest` 的 `Dispatch`：

| 字段 | 规则 |
|---|---|
| `chat_id` | 必填，不能含 `:` |
| `sender_id` | 必填 |
| `session_scope` | 可选，默认等于 `sender_id`；会话键 = `<id>:<chat_id>:<session_scope>`，与内置渠道的 `chat:user` 习惯一致，也能表达 Linear 那种“按 issue 一个会话” |
| `message_id` | 可选，用于去重；主程序存成 `<id>:<message_id>`，避免撞上全局唯一索引 |
| `text` / `attachments` | 至少一个非空 |
| `attachments[].path` | 必须在 `media_dir` 内，否则丢弃该附件路径（保留不带路径的占位符）并记 warn。`type` 取 `image` / `audio` / `video` / `document` / `sticker` |
| `raw` | 可选，不透明字符串，`send` 时原样回传 |
| `context` | 可选 JSON 对象，注入 worker 任务的 `platform_context` |
| `time` | 可选，Unix 毫秒 |

正文 `Content` 由主程序拼装：`text` 后面接每个附件的 `BuildPlaceholder` 占位符。插件不需要知道 `<media:...>` 语法。`RawContent` = `text`。

**`log`**（通知，可选）：`{ "level": "debug|info|warn|error", "message": "...", "fields": {...} }`，写进 openbee 日志。

#### raw 与 context 的存放

DB 只存一列 `Raw`。代理 Platform 存的是一个信封：`{"v":1,"raw":"<插件 raw>","message_id":"m1","context":{...}}`。

- 插件渠道注册的上下文提取器从信封里取 `context`，用 `platform.MarshalContext(id, context)` 输出。
- 发送时代理把信封拆开，把插件自己的 `raw` 和 `message_id` 放进 `send.reply_to`。

这样不需要改 DB 表结构，插件也看不到主程序的存储格式。

### 6.7 代码位置（建议）

- `internal/plugin/`：清单解析与校验、`plugins.json` 登记表、npm 安装/卸载（CLI 和 app 共用）。
- `internal/platform/pluginchannel/`：代理 Platform（实现 `Platform` / receiver / sender / `CombinedSender`）、JSON-RPC 连接、子进程监管。
- `cmd/openbee/internal/cli/plugincmd/`：`openbee plugin ...` 子命令。
- `sdk/channel-node/`：Node/TS SDK。
- `plugins/<name>/`：示例插件。
- `docs/channel-plugin-protocol.md`：协议参考（英文，面向第三方开发者）。

## 7. SDK 与示例插件

### 7.1 Node/TS SDK（`@theopenbee/channel-sdk`）

职责：

- NDJSON 编解码、JSON-RPC 请求/响应关联、超时。
- 生命周期：收到 `initialize` 时调用插件的 `start(ctx)`，`start` 正常返回后才回复 `initialize`；`start` 抛出 `ConfigError` 时回复 `-32001`。收到 `shutdown` 时调用 `stop()`。
- **保护 stdout**：把 `console.log` / `console.info` 等重定向到 stderr，防止插件或其依赖的输出污染协议流。
- 未捕获异常：写 stderr 后 `exit(1)`，交给主程序重启。
- 帮手函数：`ctx.dispatch(inbound)`、`ctx.saveMedia(buffer, ext)`（写进 `media_dir`，返回路径）、`ctx.log`、`ctx.config`、`ctx.botName`、`ctx.dataDir`。

接口草图：

```ts
import { defineChannel, ConfigError } from "@theopenbee/channel-sdk";

defineChannel({
  async start(ctx) {
    if (!ctx.config.token) throw new ConfigError("token is required");
    const client = await connect(ctx.config.token);
    client.on("message", (m) => ctx.dispatch({ chat_id: m.channelId, sender_id: m.authorId, text: m.content, raw: m.id }));
    return { stop: () => client.destroy() };
  },
  async send(msg, ctx) {
    await deliver(msg);
  },
});
```

最低 Node 版本由实现者定（建议 Node 20 LTS）。npm 发布属于对外操作，发布前需要单独确认；开发期示例插件可以用本地路径引用 SDK。

### 7.2 示例插件

用 SDK 实现一个真实渠道，作为协议的端到端验证和第三方的参考实现。**具体选哪个渠道待定（见 §10）**。我的默认建议是 Discord：只要一个 bot token，走 Gateway WebSocket，不需要公网回调地址（openbee 常跑在本机或内网），`discord.js` 成熟。

## 8. 为以后预留（本期不实现）

- `setup` 方法：插件驱动的交互式配置（扫码登录、OAuth），由 CLI 向导转发提示。
- `capabilities`：`initialize` 返回值里预留，v1 不定义任何能力位。
- `ping` 心跳：v1 靠进程退出和 `send` 超时发现问题。
- Web 端插件管理与运行状态展示。

## 9. 我做的决定（请重点看）

- **D1 内置渠道的配置和 CLI 向导这期不动。** 之前讨论里提过“每个渠道自己声明配置 schema，取代 config.go 强类型结构和向导硬编码”。仔细看过后改主意了：有了插件之后，新渠道基本都走插件，内置渠道很少再加；而向导里有微信扫码、Telegram 随机授权码这类定制流程，重写成 schema 驱动收益小、风险大。第 0 步只收敛运行时部分（§5）。
- **D2 插件配置写在 `config.yaml` 的 `bee.platforms.<id>` 下**，不另起 `plugins.json` 存配置。用户眼里插件渠道和内置渠道是同一类东西。代价是向导模板必须正确保留插件段落（§6.4）。
- **D3 插件启动失败只跳过该渠道**，不阻止 openbee 启动。
- **D4 会话键由主程序拼装**（`<id>:<chat_id>:<session_scope>`），插件不能自己给完整会话键，避免格式写错导致路由失败。
- **D5 消息去重 ID 自动加渠道前缀**，避免撞上全局唯一索引。
- **D6 不兼容 OpenClaw 渠道插件。** OpenClaw 的渠道插件是进程内 TS 插件，依赖 `openclaw/plugin-sdk/*` 下十几个子模块和二十多个适配面（config、setup、security、pairing、outbound、message、gateway、status……），而且还在快速演进。做兼容层等于长期追着它的版本跑。我们的协议足够小，把 OpenClaw 插件里的平台对接代码移植过来很容易。
- **D7 只有 npm 和本地目录两种安装来源。** git 仓库通过 npm 的 `github:owner/repo` 写法就能装，不单独实现。安装时不禁用 npm 的 install 脚本：插件本身就是要以用户权限执行的可信代码，禁用脚本防不住什么，反而会让带原生依赖的包装不上。
- **D8 发送时插件不在线就直接报错**，不做队列和重试。
- **D9 不改 `openbee ctl` 的 30s 超时。** 插件 `send` 跟随调用方超时，并支持取消通知（§6.6）。大附件上传超过 30s 的问题内置渠道今天也有，单独处理更合适。
- **D10 第 0 步保持“未启用渠道的 bot 名也生效”的现有行为**（§5.1），保证真正的零行为变化。

## 10. 需要你回答的问题

1. **示例插件做哪个渠道？它只是参考实现，还是要做成正式支持、发布到 npm 给用户装的插件？端到端测试用的真实账号谁来提供？**
   - 渠道决定工作形态：走 WebSocket 的（Discord、Slack Socket Mode）不需要公网地址；走 webhook 的需要内网穿透或公网入口才能测。
   - 定位决定质量要求：正式插件要做重连、限流、Markdown 转换，还涉及 npm 发布；参考实现可以很薄，也不需要对外发布。
   - 我的默认建议：Discord，先做参考实现，测试账号我可以按步骤说明你来建（需要在开发者后台开 Message Content intent）。

## 11. 测试与验收

- 第 0 步：现有单测全过；`app` 装配相关补测试；按 E2E 验收流程在隔离实例上用一个真实 IM 渠道做收发、主动推送、附件冒烟。
- 代理 Platform：用 Go 写一个假插件（测试 helper 子进程，不依赖 Node），覆盖：握手、配置错误不重启、协议版本不匹配、`inbound` 字段校验与会话键拼装、附件路径越界、raw 信封往返、`send` 超时、插件崩溃后退避重启、关闭时进程组清理、stdout 脏数据。
- SDK：`node:test` 单测覆盖编解码、生命周期、stdout 重定向。
- 进程生命周期：openbee 被 SIGKILL 后插件随 stdin EOF 退出，不留孤儿。
- 端到端：隔离实例上 `openbee plugin install <示例插件路径>` → `openbee config` 配置 → 重启 → 在真实渠道里完成收消息、bee 回复、worker 通过 `openbee ctl message send` 回复和发附件、`/clear` 等命令。

## 12. 交付顺序

1. 第 0 步重构（一个独立 PR，行为零变化）。
2. 插件核心：清单/登记表、代理 Platform、协议、监管、配置读取（含 Go 假插件测试）。
3. CLI：`openbee plugin` 子命令 + `openbee config` 向导集成。
4. SDK + 示例插件 + 协议文档 + CHANGELOG。

每一步单独可合并。

## 13. 留给实现者的决定

- §5.1 结构体的具体命名和放的位置；`CombinedSender` 怎么穿透 logging adapter。
- JSON-RPC 实现：手写（推荐，体量小）还是引入库。
- 进程组工具函数是直接复用 `ai.ConfigureCmd` 还是抽到 `internal/infra` 下的公共包。
- 退避、超时的具体常量可以微调，但要集中定义。
- 向导里插件配置项的提问顺序与校验细节。
- 示例插件里平台侧的实现细节（重连、限流、Markdown 转换等）。
