# WeCom (企业微信) Platform Integration Design

**Date:** 2026-03-16
**Status:** Approved

## Overview

Add WeCom (企业微信 AI Bot) as a third messaging platform in robobee/core, alongside the existing Feishu and DingTalk integrations. The implementation uses the WeCom AI Bot WebSocket persistent connection API.

**Reference material:**
- `wecom-openclaw-plugin-main/` — TypeScript WeCom plugin (OpenClaw framework)
- `aibot-node-sdk-main/` — WeCom AI Bot Node.js SDK (protocol reference)
- `internal/platform/feishu/` and `internal/platform/dingtalk/` — existing Go platform patterns

---

## Architecture

### New Files

```
internal/platform/wecom/
├── wsconn.go    # WebSocket connection manager (ported from aibot-node-sdk ws.ts)
├── crypto.go    # AES-256-CBC file decryption (ported from aibot-node-sdk crypto.ts)
└── handler.go   # WeComPlatform / WeComReceiver / WeComSender implementation
```

### Modified Files

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `WeComConfig` struct; add to `PlatformsConfig`; add default for `WebSocketURL` |
| `cmd/server/app.go` | Add WeCom to `buildPlatforms()`; update call site |
| `config.example.yaml` | Add WeCom config example |

### Data Flow

```
WeCom WS Server
    │  aibot_msg_callback frame
    ▼
WeComReceiver.Start()
    │  1. Parse frame body → MessageBody
    │  2. Extract content by msgtype (download+decrypt media if needed)
    │  3. Send <think></think> thinking indicator (aibot_respond_msg, finish=false)
    │  4. pendingStreams.Store(msgId, streamId) + schedule 10-min TTL cleanup
    │  5. dispatch(InboundMessage{Raw: string(JSON-encoded WsFrame)})
    ▼
msgingest.Gateway → bee.Feeder → AI processing
    ▼
WeComSender.Send()
    │  1. Decode Raw → WsFrame; extract req_id, chatId, msgId
    │  2. pendingStreams.LoadAndDelete(msgId) → streamId
    │  3a. Text: aibot_respond_msg (stream, finish=true, content=reply)
    │  3b. Media: 3-step chunked upload → aibot_send_msg + finish thinking stream
    ▼
WeCom WS Server
```

---

## Configuration

### `internal/config/config.go`

```go
type WeComConfig struct {
    Enabled      bool   `yaml:"enabled"`
    BotID        string `yaml:"bot_id"`
    Secret       string `yaml:"secret"`
    WebSocketURL string `yaml:"websocket_url"` // default: wss://openws.work.weixin.qq.com
}
```

`PlatformsConfig` extended:
```go
type PlatformsConfig struct {
    Feishu   FeishuConfig   `yaml:"feishu"`
    DingTalk DingTalkConfig `yaml:"dingtalk"`
    WeCom    WeComConfig    `yaml:"wecom"`
}
```

`applyDefaults` addition:
```go
if cfg.Bee.Platforms.WeCom.WebSocketURL == "" {
    cfg.Bee.Platforms.WeCom.WebSocketURL = "wss://openws.work.weixin.qq.com"
}
```

### `config.example.yaml` addition

```yaml
platforms:
  wecom:
    enabled: false
    bot_id: "YOUR_BOT_ID"
    secret: "YOUR_BOT_SECRET"
    # websocket_url: wss://openws.work.weixin.qq.com
```

---

## `wsconn.go` — WebSocket Connection Manager

Ported directly from `aibot-node-sdk-main/src/ws.ts`. All protocol logic lives here.

### WsFrame types

```go
type WsFrame struct {
    Cmd     string          `json:"cmd,omitempty"`
    Headers WsFrameHeaders  `json:"headers"`
    Body    json.RawMessage `json:"body,omitempty"`
    ErrCode int             `json:"errcode,omitempty"`
    ErrMsg  string          `json:"errmsg,omitempty"`
}

type WsFrameHeaders struct {
    ReqID string `json:"req_id"`
}
```

### WsCmd constants

