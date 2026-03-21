# Telegram 频道接入设计文档

> 版本：v1.0
> 日期：2026-03-20
> 作者：毛毛

---

## 1. 背景与目标

### 背景

OpenBee 目前已支持飞书（Feishu）、钉钉（DingTalk）、企业微信（WeCom）三个消息平台。Telegram 是全球范围内广泛使用的即时通讯工具，具有开放的 Bot API 生态和活跃的开发者社区，接入 Telegram 可以显著扩大 OpenBee 的适用场景。

### 目标

- 以与现有飞书/钉钉/企业微信一致的架构风格，将 Telegram 接入 OpenBee
- 支持私聊（DM）和群组（Group）两种会话场景
- 支持文本、图片、视频、音频、文档等常见消息类型的收发
- 对用户和运维人员透明：通过相同的配置文件格式启用，无需改动其他业务逻辑

---

## 2. 整体架构设计

### 2.1 现有平台架构模式

OpenBee 的平台接入遵循统一的三层接口模式：

```
platform.Platform
├── ID() string                                 // 平台唯一标识
├── Receiver() PlatformReceiverAdapter          // 消息接收适配器
│   └── Start(ctx, dispatch func(InboundMessage)) error
└── Sender() PlatformSenderAdapter              // 消息发送适配器
    └── Send(ctx, OutboundMessage) error
```

各平台实例在 `app.go` 的 `buildPlatforms()` 中根据配置按需创建，消息接收后通过 `dispatch` 回调统一进入消息队列，发送时通过 `sendersByPlatform` map 路由到对应平台。

### 2.2 Telegram 接入架构

Telegram 接入完全遵循上述模式：

```
TelegramPlatform (implements platform.Platform)
├── ID() → "telegram"
├── TelegramReceiver
│   ├── 长轮询 getUpdates（无需 Webhook，适合自部署）
│   ├── 处理 Update → InboundMessage
│   └── 下载媒体文件，保存至 mediaSvc
└── TelegramSender
    ├── 从 Raw JSON 解析 chatID / messageID
    ├── 文本以 HTML 模式发送（避免 MarkdownV2 转义复杂性）
    └── 媒体文件上传并发送对应类型消息
```

### 2.3 整体数据流

```
Telegram Bot API
      │  长轮询 getUpdates
      ▼
TelegramReceiver.Start()
      │  parse Update → InboundMessage
      │  下载媒体（异步）
      │  发送 "typing..." action
      ▼
dispatch(InboundMessage)
      │
      ▼
[OpenBee 消息队列 / bee 处理流程]
      │
      ▼
TelegramSender.Send(OutboundMessage)
      │  解析 Raw → chatID, messageID
      │  格式化文本 / 上传媒体
      ▼
Telegram Bot API (sendMessage / sendPhoto / sendDocument / ...)
```

---

## 3. 核心模块划分与职责

### 3.1 新增文件

#### `internal/platform/telegram/handler.go`

| 类型 | 职责 |
|------|------|
| `TelegramPlatform` | 实现 `platform.Platform`，组合 receiver + sender |
| `TelegramReceiver` | 长轮询接收消息，解析 Update，下载媒体，调用 dispatch |
| `TelegramSender` | 发送文本/媒体回复，从 Raw 字段提取路由信息 |

### 3.2 修改文件

| 文件 | 变更说明 |
|------|---------|
| `internal/config/config.go` | 新增 `TelegramConfig` 结构体；`PlatformsConfig` 加入 `Telegram` 字段；`applyDefaults` 设置 `MaxMediaSize` 默认值（50MB） |
| `internal/config/config.yaml.tmpl` | 新增 `telegram:` 配置段 |
| `internal/app/app.go` | `buildPlatforms` 函数签名加入 `TelegramConfig`，条件创建 `TelegramPlatform` |
| `cmd/openbee/config.go` | `configValues` 加入 Telegram 字段；交互式向导加入 Telegram 选项（Token 输入） |

---

## 4. 关键接口/数据结构设计

### 4.1 配置结构

