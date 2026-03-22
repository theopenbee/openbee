# 微信（WeChat）消息通道集成设计

## 概述

将微信个人消息通道接入 OpenBee，通过 HTTP JSON API + 长轮询 + AES-128-ECB 加密实现消息收发。遵循现有通道架构（Platform/Receiver/Sender 三接口模式），参考 Telegram 长轮询和 WeCom AES 加密的实现经验。

**核心决策：**
- 单账号模式（与现有通道一致）
- 全量媒体支持（文本/图片/视频/文件/语音，含 SILK→WAV 转码）
- 命令行 QR 扫码登录
- Typing 状态支持
- 会话过期静默等待自动恢复

---

## 包结构

**新增目录：** `internal/platform/weixin/`

| 文件 | 职责 |
|------|------|
| `handler.go` | WeixinPlatform + WeixinReceiver + WeixinSender |
| `crypto.go` | AES-128-ECB 加解密 + PKCS7 填充 + key 双格式解析 |
| `api.go` | HTTP API 客户端（所有 spec 端点 + CDN 上传/下载） |

---

## 配置

```go
type WeixinConfig struct {
    Enabled      bool   `yaml:"enabled"`
    Token        string `yaml:"token"`          // Bearer token（QR 扫码获取）
    BaseURL      string `yaml:"base_url"`       // 默认 https://ilinkai.weixin.qq.com
    CDNBaseURL   string `yaml:"cdn_base_url"`   // 默认 https://novac2c.cdn.weixin.qq.com/c2c
    RouteTag     int    `yaml:"route_tag"`      // 可选
    UserID       string `yaml:"user_id"`        // 扫码登录获取的 user_id
    MaxMediaSize int    `yaml:"max_media_size"` // 默认 100MB
}
```

添加到 `PlatformsConfig`，`applyDefaults()` 设置默认 URL 和 MaxMediaSize。

---

## Receiver（入站消息接收）

**连接模式：** 长轮询（与 Telegram 一致）

**WeixinReceiver.Start() 主循环：**

1. `POST ilink/bot/getupdates`（35s 超时）
   - 传入 `get_updates_buf`（sync cursor，首次为空）
   - 响应返回新 cursor，持久化到内存
2. 遍历 `msgs` 数组，逐条处理：
   - 过滤：只处理 `message_type=1`(USER) 且 `message_state=2`(FINISH)
   - 保存 `context_token` 到内存 map（key: `fromUserID`）
   - 提取 `item_list` 内容：
     - TextItem → 直接取 text
     - ImageItem → 下载解密 → `mediaSvc.SaveInbound` → `BuildPlaceholder`
     - VoiceItem → 下载解密 → SILK→WAV 转码(FFmpeg) → SaveInbound
     - FileItem → 下载解密 → SaveInbound
     - VideoItem → 下载解密 → SaveInbound
   - 构建 InboundMessage：
     - `Platform`: "weixin"
     - `SenderID`: fromUserID
     - `SessionKey`: "weixin:{fromUserID}:{fromUserID}"
     - `PlatformMessageID`: "{seq}:{messageID}"
     - `MessageTime`: createTimeMs
     - `Raw`: 原始 WeixinMessage JSON

**媒体下载解密流程：**
1. 从 CDNMedia 提取 `encrypt_query_param`
2. `GET {cdnBaseUrl}/download?encrypted_query_param=...`
3. 解析 AES key（双格式：16 字节原始 or 32 字符 hex）
4. AES-128-ECB 解密 + PKCS7 去填充
5. `mediaSvc.SaveInbound()` 保存明文

**Typing 状态：** 收到消息后立即在 goroutine 中发送 `sendtyping(status=1)`，其中 `ilink_user_id` 使用 bot 自身的 `config.UserID`（非消息发送者 ID）。需先通过 `getconfig` 获取 `typing_ticket`（缓存 24h）。

---

## Sender（出站消息发送）

**WeixinSender.Send() 流程：**

1. 从 `msg.ReplyTo.Raw` 解析 weixinRaw，获取 `toUserID`（原消息 fromUserID）和 `context_token`
2. 纯文本 → 直接 `sendmessage`，`item_list` 包含 TextItem
3. 有 MediaPath → 媒体上传后 `sendmessage`