```go
const (
    WsCmdSubscribe         = "aibot_subscribe"
    WsCmdHeartbeat         = "ping"
    WsCmdCallback          = "aibot_msg_callback"
    WsCmdEventCallback     = "aibot_event_callback"
    WsCmdResponse          = "aibot_respond_msg"
    WsCmdSendMsg           = "aibot_send_msg"
    WsCmdUploadMediaInit   = "aibot_upload_media_init"
    WsCmdUploadMediaChunk  = "aibot_upload_media_chunk"
    WsCmdUploadMediaFinish = "aibot_upload_media_finish"
)
```

### WsConn struct and interface

```go
type WsConnConfig struct {
    BotID                string
    Secret               string
    URL                  string
    HeartbeatInterval    time.Duration // default 30s
    MaxReconnectAttempts int           // default 100
    ReconnectBaseDelay   time.Duration // default 1s, exponential backoff, cap 30s
    ReplyAckTimeout      time.Duration // default 5s
}

type WsConn struct { /* internal state */ }

func NewWsConn(cfg WsConnConfig) *WsConn
func (c *WsConn) Connect(ctx context.Context) error  // blocks until ctx cancelled
func (c *WsConn) SendReply(reqID, cmd string, body any) (WsFrame, error)
func (c *WsConn) IsConnected() bool

// Callbacks — set before Connect()
OnAuthenticated func()
OnMessage       func(frame WsFrame)
```

### Connection lifecycle

1. Open WebSocket to `wsUrl`
2. On open: send `aibot_subscribe` frame with `{bot_id, secret}`
3. On subscribe ack (errcode=0): start heartbeat ticker, call `OnAuthenticated`
4. Heartbeat: every 30s send `ping`; track missed pongs; if ≥2 missed → `ws.Close()` → triggers reconnect
5. On close: exponential back-off reconnect (1s, 2s, 4s … cap 30s), max 100 attempts
6. Inbound `aibot_msg_callback` → `OnMessage(frame)`
7. Inbound `aibot_event_callback` → drop with `slog.Debug`. Event types such as `enter_chat` and `template_card_event` are **out of scope** for this integration; welcome messages and card interactions are not supported.
8. Other frames (no cmd): matched by `req_id` prefix to release reply queue or ack pending

### Reply queue

Same-`req_id` messages are serialized: send head item → wait for server ack (or 5s timeout) → send next. Uses `map[string][]replyQueueItem` + `map[string]pendingAck`. Prevents WeCom from receiving out-of-order frames for the same conversation.

### req_id generation

```go
func generateReqID(prefix string) string {
    // prefix + "-" + timestamp_ms + "-" + random_hex
}
```

Each distinct send operation (subscribe, heartbeat, each upload step, each reply) must use its own freshly generated `req_id`. Never reuse a `req_id` across different operations or upload steps, as this would serialize unrelated operations through the same reply queue slot and cause deadlocks.

---

## `crypto.go` — AES Decryption

Ported from `aibot-node-sdk-main/src/crypto.ts`.

```go
// DecryptFile decrypts a WeCom media file using AES-256-CBC.
// aesKeyBase64 is the Base64-encoded 256-bit key from the message body
// (image.aeskey / file.aeskey). IV = first 16 bytes of the decoded key.
// Padding = PKCS#7 with 32-byte block size (manual removal, no auto-padding).
func DecryptFile(encrypted []byte, aesKeyBase64 string) ([]byte, error)
```

Uses only stdlib `crypto/aes` and `crypto/cipher` — no new dependencies.

---

## `handler.go` — WeComPlatform / WeComReceiver / WeComSender

### WeComPlatform

```go
type WeComPlatform struct {
    receiver      *WeComReceiver
    sender        *WeComSender
    pendingStreams sync.Map  // key: msgId (string) → value: streamId (string)
}

func NewPlatform(cfg config.WeComConfig, mediaSvc *media.Service) platform.Platform

func (p *WeComPlatform) ID() string                                 { return "wecom" }
func (p *WeComPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *WeComPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }
```

### WeComReceiver

```go
type WeComReceiver struct {
    cfg           config.WeComConfig
    pendingStreams *sync.Map
    mediaSvc      *media.Service
    wsConn        *WsConn
}

func (r *WeComReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error
```

**`processMessage(frame, dispatch)` steps:**

1. Unmarshal `frame.Body` → `MessageBody`
2. Determine `chatId` and `senderID`:
   - Single (`chattype == "single"`): `chatId = body.from.userid`, `senderID = body.from.userid`
   - Group (`chattype == "group"`): `chatId = body.chatid`, `senderID = body.from.userid`
