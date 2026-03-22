# WeChat (微信智能体) Platform Integration Design

## Overview

Integrate WeChat personal intelligent agent (ilinkai.weixin.qq.com) as a new platform in OpenBee, enabling the AI assistant to receive and reply to messages via personal WeChat. Implementation follows existing platform adapter patterns (Feishu, Telegram, WeCom, DingTalk) using Go native code, referencing the `@tencent-weixin/openclaw-weixin` SDK for protocol details.

## Core Decisions

| Decision | Choice |
|----------|--------|
| Tech stack | Go native rewrite (no Node.js dependency) |
| Connection method | Long-polling (faithful to SDK's approach) |
| Message types | Full: text, image, voice (with STT), video, file |
| Authentication | Interactive QR scan in `openbee config` subcommand |
| Media encryption | Self-contained AES-128-ECB in platform package |
| Session management | 1-hour pause on errcode -14 (native behavior) |

## Package Structure

```
internal/platform/weixin/
├── handler.go      # WeixinPlatform, WeixinReceiver, WeixinSender
├── api.go          # WeChat API client (HTTP POST + JSON)
├── types.go        # Protocol type definitions
├── cdn.go          # CDN upload/download + AES-128-ECB encryption
├── auth.go         # QR code login flow (used by config command)
├── session.go      # Session pause management (errcode -14)
```

## Interface Implementation

Implements the standard `Platform` interface triple:

```go
type WeixinPlatform struct {
    cfg      config.WeixinConfig
    media    *media.Service
    receiver *WeixinReceiver
    sender   *WeixinSender
    client   *WeixinAPIClient
}

func (p *WeixinPlatform) ID() string                                  // returns "weixin"
func (p *WeixinPlatform) Receiver() platform.PlatformReceiverAdapter
func (p *WeixinPlatform) Sender() platform.PlatformSenderAdapter
```

**Compile-time interface assertions:**

```go
var _ platform.Platform = (*WeixinPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*WeixinReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*WeixinSender)(nil)
```

**Session Key Format:** `"weixin:{from_user_id}:{from_user_id}"` (3-segment format consistent with all other platforms; WeChat personal agent is always 1:1, so chat scope and sender are the same)

## API Client

```go
type WeixinAPIClient struct {
    baseUrl    string        // https://ilinkai.weixin.qq.com
    cdnBaseUrl string        // https://novac2c.cdn.weixin.qq.com/c2c
    token      string        // bot_token from QR login
    httpClient *http.Client
}
```

### Endpoints

| Method | Endpoint | Purpose | Timeout |
|--------|----------|---------|---------|
| `GetUpdates(syncBuf)` | `/ilink/bot/get_updates` | Long-poll for new messages | 35s |
| `SendMessage(msg)` | `/ilink/bot/send_message` | Send message | 15s |
| `SendTyping(userId, ticket, status)` | `/ilink/bot/send_typing` | Typing indicator | 15s |
| `GetConfig(userId, contextToken)` | `/ilink/bot/get_config` | Get typing ticket etc. | 10s |
| `GetUploadUrl(req)` | `/ilink/bot/get_upload_url` | Get CDN upload credentials | 15s |
| `GetBotQRCode(botType)` | `/ilink/bot/get_bot_qrcode` | Get login QR code | 15s |
| `GetQRCodeStatus(qrcode)` | `/ilink/bot/get_qrcode_status` | Poll QR scan status | 35s |

**Common request pattern:**
- HTTP POST + JSON body
- Headers: `Authorization: Bearer {token}`, `X-WECHAT-UIN: {random base64 of 4 bytes (uint32)}` — random per request, as per SDK `randomWechatUin()` in `src/api/api.ts`
- Response: check `ret` and `errcode` fields
- All requests use `http.NewRequestWithContext` for graceful shutdown cancellation

## Authentication & Token Management

### Login Flow (integrated into `openbee config`)

In Step 4 (Platform Configuration), "微信" is added to the multi-select list. When selected:

1. **Check existing config** for `platforms.weixin.token`
   - Token exists → prompt: "Skip (use existing)" or "Re-login"
   - No token → proceed to QR scan
2. **QR scan flow:**
   - Call `GetBotQRCode(botType="3")`
   - Render QR code in terminal using `github.com/mdp/qrterminal` (or similar Go library)
   - Long-poll `GetQRCodeStatus` until scanned (35s timeout per attempt, loop up to 5 minutes total)
   - User can Ctrl+C to cancel (handled via `handleSurveyErr` pattern)
   - On success: receive `bot_token`, `ilink_bot_id`, `base_url`
   - On timeout: display error message, allow retry or skip
3. **Write to config.yaml** under `platforms.weixin`

**Note:** The token from QR login cannot be regenerated from a developer portal. Users should back up `config.yaml` after successful login.

### Platform startup

Reads token from config directly. No auto-login at startup. If token is invalid, logs warning suggesting `openbee config` re-login.

### Session Pause (session.go)

- errcode -14 → record pause timestamp, skip polling for 1 hour
- Log remaining pause time periodically
- Auto-resume after 1 hour
- Persistent failures → log suggestion to re-run `openbee config`

## Configuration

### Config Structure

```go
type WeixinConfig struct {
    Enabled      bool   `yaml:"enabled"`
    Token        string `yaml:"token"`
    BaseURL      string `yaml:"base_url"`
    CdnBaseURL   string `yaml:"cdn_base_url"`
    UserID       string `yaml:"user_id"`
    MaxMediaSize int    `yaml:"max_media_size"`
}
```

### config.yaml Example

```yaml
platforms:
  weixin:
    enabled: true
    token: "bot_token_from_qr_login"
    base_url: "https://ilinkai.weixin.qq.com"
    cdn_base_url: "https://novac2c.cdn.weixin.qq.com/c2c"
    user_id: "xxx"
    max_media_size: 52428800  # 50MB
```

### Defaults

- `base_url`: `https://ilinkai.weixin.qq.com`
- `cdn_base_url`: `https://novac2c.cdn.weixin.qq.com/c2c`
- `max_media_size`: 50MB (52428800)

### Platform Registration (app.go)

```go
if cfg.Bee.Platforms.Weixin.Enabled {
    p := weixin.NewPlatform(cfg.Bee.Platforms.Weixin, mediaSvc)
    platforms = append(platforms, p)
}
```

## Receiver — Long-Polling Loop

```
Start(ctx, dispatch)
  ├─ Load token, baseUrl from config
  ├─ Initialize sync buffer (load cursor from ~/.openbee/weixin/sync.json)
  └─ Loop:
       ├─ Check session pause → sleep if paused, continue
       ├─ getUpdates(syncBuf, timeout=35s)
       ├─ Error handling:
       │    ├─ errcode -14 → trigger session pause, log warning
       │    ├─ Network timeout → retry immediately
       │    └─ 3 consecutive failures → backoff 30s
       ├─ Persist new sync buffer cursor
       └─ For each msg:
            ├─ Filter: message_type=1 (USER) AND message_state=2 (FINISH)
            ├─ Parse item_list:
            │    ├─ TEXT → extract text
            │    ├─ IMAGE → CDN download+decrypt → media.SaveInbound → placeholder
            │    ├─ VOICE → CDN download+decrypt → SILK→WAV (if ffmpeg) → placeholder
            │    │          Also extract voice_item.text as STT
            │    ├─ VIDEO → CDN download+decrypt → placeholder
            │    └─ FILE → CDN download+decrypt → placeholder
            ├─ Build InboundMessage:
            │    ├─ Platform: "weixin"
            │    ├─ SenderID: from_user_id
            │    ├─ SessionKey: "weixin:{from_user_id}:{from_user_id}"
            │    ├─ Content: text + media placeholders
            │    ├─ RawContent: text only (no media placeholders)
            │    ├─ Raw: JSON marshal of WeixinMessage (includes context_token)
            │    ├─ PlatformMessageID: strconv.FormatInt(message_id, 10)
            │    └─ MessageTime: create_time_ms
            ├─ sendTyping(TYPING)
            └─ dispatch(inboundMsg)
```

## Sender — Message Sending

```
Send(ctx, msg)
  ├─ Deserialize msg.ReplyTo.Raw → extract context_token, to_user_id
  ├─ markdownToPlainText(msg.Content)
  ├─ If msg.MediaPath is set:
  │    ├─ Read file, detect MIME
  │    ├─ Generate random AES-128 key (16 bytes)
  │    ├─ AES-128-ECB encrypt file
  │    ├─ Compute MD5 (plaintext + ciphertext)
  │    ├─ getUploadUrl() → get upload credentials
  │    ├─ uploadBufferToCdn() → upload to CDN
  │    └─ Send as IMAGE/VIDEO/FILE/VOICE based on MIME
  └─ Text only: sendMessage(TEXT item)
```

## CDN Media Encryption (cdn.go)

### AES-128-ECB

```go
func encryptAES128ECB(plaintext []byte, key []byte) []byte
func decryptAES128ECB(ciphertext []byte, key []byte) ([]byte, error)
func downloadAndDecrypt(encryptedQueryParam, aesKeyBase64, cdnBaseUrl string) ([]byte, error)
func encryptAndUpload(data []byte, uploadParam, filekey, cdnBaseUrl string) (downloadParam, aesKey string, err error)
```

- PKCS#7 padding to 16-byte blocks
- Manual ECB mode using Go `crypto/aes` (block-by-block)
- Upload retries: up to 3 times with exponential backoff
- **Note:** ECB mode is inherently insecure for multi-block data; this is dictated by the WeChat protocol, not a design choice

### Comparison with WeCom

| | WeChat | WeCom |
|--|--------|-------|
| Algorithm | AES-128-ECB | AES-256-CBC |
| Key length | 16 bytes | 32 bytes |
| IV | None (ECB) | First 16 bytes of key |
| Padding | PKCS#7 (16-byte blocks) | PKCS#7 (32-byte blocks) |
| Scope | Upload + download | Download only |

Implemented independently in each platform package.

## Markdown to Plain Text

WeChat does not support Markdown rendering. Conversion rules:

```go
func markdownToPlainText(text string) string
```

| Markdown | Plain Text |
|----------|------------|
| `` ```code``` `` | Code content (no fences) |
| `` `inline` `` | Remove backticks |
| `**bold**` | Remove markers |
| `*italic*` | Remove markers |
| `[text](url)` | `text (url)` |
| `![alt](url)` | Remove entirely |
| `# Heading` | Remove `#` prefix |
| Tables | Keep as-is (pipe chars are readable) |

Regex-based sequential replacement, no AST parsing.

## Sync Buffer Persistence

Long-polling cursor persisted to `~/.openbee/weixin/sync.json` for resume after restart:
- Load on startup; if missing or corrupted, start fresh (empty cursor = full sync)
- Save after each successful `getUpdates` response
- Not configurable (internal state file)

## Voice Transcoding

SILK-to-WAV transcoding reuses the existing `internal/ffmedia` package (already used by DingTalk) rather than reimplementing. Falls back to raw SILK + STT text if ffmpeg is unavailable.

## Testing

| Area | Strategy |
|------|----------|
| `markdownToPlainText` | Unit tests with table-driven cases |
| AES-128-ECB encrypt/decrypt | Unit tests: roundtrip, known vectors, padding edge cases |
| Message filtering | Unit tests: message_type/message_state combinations |
| Receiver long-poll | Integration test with mock HTTP server |
| CDN upload/download | Unit tests with mock HTTP + known ciphertext |

## Files to Modify

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `WeixinConfig` struct, add `Weixin` field to `PlatformsConfig`, add defaults in `applyDefaults()` |
| `internal/config/config.yaml.tmpl` | Add `weixin` section template |
| `internal/app/app.go` | Add weixin platform construction in `buildPlatforms` |
| `cmd/openbee/config.go` | Add "微信" to platform multi-select, implement QR scan flow |

## Protocol Types (types.go)

```go
const (
    MessageTypeUser = 1
    MessageTypeBot  = 2
)

const (
    MessageItemTypeText  = 1
    MessageItemTypeImage = 2
    MessageItemTypeVoice = 3
    MessageItemTypeFile  = 4
    MessageItemTypeVideo = 5
)

const (
    MessageStateNew        = 0
    MessageStateGenerating = 1
    MessageStateFinish     = 2
)

const (
    TypingStatusTyping = 1
    TypingStatusCancel = 2
)

type WeixinMessage struct {
    Seq          int64          `json:"seq,omitempty"`
    MessageID    int64          `json:"message_id,omitempty"`
    FromUserID   string         `json:"from_user_id,omitempty"`
    ToUserID     string         `json:"to_user_id,omitempty"`
    CreateTimeMs int64          `json:"create_time_ms,omitempty"`
    MessageType  int            `json:"message_type,omitempty"`
    MessageState int            `json:"message_state,omitempty"`
    ItemList     []MessageItem  `json:"item_list,omitempty"`
    ContextToken string         `json:"context_token,omitempty"`
    SessionID    string         `json:"session_id,omitempty"`
}

type MessageItem struct {
    Type       int        `json:"type,omitempty"`
    TextItem   *TextItem  `json:"text_item,omitempty"`
    ImageItem  *ImageItem `json:"image_item,omitempty"`
    VoiceItem  *VoiceItem `json:"voice_item,omitempty"`
    FileItem   *FileItem  `json:"file_item,omitempty"`
    VideoItem  *VideoItem `json:"video_item,omitempty"`
}

type CDNMedia struct {
    EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
    AesKey            string `json:"aes_key,omitempty"`
    EncryptType       int    `json:"encrypt_type,omitempty"`
}
```

## Error Handling

### Long-polling resilience

| Scenario | Action |
|----------|--------|
| Network timeout | Retry immediately, no failure count |
| API error (non -14) | Increment failure count; backoff 30s after 3 consecutive |
| errcode -14 (session timeout) | Pause 1 hour, log warning |
| Successful response | Reset failure counter |

### Media resilience

| Scenario | Action |
|----------|--------|
| CDN download failure | Skip media item, dispatch text portion, log error |
| CDN upload failure | Retry 3x (exponential backoff), fallback to text-only |
| SILK→WAV failure (no ffmpeg) | Skip transcoding, use STT text if available |

### Send resilience

| Scenario | Action |
|----------|--------|
| Send failure | Log error, no retry (consistent with other platforms) |
| Missing context_token | Log warning, attempt send anyway |