```go
// internal/config/config.go

type TelegramConfig struct {
    Enabled      bool   `yaml:"enabled"`
    Token        string `yaml:"token"`
    MaxMediaSize int    `yaml:"max_media_size"` // 字节，默认 50MB
}

type PlatformsConfig struct {
    Feishu   FeishuConfig   `yaml:"feishu"`
    DingTalk DingTalkConfig `yaml:"dingtalk"`
    WeCom    WeComConfig    `yaml:"wecom"`
    Telegram TelegramConfig `yaml:"telegram"`  // 新增
}
```

### 4.2 配置文件模板

```yaml
# internal/config/config.yaml.tmpl（新增段）
    telegram:
      enabled: {{.TelegramEnabled}}
      token: "{{.TelegramToken}}"
      # max_media_size: 52428800  # 50MB，默认值
```

### 4.3 InboundMessage 格式

| 字段 | 值示例 | 说明 |
|------|--------|------|
| `Platform` | `"telegram"` | 平台标识 |
| `SenderID` | `"123456789"` | Telegram user ID |
| `SessionKey` | `"telegram:chatID:senderID"` | 群组：chatID 为群组 ID；私聊：chatID == senderID |
| `Content` | `"你好"` / `"[图片: /path/to/file.jpg]"` | 文本或媒体占位符 |
| `RawContent` | 原始文本（含 @tag 等） | 目前与 Content 相同 |
| `Raw` | `{"update_id":..., "message":{...}}` | 完整 Update JSON，用于 Sender 提取回复元数据 |
| `PlatformMessageID` | `"update_id:message_id"` | 用于去重 |
| `MessageTime` | Unix 毫秒 | 取自 `message.date * 1000` |

### 4.4 Raw JSON 结构（Sender 解析用）

Sender 在 `Send()` 中从 `msg.ReplyTo.Raw` 反序列化，提取：
- `message.chat.id` → chatID（发送目标）
- `message.message_id` → messageID（群组 reply 使用）

```go
type telegramRaw struct {
    Message struct {
        Chat      struct{ ID int64 `json:"id"` }  `json:"chat"`
        MessageID int                              `json:"message_id"`
    } `json:"message"`
}
```

### 4.5 消息类型处理矩阵

| Telegram 消息类型 | 接收处理 | 发送处理 |
|------------------|---------|---------|
| `text` | 直接提取 text | `sendMessage`（HTML mode） |
| `photo` | 下载最大分辨率版本 | `sendPhoto` |
| `document` | 下载文件，保留原始文件名 | `sendDocument` |
| `audio` | 下载音频 | `sendAudio` |
| `voice` | 下载语音（`.ogg`）| `sendVoice` |
| `video` | 下载视频 | `sendVideo` |
| `sticker` | 下载贴纸（`.webp`）| — |
| 其他 | 跳过，记录 warn | — |

### 4.6 平台注册接口

```go
// internal/app/app.go

func buildPlatforms(
    fc config.FeishuConfig,
    dc config.DingTalkConfig,
    wc config.WeComConfig,
    tc config.TelegramConfig,  // 新增参数
    mc config.MediaConfig,
) []platform.Platform {
    mediaSvc := media.NewService()
    var result []platform.Platform
    if fc.Enabled { result = append(result, feishu.NewPlatform(fc, mediaSvc)) }
    if dc.Enabled { result = append(result, dingtalk.NewPlatform(dc, mc, mediaSvc)) }
    if wc.Enabled { result = append(result, wecom.NewPlatform(wc, mediaSvc)) }
    if tc.Enabled { result = append(result, telegram.NewPlatform(tc, mediaSvc)) }  // 新增
    return result
}
```

---

## 5. 实现步骤与计划

### Step 1 — 添加 Go 依赖
```bash
go get github.com/go-telegram-bot-api/telegram-bot-api/v5
```
选择理由：最主流的 Go Telegram Bot 库（~6k stars），API 简洁，长轮询/Webhook 均支持，维护活跃。

### Step 2 — 新增 TelegramConfig
修改 `internal/config/config.go`：
- 添加 `TelegramConfig` 结构体
- 将 `Telegram TelegramConfig` 加入 `PlatformsConfig`
- 在 `applyDefaults` 中设置 `MaxMediaSize` 默认值（50MB）

