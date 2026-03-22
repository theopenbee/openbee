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

**Typing 状态：** 收到消息后立即在 goroutine 中发送 `sendtyping(status=1)`，需先通过 `getconfig` 获取 `typing_ticket`（缓存 24h）。

---

## Sender（出站消息发送）

**WeixinSender.Send() 流程：**

1. 从 `msg.ReplyTo.Raw` 解析出 `toUserID`（原始消息的 `fromUserID`）
2. 从内存 map 获取 `context_token`（key: toUserID）
3. 纯文本 → 直接 `sendmessage`，`item_list` 包含 TextItem
4. 有 MediaPath → 媒体上传后 `sendmessage`

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
- `sync.Map`，key=fromUserID, value=contextToken
- 每条入站消息更新，发送时读取，缺失时 warn 但不阻塞

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

## API 端点参考

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

## 公共请求头

| Header | Value |
|--------|-------|
| `Content-Type` | `application/json` |
| `AuthorizationType` | `ilink_bot_token` |
| `Authorization` | `Bearer {token}` |
| `X-WECHAT-UIN` | 随机 uint32 → string → base64 |
| `SKRouteTag` | `{routeTag}`（可选） |
