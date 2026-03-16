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
| `cmd/server/app.go` | Add WeCom to `buildPlatforms()`; pass `WeComConfig` |
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
    │  4. pendingStreams.Store(msgId, streamId)
    │  5. dispatch(InboundMessage{Raw: JSON-encoded WsFrame})
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
    WsCmdSubscribe          = "aibot_subscribe"
    WsCmdHeartbeat          = "ping"
    WsCmdCallback           = "aibot_msg_callback"
    WsCmdEventCallback      = "aibot_event_callback"
    WsCmdResponse           = "aibot_respond_msg"
    WsCmdSendMsg            = "aibot_send_msg"
    WsCmdUploadMediaInit    = "aibot_upload_media_init"
    WsCmdUploadMediaChunk   = "aibot_upload_media_chunk"
    WsCmdUploadMediaFinish  = "aibot_upload_media_finish"
)
```

### WsConn struct and interface

```go
type WsConnConfig struct {
    BotID               string
    Secret              string
    URL                 string
    HeartbeatInterval   time.Duration // default 30s
    MaxReconnectAttempts int          // default 100
    ReconnectBaseDelay  time.Duration // default 1s, exponential backoff, cap 30s
    ReplyAckTimeout     time.Duration // default 5s
}

type WsConn struct {
    // internal state
}

func NewWsConn(cfg WsConnConfig) *WsConn
func (c *WsConn) Connect(ctx context.Context) error  // blocks until ctx cancelled
func (c *WsConn) SendReply(reqID, cmd string, body any) (WsFrame, error)
func (c *WsConn) IsConnected() bool

// Callbacks set before Connect()
OnAuthenticated func()
OnMessage       func(frame WsFrame)
```

### Connection lifecycle

1. Open WebSocket to `wsUrl`
2. On open: send `aibot_subscribe` frame with `{bot_id, secret}`
3. On subscribe ack (errcode=0): start heartbeat ticker, call `OnAuthenticated`
4. Heartbeat: every 30s send `ping`; track missed pongs; if ≥2 missed → `ws.Close()` → triggers reconnect
5. On close: exponential back-off reconnect (1s, 2s, 4s … cap 30s), max 100 attempts
6. Inbound `aibot_msg_callback` / `aibot_event_callback` → `OnMessage(frame)`
7. Other frames (no cmd): matched by `req_id` prefix to release reply queue or ack pending

### Reply queue

Same-`req_id` messages are serialized: send head item → wait for ack (or 5s timeout) → send next. Uses `map[string][]replyQueueItem` + `map[string]pendingAck`. Prevents WeCom from receiving out-of-order frames for the same conversation.

### req_id generation

```go
func generateReqID(prefix string) string {
    // prefix + "-" + timestamp_ms + "-" + random_hex
}
```

---

## `crypto.go` — AES Decryption

Ported from `aibot-node-sdk-main/src/crypto.ts`.

```go
// DecryptFile decrypts a WeCom media file using AES-256-CBC.
// aesKeyBase64 is the Base64-encoded 256-bit key from the message body (image.aeskey / file.aeskey).
// IV = first 16 bytes of the decoded key.
// Padding = PKCS#7 with 32-byte block size (manual removal, no auto-padding).
func DecryptFile(encrypted []byte, aesKeyBase64 string) ([]byte, error)
```

Uses only stdlib `crypto/aes` and `crypto/cipher` — no new dependencies.

---

## `handler.go` — WeComPlatform / WeComReceiver / WeComSender

### WeComPlatform

```go
type WeComPlatform struct {
    receiver       *WeComReceiver
    sender         *WeComSender
    pendingStreams  sync.Map  // key: msgId (string) → value: streamId (string)
}

func NewPlatform(cfg config.WeComConfig, mediaSvc *media.Service) platform.Platform

func (p *WeComPlatform) ID() string                                  { return "wecom" }
func (p *WeComPlatform) Receiver() platform.PlatformReceiverAdapter  { return p.receiver }
func (p *WeComPlatform) Sender() platform.PlatformSenderAdapter      { return p.sender }
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
   - Single: `chatId = body.from.userid`, `senderID = body.from.userid`
   - Group: `chatId = body.chatid`, `senderID = body.from.userid`