### Step 3 — 实现 Telegram 平台处理器
创建 `internal/platform/telegram/handler.go`，实现：
- `TelegramPlatform` / `TelegramReceiver` / `TelegramSender`
- 长轮询消息循环（`GetUpdatesChan`）
- 各消息类型的接收处理与媒体下载
- `typing` 状态动作发送（接收确认）
- 文本/媒体发送，群组 reply 支持

### Step 4 — 集成到 app.go
修改 `internal/app/app.go`：
- `buildPlatforms` 参数加入 `TelegramConfig`
- 调用处传入 `cfg.Bee.Platforms.Telegram`

### Step 5 — 更新配置模板和向导
- `internal/config/config.yaml.tmpl`：添加 telegram 配置段
- `cmd/openbee/config.go`：
  - `configValues` 加入 `TelegramEnabled`、`TelegramToken`
  - `loadExistingConfig` 映射对应字段
  - 交互向导 MultiSelect 加入 "Telegram" 选项
  - Token 输入提示（password 模式，隐藏输入）

### Step 6 — 构建验证
```bash
go build ./...
go vet ./...
```

---

## 6. 潜在风险与应对方案

### 6.1 长轮询网络稳定性

**风险**：网络抖动导致长轮询中断，机器人停止响应。

**应对**：
- 使用带重试逻辑的轮询循环，遇到网络错误等待后重连（参考钉钉的 `supervisorLoop` 模式）
- 设置合理的 `getUpdates` 超时参数（建议 30s），避免连接长期挂起
- ctx 取消时优雅退出

### 6.2 消息去重

**风险**：重启后 update_id offset 未持久化，导致重复处理旧消息。

**应对**：
- 利用 OpenBee 现有的 `PlatformMessageID` 去重机制（`msgingest` 层已实现）
- `PlatformMessageID` 设为 `"updateID:messageID"` 格式，全局唯一

### 6.3 媒体文件大小限制

**风险**：Telegram 免费 Bot API 下载文件上限为 20MB（Bot API），上传上限为 50MB。

**应对**：
- 接收：`MaxMediaSize` 默认 50MB，超限文件返回占位符并记录 warn
- 发送：超过 50MB 的文件记录错误并返回文本提示
- 配置文件中可自定义 `max_media_size`

### 6.4 文本格式化

**风险**：MarkdownV2 模式特殊字符（`.`、`-`、`(`、`)` 等）需全部转义，漏转义会导致发送失败（400 错误）。

**应对**：
- 使用 HTML parse mode 替代 MarkdownV2，转义规则更简单（仅 `<`, `>`, `&`）
- 利用现有 `platform.SanitizeFileName` 等工具函数

### 6.5 群组消息过滤

**风险**：在群组中，Bot 可能接收到大量无关消息。

**应对**：
- 仅处理 `message.text` 非空 或 有支持媒体类型的消息
- 私聊消息：全部处理
- 群组消息：当前不过滤（与飞书/钉钉行为一致，由 OpenBee 上层处理 @mention 逻辑）

### 6.6 Token 安全

**风险**：Bot Token 泄露会导致 Bot 被劫持。

**应对**：
- 配置向导使用 password 模式（隐藏输入）
- 配置文件权限建议设为 600（文档中说明）
- 不在日志中打印 Token

---

## 7. 目录结构

```
internal/
└── platform/
    ├── interfaces.go          # 现有接口定义（不变）
    ├── feishu/
    ├── dingtalk/
    ├── wecom/
    └── telegram/              # 新增
        └── handler.go         # TelegramPlatform / Receiver / Sender

internal/config/
├── config.go                  # 新增 TelegramConfig
└── config.yaml.tmpl           # 新增 telegram 段

internal/app/
└── app.go                     # buildPlatforms 加入 Telegram

cmd/openbee/
└── config.go                  # 向导加入 Telegram
```

---

## 8. 依赖项

| 包 | 版本 | 用途 |
|----|------|------|
| `github.com/go-telegram-bot-api/telegram-bot-api/v5` | v5.x | Telegram Bot API 客户端 |

其余依赖（`go.uber.org/zap`、`media.Service`、`platform.SanitizeFileName` 等）均复用现有代码。