3. Extract text content by `msgtype`. For `quote` messages, first extract the primary message content (from the top-level `msgtype` field, e.g. `body.text.content`), then append the quoted content afterward:
   - `text` → `rawText = body.text.content`; `content = rawText`
   - `voice` → `content = body.voice.content` (already transcribed by WeCom); `rawText = content`
   - `image` → `content = downloadDecryptSave(body.image.url, body.image.aeskey, "image", "")`; `rawText = ""`
   - `file` → `content = downloadDecryptSave(body.file.url, body.file.aeskey, "document", filename)`; `rawText = ""`
   - `mixed` → iterate `body.mixed.msg_item`, parallel-download images via `errgroup`; join text parts and image placeholders in order; `rawText = joined plain text parts only` (no image placeholders; `at`-markup in text items is preserved as-is)
   - If `body.quote != nil`: append quoted content to `content` based on `body.quote.msgtype`:
     - `text` → append `body.quote.text.content`
     - `voice` → append `body.quote.voice.content`
     - `image` → `downloadDecryptSave(quote.image.url, quote.image.aeskey, "image", "")` → append placeholder
     - `file` → `downloadDecryptSave(quote.file.url, quote.file.aeskey, "document", filename)` → append placeholder
     - `mixed` → iterate `quote.mixed.msg_item` same as top-level `mixed` (parallel image download via errgroup), append result
     - Other quote types → skip
   - Other `msgtype` → `slog.Warn("skipping unsupported msgtype")`, return
4. If `content` is empty → return (skip dispatch)
5. Send thinking message (failure → `slog.Warn`, continue):
   ```
   streamId = generateReqID("stream")
   wsConn.SendReply(frame.Headers.ReqID, WsCmdResponse, {
       msgtype: "stream",
       stream: { id: streamId, finish: false, content: "<think></think>" }
   })
   ```
6. `pendingStreams.Store(body.msgid, streamId)`; schedule TTL cleanup via `time.AfterFunc(10*time.Minute, func() { pendingStreams.Delete(body.msgid) })` to prevent leaks if the downstream pipeline drops the message without ever calling `Send`
7. Marshal `frame` → `rawBytes`
8. `dispatch(platform.InboundMessage{Platform: "wecom", SenderID: senderID, SessionKey: "wecom:" + chatId + ":" + senderID, Content: content, RawContent: rawText, Raw: string(rawBytes), PlatformMessageID: body.msgid, MessageTime: body.create_time * 1000})` — note: `create_time` is Unix seconds from WeCom; multiply by 1000 for milliseconds; `Raw` is `string` not `[]byte`

**`downloadDecryptSave(url, aeskey, mediaType, filename string) string`:**
- HTTP GET encrypted bytes (120s timeout)
- If `aeskey != ""`: `DecryptFile(bytes, aeskey)`
- `mediaSvc.DetectMIME(data, filename)` → MIME → ext
- `mediaSvc.SaveInbound(ctx, data, ext)` → path
- Return `mediaSvc.BuildPlaceholder(mediaType, path, filename)`; on any error log and return `mediaSvc.BuildPlaceholder(mediaType, "", filename)`

### WeComSender

```go
type WeComSender struct {
    pendingStreams *sync.Map
    wsConn        *WsConn
}

func (s *WeComSender) Send(ctx context.Context, msg platform.OutboundMessage) error
```

**Send steps:**