3. Extract text content by `msgtype`:
   - `text` → `body.text.content`
   - `voice` → `body.voice.content` (already transcribed)
   - `image` → `downloadDecryptSave(body.image.url, body.image.aeskey, "image")` → placeholder
   - `file` → `downloadDecryptSave(body.file.url, body.file.aeskey, "document")` → placeholder
   - `mixed` → iterate `body.mixed.msg_item`, parallel-download images via errgroup
   - `quote` → append quoted text/image after main content
   - Other → skip with `slog.Warn`
4. If content empty → skip
5. Send thinking message:
   ```
   streamId = generateReqID("stream")
   wsConn.SendReply(frame.Headers.ReqID, WsCmdResponse, {
       msgtype: "stream",
       stream: { id: streamId, finish: false, content: "<think></think>" }
   })
   ```
6. `pendingStreams.Store(body.msgid, streamId)`
7. Marshal `frame` → rawBytes
8. `dispatch(InboundMessage{Platform: "wecom", SenderID: senderID, SessionKey: "wecom:"+chatId+":"+senderID, Content: content, Raw: rawBytes, PlatformMessageID: body.msgid, MessageTime: body.create_time * 1000})`

**`downloadDecryptSave`:**
- HTTP GET encrypted bytes
- `DecryptFile(bytes, aeskey)` if aeskey non-empty
- `mediaSvc.DetectMIME` → ext
- `mediaSvc.SaveInbound` → path
- `mediaSvc.BuildPlaceholder(mediaType, path, filename)`

### WeComSender

```go
type WeComSender struct {
    pendingStreams *sync.Map
    wsConn        *WsConn
}

func (s *WeComSender) Send(ctx context.Context, msg platform.OutboundMessage) error
```

**Send steps:**

1. Unmarshal `msg.ReplyTo.Raw` → `WsFrame`; extract `reqID = frame.Headers.ReqID`, `msgId = body.msgid`, `chatId`
2. `streamId, ok = pendingStreams.LoadAndDelete(msgId)`; if not found use a new streamId (fallback)
3. If `msg.MediaPath != ""`:
   a. Read file bytes
   b. Detect MIME → WeCom media type (`image` / `voice` / `video` / `file`); voice must be AMR, otherwise send as file
   c. Chunk upload (512KB/chunk, base64):
      - `SendReply(newReqID, WsCmdUploadMediaInit, {type, filename, total_size, total_chunks, md5})`
      - For each chunk: `SendReply(newReqID, WsCmdUploadMediaChunk, {upload_id, chunk_index, base64_data})`
      - `SendReply(newReqID, WsCmdUploadMediaFinish, {upload_id})` → `media_id`
   d. `SendReply(newReqID, WsCmdSendMsg, {chatid, msgtype, [msgtype]: {media_id}})`
   e. Finish thinking stream: `SendReply(reqID, WsCmdResponse, stream{id: streamId, finish: true, content: "📎 文件已发送，请查收。"})`
4. Else (text):
   - `SendReply(reqID, WsCmdResponse, {msgtype: "stream", stream: {id: streamId, finish: true, content: msg.Content}})`

### SessionKey format

`wecom:<chatId>:<senderUserId>`

- Single chat: `wecom:<userid>:<userid>`
- Group chat: `wecom:<chatid>:<userid>`

---

## `cmd/server/app.go` changes

`buildPlatforms` signature extended:

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

Call site in `buildApp` updated to pass `cfg.Bee.Platforms.WeCom`.

---

## Error Handling

- Thinking message send failure: log warning, continue (don't block dispatch)
- Media download/decrypt failure: log error, use `mediaSvc.BuildPlaceholder(type, "", filename)` (no path)
- Media upload failure in sender: return error (caller logs)
- WsConn `SendReply` timeout: return error
- Reconnect after auth failure: log error + reconnect (same as DingTalk supervisor pattern)

---

## Dependencies

No new Go module dependencies. Uses only:
- `github.com/gorilla/websocket` (already in go.mod)
- stdlib: `crypto/aes`, `crypto/cipher`, `crypto/md5`, `encoding/base64`, `encoding/json`

---

## Testing

- `crypto_test.go` — unit test for `DecryptFile` with known test vectors
- `handler_test.go` — test `processMessage` for each message type using mock wsConn
- Existing `msgingest` and platform interface compile-time checks (`var _ platform.Platform = ...`)