**媒体上传流程：**
1. 读取文件，检测 MIME 类型
2. 按 MIME 路由确定 media_type：video/*→2, image/*→1, 其他→3
3. 生成 filekey(32字符hex) + aeskey(16字节随机)
4. 计算 rawsize, rawfilemd5, filesize(填充后大小)
5. `POST getuploadurl` → 获得 upload_param
6. AES-128-ECB 加密文件
7. `PUT {cdnBaseUrl}/upload?encrypted_query_param={upload_param}`
8. 响应头 `x-encrypted-param` → downloadEncryptedQueryParam
9. 构建对应 MessageItem（ImageItem/VideoItem/FileItem）
10. `sendmessage`（附带 context_token）

**重试策略：** CDN 上传最多 3 次，仅 5xx 重试，4xx 立即失败。sendMessage 超时 15s。

**Raw 结构（Sender 解析用）：**
```go
type weixinRaw struct {
    FromUserID   string `json:"from_user_id"`
    ToUserID     string `json:"to_user_id"`
    SessionID    string `json:"session_id"`
    ContextToken string `json:"context_token"`
}
```

---

## AES-128-ECB 加解密（crypto.go）

与 WeCom 的 AES-256-CBC 完全不同，需独立实现。

**加密（上传用）：**
```go
func encryptAesEcb(plaintext, key []byte) []byte
// PKCS7 填充到 16 字节对齐 → 逐 16 字节块 AES-ECB 加密
```

**解密（下载用）：**
```go
func decryptAesEcb(ciphertext, key []byte) ([]byte, error)
// 逐 16 字节块 AES-ECB 解密 → 去除 PKCS7 填充
```

**AES Key 双格式解析：**
```go
func parseAesKey(base64Key string) ([]byte, error)
// base64 解码后：16 字节→直接用，32 字节→hex decode 得 16 字节
```

**密文大小计算：**
```go
func aesEcbPaddedSize(plaintextSize int) int {
    return ((plaintextSize + 1) / 16 + 1) * 16
}
```

---

## QR 码登录（CLI 配置流程）

集成到 `cmd/openbee/config.go` 交互式配置向导：

1. 用户选择启用 weixin 平台
2. `GET ilink/bot/get_bot_qrcode?bot_type=3` 获取 QR UUID
3. 使用 `github.com/mdp/qrterminal/v3` 在终端渲染二维码
4. 长轮询 `GET ilink/bot/get_qrcode_status?qrcode={uuid}`（35s 超时，最多 3 次）
5. 展示状态变化：wait → scaned → confirmed
6. confirmed 后写入 config.yaml

注意：QR 码请求无需 Authorization 头；`get_qrcode_status` 需 `iLink-App-ClientVersion: 1` 头。

---

## 错误处理与状态管理

**会话过期（ret=-14）：**
- 日志 warning，设置 pauseUntil = now + 60min
- 轮询主循环检查，到期后自动恢复

**Context Token：**
- 通过 Raw JSON 传递（Receiver 存入 InboundMessage.Raw → Sender 从 ReplyTo.Raw 解析）
- 与现有通道的 Raw 传递模式一致，无需额外内存存储

**Typing Ticket 缓存：**
- 内存缓存，24h TTL
- getconfig 失败时指数退避（2s → 1h 上限）
- typing 发送失败不阻塞消息处理

**日志脱敏：**
- Token 只显示前 6 位，URL 去掉 query 参数

**长轮询连续失败：**
- 计数器跟踪，达 3 次后 backoff 30s，成功后重置

---

## 集成变更

**修改现有文件：**

| 文件 | 变更 |
|------|------|
| `internal/config/config.go` | 添加 WeixinConfig，PlatformsConfig 增加 Weixin 字段，applyDefaults 增加默认值 |
| `internal/config/config.yaml.tmpl` | 增加 weixin 配置模板段 |
| `internal/app/app.go` | buildPlatforms() 增加 weixin 参数和条件创建 |
| `cmd/openbee/config.go` | 配置向导增加 weixin QR 扫码登录流程 |
| `go.mod` / `go.sum` | 新增 github.com/mdp/qrterminal/v3 |

**新增文件：**

| 文件 | 职责 |
|------|------|
| `internal/platform/weixin/handler.go` | WeixinPlatform + WeixinReceiver + WeixinSender |
| `internal/platform/weixin/crypto.go` | AES-128-ECB 加解密 |
| `internal/platform/weixin/api.go` | HTTP API 客户端 |

**不涉及的变更：**
- `platform/interfaces.go`（完全适配现有接口）
- 消息管线（msgingest, task_dispatcher）
- media.Service（直接复用）

---

## API 客户端约定

**公共请求头（所有已认证请求）：**

| Header | Value |
|--------|-------|
| `Content-Type` | `application/json` |
| `AuthorizationType` | `ilink_bot_token` |
| `Authorization` | `Bearer {token}` |
| `X-WECHAT-UIN` | 随机 uint32 → string → base64 |
| `SKRouteTag` | `{routeTag}`（可选，仅配置了 RouteTag 时添加） |

**公共请求体字段：** 所有 POST 请求 body 必须包含 `"base_info": { "channel_version": "1.0.2" }`。

**sendmessage 请求体结构：**
```json
{
  "msg": { /* WeixinMessage，包含 item_list, to_user_id, context_token 等 */ },
  "base_info": { "channel_version": "1.0.2" }
}
```

**getuploadurl 中 aeskey 编码：** `aeskey` 字段使用 32 字符 hex 编码（16 字节 key 的 hex 表示）。注意区别于消息中 CDNMedia 的 `aes_key` 字段使用 base64 编码。

**API 端点参考：**

| 端点 | 用途 | 超时 |
|------|------|------|
| `POST ilink/bot/getupdates` | 长轮询获取消息 | 35s |
| `POST ilink/bot/sendmessage` | 发送消息 | 15s |
| `POST ilink/bot/getuploadurl` | 获取 CDN 上传地址 | 15s |
| `POST ilink/bot/getconfig` | 获取 typing_ticket | 10s |
| `POST ilink/bot/sendtyping` | 发送输入状态 | 10s |
| `GET ilink/bot/get_bot_qrcode` | 获取登录二维码 | 10s |
| `GET ilink/bot/get_qrcode_status` | 轮询二维码状态 | 35s |
| `PUT {cdn}/upload` | CDN 文件上传 | 120s |
| `GET {cdn}/download` | CDN 文件下载 | 120s |

---

## 边界情况与设计决策

**群消息：** v1 不支持群消息。收到 `group_id` 非空的消息时跳过（日志 debug 记录），后续版本可扩展。

**消息去重：** `PlatformMessageID` 设为 `{seq}:{messageID}`，下游 `msgingest.Gateway` 已内置去重逻辑，无需在 weixin 层额外处理。重连后如果 cursor 过期可能收到重复消息，由 Gateway 过滤。

**空/未知 item_list：** `item_list` 为空或 nil 时跳过该消息（不 dispatch）。遇到未知 item type 时日志 warn 并跳过该 item，继续处理同消息中的其他 item。

**Context Token 获取方式：** 统一通过 Raw JSON 获取（Sender 从 `msg.ReplyTo.Raw` 解析），与现有通道模式一致。去掉独立的 `sync.Map` 存储，避免冗余。Raw 中已包含 `context_token` 和 `from_user_id`（即回复目标）。

**SILK→WAV 转码：** 需要 `MediaConfig` 中的 `FFmpegPath`。WeixinReceiver 构造时接收 `config.MediaConfig`。FFmpeg 命令：`ffmpeg -i input.silk -ar 16000 -ac 1 output.wav`。转码失败时仍保存原始 SILK 文件，日志 warn。

**Typing 中的 ilink_user_id：** 使用 `config.WeixinConfig.UserID`（bot 自身的 user_id，扫码登录时获取），不是消息发送者的 ID。

**Sync cursor 持久化：** v1 仅在内存中保存。进程重启后从空 cursor 开始，可能收到已处理的消息，由 Gateway 去重。已知 trade-off，v2 可持久化到磁盘。

**CDN 下载大小限制：** 使用 `io.LimitReader(resp.Body, MaxMediaSize+1)` 流式读取，超过 MaxMediaSize 时跳过并返回 placeholder（与 Telegram 一致）。

**接口合规检查：**
```go
var _ platform.Platform                = (*WeixinPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*WeixinReceiver)(nil)
var _ platform.PlatformSenderAdapter   = (*WeixinSender)(nil)
```

**NewPlatform 签名：**
```go
func NewPlatform(cfg config.WeixinConfig, mc config.MediaConfig, mediaSvc *media.Service) platform.Platform
```