1. Unmarshal `msg.ReplyTo.Raw` → `WsFrame`; unmarshal `frame.Body` → `MessageBody`; extract `reqID = frame.Headers.ReqID`, `msgId = body.msgid`, `chatId` (same logic as receiver step 2)
2. `val, ok = pendingStreams.LoadAndDelete(msgId)`; extract `streamId` from `val` if ok; if not found generate a new `streamId` as fallback
3. If `msg.MediaPath != ""`:
   a. Read file bytes from `msg.MediaPath`
   b. Detect MIME → WeCom media type; apply size limits and downgrade rules:
      - `image`: max 10 MB; if >10 MB downgrade to `file`
      - `video`: max 10 MB; if >10 MB downgrade to `file`
      - `voice`: only AMR (`audio/amr`) is supported; non-AMR → downgrade to `file`; max 2 MB; if >2 MB downgrade to `file`
      - `file`: max 20 MB; if >20 MB return error (reject, do not upload)
   c. Chunk upload — **each step uses its own freshly generated `req_id`**:
      - Compute `md5 = md5(fileBytes)`, `totalChunks = ceil(len / 512KB)`
      - `SendReply(generateReqID(WsCmdUploadMediaInit), WsCmdUploadMediaInit, {type, filename, total_size, total_chunks, md5})` → `upload_id`
      - For each chunk index `i`: `SendReply(generateReqID(WsCmdUploadMediaChunk), WsCmdUploadMediaChunk, {upload_id, chunk_index: i, base64_data})` — **sequential only** (the SDK reference warns that high-concurrency chunk submission causes WeCom backend to return system errors; concurrency is intentionally not used)
      - `SendReply(generateReqID(WsCmdUploadMediaFinish), WsCmdUploadMediaFinish, {upload_id})` → `media_id`
   d. `SendReply(generateReqID(WsCmdSendMsg), WsCmdSendMsg, {chatid, msgtype, [msgtype]: {media_id}})` — note: the JSON wire key is `chatid` (snake_case), matching the WeCom protocol; Go variable is `chatId` but the struct tag must be `json:"chatid"`
   e. Finish thinking stream: `SendReply(reqID, WsCmdResponse, {msgtype: "stream", stream: {id: streamId, finish: true, content: "📎 文件已发送，请查收。"}})`
4. Else (text):
   - `SendReply(reqID, WsCmdResponse, {msgtype: "stream", stream: {id: streamId, finish: true, content: msg.Content}})`

### SessionKey format

`wecom:<chatId>:<senderUserId>`

- Single chat: `wecom:<userid>:<userid>`
- Group chat: `wecom:<chatid>:<userid>`

---

## `cmd/server/app.go` changes

`buildPlatforms` updated — `wc` inserted before `mc` to keep WeCom adjacent to other platform configs:

```go
func buildPlatforms(fc config.FeishuConfig, dc config.DingTalkConfig, wc config.WeComConfig, mc config.MediaConfig) []platform.Platform {
    mediaSvc := media.NewService()
    var result []platform.Platform
    if fc.Enabled { result = append(result, feishu.NewPlatform(fc, mediaSvc)) }
    if dc.Enabled { result = append(result, dingtalk.NewPlatform(dc, mc, mediaSvc)) }
    if wc.Enabled { result = append(result, wecom.NewPlatform(wc, mediaSvc)) }
    return result
}
```

Call site in `buildApp` (line ~97) updated:
```go
platforms := buildPlatforms(cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom, cfg.Bee.Media)
```

---

## Error Handling

- Thinking message send failure: `slog.Warn`, continue (don't block dispatch)
- `aibot_event_callback` frames: drop with `slog.Debug`
- Media download/decrypt failure: `slog.Error`, use `mediaSvc.BuildPlaceholder(type, "", filename)` (no path)
- Media upload failure in sender: return error
- File >20 MB: return error without uploading
- `WsConn.SendReply` timeout (5s): return error
- Reconnect after auth failure: `slog.Error` + reconnect (same as DingTalk supervisor pattern)
- `pendingStreams` orphan leak: prevented by 10-minute `time.AfterFunc` TTL cleanup set at Store time

---

## Dependencies

No new Go module dependencies. Uses only:
- `github.com/gorilla/websocket` (already in go.mod)
- stdlib: `crypto/aes`, `crypto/cipher`, `crypto/md5`, `encoding/base64`, `encoding/json`

---

## Testing

- `crypto_test.go` — unit test for `DecryptFile` with known test vectors (encrypt with known key, verify round-trip)
- `handler_test.go` — test `processMessage` for each message type (text, voice, image, file, mixed, quote) using a mock `dispatch` function; verify `SessionKey`, `Content`, `RawContent`, `PlatformMessageID` for each
- Compile-time interface checks at bottom of `handler.go`:
  ```go
  var _ platform.Platform                = (*WeComPlatform)(nil)
  var _ platform.PlatformReceiverAdapter = (*WeComReceiver)(nil)
  var _ platform.PlatformSenderAdapter   = (*WeComSender)(nil)
  ```
