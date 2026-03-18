# WeCom Integration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add WeCom (企业微信 AI Bot) as a third messaging platform in robobee/core, with full parity to the existing Feishu and DingTalk integrations.

**Architecture:** `internal/platform/wecom/` contains three files: `wsconn.go` (WebSocket connection manager), `crypto.go` (AES-256-CBC decryption), and `handler.go` (Platform/Receiver/Sender). Config wired through `internal/config/config.go` and `cmd/server/app.go`. The WeCom AI Bot protocol uses WebSocket with `aibot_subscribe` auth, `ping` heartbeat, `aibot_msg_callback` inbound, and `aibot_respond_msg`/`aibot_send_msg` outbound.

**Tech Stack:** Go, `github.com/gorilla/websocket` (already in go.mod), stdlib crypto only. Reference: `aibot-node-sdk-main/src/` (TypeScript SDK) and `wecom-openclaw-plugin-main/src/`.

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/config/config.go` | Modify | Add `WeComConfig`, extend `PlatformsConfig`, add default |
| `config.example.yaml` | Modify | Add WeCom example block |
| `internal/platform/wecom/crypto.go` | Create | `DecryptFile` AES-256-CBC |
| `internal/platform/wecom/crypto_test.go` | Create | Unit tests for `DecryptFile` |
| `internal/platform/wecom/wsconn.go` | Create | WebSocket connection manager (frames, auth, heartbeat, reply queue, reconnect) |
| `internal/platform/wecom/wsconn_test.go` | Create | Basic instantiation / config defaults test |
| `internal/platform/wecom/handler.go` | Create | `WeComPlatform`, `WeComReceiver`, `WeComSender`, message body types, `processMessage`, `downloadDecryptSave`, `Send` |
| `internal/platform/wecom/handler_test.go` | Create | Tests for `processMessage` (all msg types) |
| `cmd/server/app.go` | Modify | `buildPlatforms` signature + call site |

---

## Chunk 1: Config + Crypto

### Task 1: Add WeComConfig to config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config.example.yaml`

- [ ] **Step 1.1: Add `WeComConfig` struct and extend `PlatformsConfig`**

In `internal/config/config.go`, add after `DingTalkConfig`:

```go
type WeComConfig struct {
	Enabled      bool   `yaml:"enabled"`
	BotID        string `yaml:"bot_id"`
	Secret       string `yaml:"secret"`
	WebSocketURL string `yaml:"websocket_url"`
}
```

Change `PlatformsConfig` to:
```go
type PlatformsConfig struct {
	Feishu   FeishuConfig   `yaml:"feishu"`
	DingTalk DingTalkConfig `yaml:"dingtalk"`
	WeCom    WeComConfig    `yaml:"wecom"`
}
```

Add to `applyDefaults` (after the DingTalk block is fine, before the `return nil`):
```go
if cfg.Bee.Platforms.WeCom.WebSocketURL == "" {
    cfg.Bee.Platforms.WeCom.WebSocketURL = "wss://openws.work.weixin.qq.com"
}
```

- [ ] **Step 1.2: Add WeCom example block to `config.example.yaml`**

Append under the existing `platforms:` section (after the `dingtalk:` block). Use 4-space indentation to match `feishu:` and `dingtalk:`, with 6-space indentation for children:
```yaml
    wecom:
      enabled: false
      bot_id: "YOUR_BOT_ID"
      secret: "YOUR_BOT_SECRET"
      # websocket_url: wss://openws.work.weixin.qq.com  # optional, this is the default
```

- [ ] **Step 1.3: Verify compile**

```bash
cd /Users/tengteng/work/robobee/core && go build ./internal/config/...
```
Expected: no errors.

- [ ] **Step 1.4: Commit**

```bash
git add internal/config/config.go config.example.yaml
git commit -m "feat(config): add WeComConfig to PlatformsConfig"
```

---

### Task 2: AES decryption (`crypto.go`)

**Files:**
- Create: `internal/platform/wecom/crypto.go`
- Create: `internal/platform/wecom/crypto_test.go`

**Background:** WeCom encrypts downloaded media files with AES-256-CBC. The key is Base64-encoded 32 bytes; IV = first 16 bytes of the key. Padding is PKCS#7 with 32-byte block size (non-standard — must be removed manually). Reference: `aibot-node-sdk-main/src/crypto.ts`.

- [ ] **Step 2.1: Write the failing test first**

Create `internal/platform/wecom/crypto_test.go`:

```go
package wecom

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// encryptForTest encrypts plaintext with AES-256-CBC using the WeCom scheme:
// IV = key[:16], PKCS#7 padding to 32-byte block multiples.
func encryptForTest(t *testing.T, plaintext, key []byte) []byte {
	t.Helper()
	iv := key[:16]
	block, err := aes.NewCipher(key)
	require.NoError(t, err)

	// PKCS#7 pad to 32-byte multiple
	padLen := 32 - (len(plaintext) % 32)
	padded := make([]byte, len(plaintext)+padLen)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}

	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)
	return encrypted
}

func TestDecryptFile_RoundTrip(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	aesKeyB64 := base64.StdEncoding.EncodeToString(key)

	plaintext := []byte("hello, wecom media content!")
	encrypted := encryptForTest(t, plaintext, key)

	got, err := DecryptFile(encrypted, aesKeyB64)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestDecryptFile_AlignedPlaintext(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	aesKeyB64 := base64.StdEncoding.EncodeToString(key)

	// Exactly 32 bytes — requires a full 32-byte padding block
	plaintext := make([]byte, 32)
	_, err = rand.Read(plaintext)
	require.NoError(t, err)
	encrypted := encryptForTest(t, plaintext, key)

	got, err := DecryptFile(encrypted, aesKeyB64)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestDecryptFile_EmptyInput(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	aesKeyB64 := base64.StdEncoding.EncodeToString(key)

	_, err := DecryptFile([]byte{}, aesKeyB64)
	assert.Error(t, err)
}

func TestDecryptFile_InvalidBase64Key(t *testing.T) {
	_, err := DecryptFile([]byte("somedata"), "not-valid-base64!!!")
	assert.Error(t, err)
}

func TestDecryptFile_ShortKey(t *testing.T) {
	// 16-byte key encodes to 24 base64 chars; DecryptFile should reject non-32-byte keys.
	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)
	aesKeyB64 := base64.StdEncoding.EncodeToString(key)

	_, err = DecryptFile([]byte("anydatawontmatter"), aesKeyB64)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32-byte key")
}
```

- [ ] **Step 2.2: Run test — expect compile failure (DecryptFile undefined)**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... 2>&1 | head -20
```
Expected: `undefined: DecryptFile`

- [ ] **Step 2.3: Implement `crypto.go`**

Create `internal/platform/wecom/crypto.go`:

```go
package wecom

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
)

// DecryptFile decrypts a WeCom media file using AES-256-CBC.
//
// aesKeyBase64 is the Base64-encoded 256-bit (32-byte) key provided in the
// message body (image.aeskey / file.aeskey). The IV is the first 16 bytes of
// the decoded key. Padding is PKCS#7 with a 32-byte block size — WeCom pads to
// 32-byte multiples rather than the standard AES 16-byte block size, so
// auto-padding must be disabled and padding removed manually.
func DecryptFile(encrypted []byte, aesKeyBase64 string) ([]byte, error) {
	if len(encrypted) == 0 {
		return nil, fmt.Errorf("decryptFile: encrypted data is empty")
	}

	key, err := base64.StdEncoding.DecodeString(aesKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("decryptFile: decode aesKey: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("decryptFile: expected 32-byte key, got %d bytes", len(key))
	}

	iv := key[:16]
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("decryptFile: create cipher: %w", err)
	}

	if len(encrypted)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("decryptFile: ciphertext length %d is not a multiple of AES block size", len(encrypted))
	}

	decrypted := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(decrypted, encrypted)

	// Manual PKCS#7 unpadding — WeCom uses 32-byte blocks, not 16.
	padLen := int(decrypted[len(decrypted)-1])
	if padLen < 1 || padLen > 32 || padLen > len(decrypted) {
		return nil, fmt.Errorf("decryptFile: invalid PKCS#7 padding value: %d", padLen)
	}
	for i := len(decrypted) - padLen; i < len(decrypted); i++ {
		if decrypted[i] != byte(padLen) {
			return nil, fmt.Errorf("decryptFile: PKCS#7 padding bytes inconsistent")
		}
	}
	return decrypted[:len(decrypted)-padLen], nil
}
```

- [ ] **Step 2.4: Run tests — expect all pass**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... -v -run TestDecryptFile
```
Expected: all 5 tests PASS.

- [ ] **Step 2.5: Commit**

```bash
git add internal/platform/wecom/crypto.go internal/platform/wecom/crypto_test.go
git commit -m "feat(wecom): add AES-256-CBC DecryptFile with tests"
```

---

## Chunk 2: WebSocket Connection Manager (`wsconn.go`)

### Task 3: Implement `wsconn.go`

**Files:**
- Create: `internal/platform/wecom/wsconn.go`
- Create: `internal/platform/wecom/wsconn_test.go`

**Background:** Port `aibot-node-sdk-main/src/ws.ts` to Go. Manages the WebSocket connection lifecycle: dial → auth (`aibot_subscribe`) → heartbeat (`ping`) → message dispatch → reconnect with exponential backoff. The reply queue serializes sends per `req_id` and waits for server ack before sending the next frame with the same `req_id`. Each distinct operation must use a freshly generated `req_id`.

- [ ] **Step 3.1: Write the compile/config test first**

Create `internal/platform/wecom/wsconn_test.go`:

```go
package wecom

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewWsConn_Defaults(t *testing.T) {
	c := NewWsConn(WsConnConfig{BotID: "bot1", Secret: "sec1"})
	assert.Equal(t, wsDefaultURL, c.cfg.URL)
	assert.Equal(t, wsDefaultHeartbeat, c.cfg.HeartbeatInterval)
	assert.Equal(t, wsDefaultMaxReconnect, c.cfg.MaxReconnectAttempts)
	assert.Equal(t, wsDefaultReconnectBase, c.cfg.ReconnectBaseDelay)
	assert.Equal(t, wsDefaultAckTimeout, c.cfg.ReplyAckTimeout)
	assert.False(t, c.IsConnected())
}

func TestNewWsConn_CustomURL(t *testing.T) {
	c := NewWsConn(WsConnConfig{
		BotID:             "bot1",
		Secret:            "sec1",
		URL:               "wss://custom.example.com",
		HeartbeatInterval: 10 * time.Second,
	})
	assert.Equal(t, "wss://custom.example.com", c.cfg.URL)
	assert.Equal(t, 10*time.Second, c.cfg.HeartbeatInterval)
}

func TestGenerateReqID_Uniqueness(t *testing.T) {
	ids := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		id := generateReqID("test")
		assert.NotContains(t, ids, id, "generated duplicate req_id")
		ids[id] = struct{}{}
	}
}

func TestGenerateReqID_Prefix(t *testing.T) {
	id := generateReqID("aibot_subscribe")
	assert.Contains(t, id, "aibot_subscribe")
}
```

- [ ] **Step 3.2: Run test — expect compile failure**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... 2>&1 | head -10
```
Expected: compile errors (WsConn, NewWsConn, generateReqID undefined).

- [ ] **Step 3.3: Implement `wsconn.go`**

Create `internal/platform/wecom/wsconn.go`:

```go
package wecom

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Protocol command constants — WeCom AI Bot WebSocket wire commands.
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

// Default connection parameters.
const (
	wsDefaultURL          = "wss://openws.work.weixin.qq.com"
	wsDefaultHeartbeat    = 30 * time.Second
	wsDefaultMaxReconnect = 100
	wsDefaultReconnectBase = 1 * time.Second
	wsDefaultReconnectMax  = 30 * time.Second
	wsDefaultAckTimeout    = 5 * time.Second
	wsMaxMissedPong        = 2
)

// WsFrame is a single JSON message on the WeCom WebSocket wire.
type WsFrame struct {
	Cmd     string          `json:"cmd,omitempty"`
	Headers WsFrameHeaders  `json:"headers"`
	Body    json.RawMessage `json:"body,omitempty"`
	ErrCode int             `json:"errcode,omitempty"`
	ErrMsg  string          `json:"errmsg,omitempty"`
}

// WsFrameHeaders carries the req_id used to correlate requests and acks.
type WsFrameHeaders struct {
	ReqID string `json:"req_id"`
}

// generateReqID creates a unique request ID with the given prefix.
// Format: <prefix>-<unix_ms>-<random_hex>.
func generateReqID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UnixMilli(), hex.EncodeToString(b))
}

// WsConnConfig configures a WsConn. Zero values use defaults.
type WsConnConfig struct {
	BotID                string
	Secret               string
	URL                  string        // default: wss://openws.work.weixin.qq.com
	HeartbeatInterval    time.Duration // default: 30s
	MaxReconnectAttempts int           // default: 100
	ReconnectBaseDelay   time.Duration // default: 1s
	ReplyAckTimeout      time.Duration // default: 5s
}

// replyEntry is a single item in the per-req_id reply queue.
type replyEntry struct {
	frame WsFrame
	done  chan struct{}
	resp  WsFrame
	err   error
}

// WsConn manages a persistent WebSocket connection to the WeCom AI Bot server.
// It handles auth, heartbeat, reconnection, and serialised reply queues.
type WsConn struct {
	cfg WsConnConfig

	mu             sync.Mutex
	conn           *websocket.Conn
	connGeneration int
	isManualClose  bool
	missedPong     int
	reconnectCount int

	queueMu sync.Mutex
	queues  map[string][]*replyEntry // per req_id send queue
	pending map[string]*replyEntry   // req_ids awaiting ack

	// Callbacks — set before Connect().
	OnAuthenticated func()
	OnMessage       func(WsFrame)
}

// NewWsConn creates a WsConn with defaults applied.
func NewWsConn(cfg WsConnConfig) *WsConn {
	if cfg.URL == "" {
		cfg.URL = wsDefaultURL
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = wsDefaultHeartbeat
	}
	if cfg.MaxReconnectAttempts == 0 {
		cfg.MaxReconnectAttempts = wsDefaultMaxReconnect
	}
	if cfg.ReconnectBaseDelay == 0 {
		cfg.ReconnectBaseDelay = wsDefaultReconnectBase
	}
	if cfg.ReplyAckTimeout == 0 {
		cfg.ReplyAckTimeout = wsDefaultAckTimeout
	}
	return &WsConn{
		cfg:     cfg,
		queues:  make(map[string][]*replyEntry),
		pending: make(map[string]*replyEntry),
	}
}

// Connect establishes and maintains the WebSocket connection until ctx is cancelled.
// It blocks until ctx.Done().
func (c *WsConn) Connect(ctx context.Context) error {
	if err := c.dialAndAuth(ctx); err != nil {
		return fmt.Errorf("wecom initial connect: %w", err)
	}
	<-ctx.Done()
	c.mu.Lock()
	c.isManualClose = true
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()
	return nil
}

// dialAndAuth opens a new WebSocket, sends auth, waits for ack, then starts
// heartbeat and disconnect-watcher goroutines.
func (c *WsConn) dialAndAuth(ctx context.Context) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.cfg.URL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connGeneration++
	gen := c.connGeneration
	c.missedPong = 0
	c.mu.Unlock()

	readDone := make(chan struct{})
	authDone := make(chan error, 1)
	go c.readLoop(ctx, conn, gen, readDone, authDone)

	// Send auth frame.
	authReqID := generateReqID(WsCmdSubscribe)
	authBody, _ := json.Marshal(map[string]string{"bot_id": c.cfg.BotID, "secret": c.cfg.Secret})
	if err := c.sendRaw(conn, WsFrame{
		Cmd:     WsCmdSubscribe,
		Headers: WsFrameHeaders{ReqID: authReqID},
		Body:    authBody,
	}); err != nil {
		conn.Close()
		return fmt.Errorf("send auth: %w", err)
	}

	// Wait for auth ack.
	select {
	case err := <-authDone:
		if err != nil {
			conn.Close()
			return fmt.Errorf("auth: %w", err)
		}
	case <-ctx.Done():
		conn.Close()
		return ctx.Err()
	case <-time.After(10 * time.Second):
		conn.Close()
		return fmt.Errorf("auth timeout")
	}

	go c.heartbeatLoop(ctx, conn, gen)
	go c.watchDisconnect(ctx, readDone)
	return nil
}

// sendRaw JSON-encodes and writes a frame to conn. Caller must not hold c.mu.
func (c *WsConn) sendRaw(conn *websocket.Conn, frame WsFrame) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// readLoop reads frames from conn and routes them.
func (c *WsConn) readLoop(ctx context.Context, conn *websocket.Conn, gen int, done chan struct{}, authDone chan<- error) {
	defer close(done)
	authSignaled := false
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			manual := c.isManualClose
			c.mu.Unlock()
			if !manual {
				slog.Warn("wecom ws read error", "component", "wecom", "error", err)
			}
			if !authSignaled {
				authDone <- fmt.Errorf("read error before auth: %w", err)
			}
			return
		}
		var frame WsFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			slog.Warn("wecom ws parse error", "component", "wecom", "error", err)
			continue
		}
		c.handleFrame(frame, authDone, &authSignaled)
	}
}

// handleFrame routes a received frame to the correct handler.
func (c *WsConn) handleFrame(frame WsFrame, authDone chan<- error, authSignaled *bool) {
	reqID := frame.Headers.ReqID

	switch frame.Cmd {
	case WsCmdCallback:
		slog.Info("wecom message received", "component", "wecom", "reqId", reqID)
		if c.OnMessage != nil {
			c.OnMessage(frame)
		}
		return
	case WsCmdEventCallback:
		// Event callbacks (enter_chat, template_card_event, etc.) are out of scope.
		slog.Debug("wecom event callback dropped (out of scope)", "component", "wecom", "reqId", reqID)
		return
	}

	// No cmd = ack frame; identify by req_id prefix.
	switch {
	case strings.HasPrefix(reqID, WsCmdSubscribe):
		if !*authSignaled {
			*authSignaled = true
			if frame.ErrCode != 0 {
				authDone <- fmt.Errorf("errcode=%d msg=%s", frame.ErrCode, frame.ErrMsg)
				return
			}
			slog.Info("wecom authenticated", "component", "wecom")
			if c.OnAuthenticated != nil {
				c.OnAuthenticated()
			}
			authDone <- nil
		}
	case strings.HasPrefix(reqID, WsCmdHeartbeat):
		if frame.ErrCode != 0 {
			slog.Warn("wecom heartbeat ack error", "component", "wecom", "errcode", frame.ErrCode)
			return
		}
		c.mu.Lock()
		c.missedPong = 0
		c.mu.Unlock()
		slog.Debug("wecom heartbeat ack", "component", "wecom")
	default:
		c.releaseReplyAck(reqID, frame)
	}
}

// heartbeatLoop sends a ping every HeartbeatInterval.
// If wsMaxMissedPong consecutive pings are unanswered, it force-closes the connection.
func (c *WsConn) heartbeatLoop(ctx context.Context, conn *websocket.Conn, gen int) {
	ticker := time.NewTicker(c.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			if c.connGeneration != gen {
				c.mu.Unlock()
				return
			}
			if c.missedPong >= wsMaxMissedPong {
				c.mu.Unlock()
				slog.Warn("wecom heartbeat timeout, force-closing", "component", "wecom", "missed", c.missedPong)
				conn.Close()
				return
			}
			c.missedPong++
			c.mu.Unlock()

			reqID := generateReqID(WsCmdHeartbeat)
			if err := c.sendRaw(conn, WsFrame{
				Cmd:     WsCmdHeartbeat,
				Headers: WsFrameHeaders{ReqID: reqID},
			}); err != nil {
				slog.Warn("wecom heartbeat send failed", "component", "wecom", "error", err)
				conn.Close()
				return
			}
		}
	}
}

// watchDisconnect triggers reconnect when readLoop exits unexpectedly.
func (c *WsConn) watchDisconnect(ctx context.Context, readDone <-chan struct{}) {
	select {
	case <-ctx.Done():
		return
	case <-readDone:
		c.mu.Lock()
		manual := c.isManualClose
		c.mu.Unlock()
		if !manual {
			c.scheduleReconnect(ctx)
		}
	}
}

// scheduleReconnect waits with exponential backoff and re-dials.
func (c *WsConn) scheduleReconnect(ctx context.Context) {
	c.mu.Lock()
	c.reconnectCount++
	attempt := c.reconnectCount
	c.mu.Unlock()

	if c.cfg.MaxReconnectAttempts != -1 && attempt > c.cfg.MaxReconnectAttempts {
		slog.Error("wecom max reconnect attempts exceeded", "component", "wecom")
		return
	}

	delay := time.Duration(float64(c.cfg.ReconnectBaseDelay) * math.Pow(2, float64(attempt-1)))
	if delay > wsDefaultReconnectMax {
		delay = wsDefaultReconnectMax
	}
	slog.Info("wecom reconnecting", "component", "wecom", "attempt", attempt, "delay", delay)

	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}

	if err := c.dialAndAuth(ctx); err != nil {
		slog.Error("wecom reconnect failed", "component", "wecom", "attempt", attempt, "error", err)
		c.scheduleReconnect(ctx)
	} else {
		slog.Info("wecom reconnected", "component", "wecom", "attempt", attempt)
		c.mu.Lock()
		c.reconnectCount = 0
		c.mu.Unlock()
	}
}

// SendReply enqueues a frame for the given reqID and waits for the server ack.
// Frames with the same reqID are serialised; different reqIDs are independent.
// Each distinct operation (subscribe, heartbeat, each upload step, each reply)
// MUST use its own freshly generated reqID — reusing a reqID across operations
// would serialize them through the same queue slot and deadlock.
func (c *WsConn) SendReply(reqID, cmd string, body any) (WsFrame, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return WsFrame{}, fmt.Errorf("marshal body: %w", err)
	}
	frame := WsFrame{
		Cmd:     cmd,
		Headers: WsFrameHeaders{ReqID: reqID},
		Body:    bodyJSON,
	}
	entry := &replyEntry{
		frame: frame,
		done:  make(chan struct{}),
	}

	c.queueMu.Lock()
	c.queues[reqID] = append(c.queues[reqID], entry)
	start := len(c.queues[reqID]) == 1
	c.queueMu.Unlock()

	if start {
		go c.processQueue(reqID)
	}

	<-entry.done
	return entry.resp, entry.err
}

// processQueue drains the queue for reqID, sending one frame at a time and
// waiting for ack before proceeding.
func (c *WsConn) processQueue(reqID string) {
	for {
		c.queueMu.Lock()
		queue := c.queues[reqID]
		if len(queue) == 0 {
			delete(c.queues, reqID)
			c.queueMu.Unlock()
			return
		}
		entry := queue[0]
		c.queueMu.Unlock()

		// Send the frame.
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			entry.err = fmt.Errorf("not connected")
			close(entry.done)
			c.queueMu.Lock()
			c.queues[reqID] = c.queues[reqID][1:]
			c.queueMu.Unlock()
			continue
		}
		if err := c.sendRaw(conn, entry.frame); err != nil {
			entry.err = err
			close(entry.done)
			c.queueMu.Lock()
			c.queues[reqID] = c.queues[reqID][1:]
			c.queueMu.Unlock()
			continue
		}

		// Register ack listener.
		c.queueMu.Lock()
		c.pending[reqID] = entry
		c.queueMu.Unlock()

		// Wait for ack or timeout.
		select {
		case <-entry.done:
			// ack arrived via releaseReplyAck
		case <-time.After(c.cfg.ReplyAckTimeout):
			c.queueMu.Lock()
			delete(c.pending, reqID)
			c.queueMu.Unlock()
			entry.err = fmt.Errorf("ack timeout (5s) for reqID %s", reqID)
			close(entry.done)
		}

		c.queueMu.Lock()
		c.queues[reqID] = c.queues[reqID][1:]
		c.queueMu.Unlock()
	}
}

// releaseReplyAck resolves the pending ack for reqID with the received frame.
func (c *WsConn) releaseReplyAck(reqID string, frame WsFrame) {
	c.queueMu.Lock()
	entry, ok := c.pending[reqID]
	if ok {
		delete(c.pending, reqID)
	}
	c.queueMu.Unlock()
	if !ok {
		slog.Debug("wecom unexpected ack (ignored)", "component", "wecom", "reqId", reqID)
		return
	}
	if frame.ErrCode != 0 {
		entry.err = fmt.Errorf("ack error: code=%d msg=%s", frame.ErrCode, frame.ErrMsg)
	} else {
		entry.resp = frame
	}
	close(entry.done)
}

// IsConnected reports whether the WebSocket connection is currently open.
func (c *WsConn) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}
```

- [ ] **Step 3.4: Run tests — all pass**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... -v -run TestNewWsConn
```
Expected: 2 tests PASS.

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... -v -run TestGenerateReqID
```
Expected: 2 tests PASS.

- [ ] **Step 3.5: Full package compile check**

```bash
cd /Users/tengteng/work/robobee/core && go build ./internal/platform/wecom/...
```
Expected: no errors.

- [ ] **Step 3.6: Commit**

```bash
git add internal/platform/wecom/wsconn.go internal/platform/wecom/wsconn_test.go
git commit -m "feat(wecom): add WebSocket connection manager (wsconn.go)"
```

---

## Chunk 3: Handler — Receiver

### Task 4: `handler.go` skeleton, message body types, and text/voice handling

**Files:**
- Create: `internal/platform/wecom/handler.go`
- Create: `internal/platform/wecom/handler_test.go`

**Background:** `WeComReceiver.Start` sets up `wsConn.OnMessage` to call `processMessage` in a goroutine, then calls `wsConn.Connect(ctx)` which blocks until ctx is cancelled. `processMessage` extracts content from the WeCom frame, sends a `<think></think>` thinking indicator, and dispatches the inbound message.

- [ ] **Step 4.1: Write tests for text and voice message processing**

Create `internal/platform/wecom/handler_test.go`:

```go
package wecom

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/media"
	"github.com/theopenbee/openbee/internal/platform"
)

// mockWsConn replaces WsConn in tests — captures SendReply calls, never dials.
type mockWsConn struct {
	mu      sync.Mutex
	replies []sentReply
}

type sentReply struct {
	reqID string
	cmd   string
	body  any
}

func (m *mockWsConn) sendReply(reqID, cmd string, body any) (WsFrame, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replies = append(m.replies, sentReply{reqID: reqID, cmd: cmd, body: body})
	return WsFrame{}, nil
}

// buildFrame constructs a minimal WsFrame with the given message body.
func buildFrame(t *testing.T, reqID string, body messageBody) WsFrame {
	t.Helper()
	bodyJSON, err := json.Marshal(body)
	require.NoError(t, err)
	return WsFrame{
		Cmd:     WsCmdCallback,
		Headers: WsFrameHeaders{ReqID: reqID},
		Body:    bodyJSON,
	}
}

func newTestReceiver(mock *mockWsConn) *WeComReceiver {
	var ps sync.Map
	r := &WeComReceiver{
		pendingStreams: &ps,
		mediaSvc:      media.NewService(),
	}
	// Inject mock send function
	r.sendReplyFn = mock.sendReply
	return r
}

func TestProcessMessage_Text(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-001", messageBody{
		MsgID:    "msg-001",
		ChatType: "single",
		From:     messageFrom{UserID: "user1"},
		MsgType:  "text",
		Text:     &textContent{Content: "hello world"},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	require.Len(t, dispatched, 1)
	msg := dispatched[0]
	assert.Equal(t, "wecom", msg.Platform)
	assert.Equal(t, "user1", msg.SenderID)
	assert.Equal(t, "wecom:user1:user1", msg.SessionKey)
	assert.Equal(t, "hello world", msg.Content)
	assert.Equal(t, "hello world", msg.RawContent)
	assert.Equal(t, "msg-001", msg.PlatformMessageID)

	// Thinking message should have been sent
	require.Len(t, mock.replies, 1)
	assert.Equal(t, "req-001", mock.replies[0].reqID)
	assert.Equal(t, WsCmdResponse, mock.replies[0].cmd)
}

func TestProcessMessage_Voice(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-002", messageBody{
		MsgID:    "msg-002",
		ChatType: "single",
		From:     messageFrom{UserID: "user2"},
		MsgType:  "voice",
		Voice:    &voiceContent{Content: "transcribed text"},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	require.Len(t, dispatched, 1)
	assert.Equal(t, "transcribed text", dispatched[0].Content)
	assert.Equal(t, "transcribed text", dispatched[0].RawContent)
}

func TestProcessMessage_GroupChat(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-003", messageBody{
		MsgID:    "msg-003",
		ChatType: "group",
		ChatID:   "group-chat-1",
		From:     messageFrom{UserID: "user3"},
		MsgType:  "text",
		Text:     &textContent{Content: "group message"},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	require.Len(t, dispatched, 1)
	assert.Equal(t, "wecom:group-chat-1:user3", dispatched[0].SessionKey)
}

func TestProcessMessage_EmptyText_Skipped(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-004", messageBody{
		MsgID:    "msg-004",
		ChatType: "single",
		From:     messageFrom{UserID: "user4"},
		MsgType:  "text",
		Text:     &textContent{Content: ""},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	assert.Empty(t, dispatched)
	assert.Empty(t, mock.replies) // no thinking message either
}

func TestProcessMessage_UnsupportedMsgType_Skipped(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-005", messageBody{
		MsgID:    "msg-005",
		ChatType: "single",
		From:     messageFrom{UserID: "user5"},
		MsgType:  "link", // unsupported
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	assert.Empty(t, dispatched)
}

func TestProcessMessage_PendingStreams(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-006", messageBody{
		MsgID:    "msg-006",
		ChatType: "single",
		From:     messageFrom{UserID: "user6"},
		MsgType:  "text",
		Text:     &textContent{Content: "hello"},
	})

	r.processMessage(frame, func(m platform.InboundMessage) {})

	// Stream ID should be stored
	val, ok := r.pendingStreams.Load("msg-006")
	assert.True(t, ok)
	assert.NotEmpty(t, val.(string))
}
```

- [ ] **Step 4.2: Run test — expect compile failure**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... 2>&1 | head -20
```
Expected: `undefined: WeComReceiver`, `undefined: messageBody`, etc.

- [ ] **Step 4.3: Implement `handler.go` skeleton with text/voice support**

Create `internal/platform/wecom/handler.go`:

```go
package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/media"
	"github.com/theopenbee/openbee/internal/platform"
)

// ─── Message body types ────────────────────────────────────────────────────

type messageBody struct {
	MsgID       string        `json:"msgid"`
	AiBotID     string        `json:"aibotid"`
	ChatID      string        `json:"chatid"`      // group only
	ChatType    string        `json:"chattype"`    // "single" | "group"
	From        messageFrom   `json:"from"`
	CreateTime  int64         `json:"create_time"` // Unix seconds
	ResponseURL string        `json:"response_url"`
	MsgType     string        `json:"msgtype"`
	Text        *textContent  `json:"text"`
	Voice       *voiceContent `json:"voice"`
	Image       *mediaContent `json:"image"`
	File        *fileContent  `json:"file"`
	Mixed       *mixedContent `json:"mixed"`
	Quote       *quoteContent `json:"quote"`
}

type messageFrom struct {
	UserID string `json:"userid"`
}

type textContent struct {
	Content string `json:"content"`
}

type voiceContent struct {
	Content string `json:"content"`
}

type mediaContent struct {
	URL    string `json:"url"`
	AesKey string `json:"aeskey"`
}

type fileContent struct {
	URL    string `json:"url"`
	AesKey string `json:"aeskey"`
}

type mixedContent struct {
	MsgItem []mixedItem `json:"msg_item"`
}

type mixedItem struct {
	MsgType string        `json:"msgtype"`
	Text    *textContent  `json:"text"`
	Image   *mediaContent `json:"image"`
}

type quoteContent struct {
	MsgType string        `json:"msgtype"`
	Text    *textContent  `json:"text"`
	Voice   *voiceContent `json:"voice"`
	Image   *mediaContent `json:"image"`
	File    *fileContent  `json:"file"`
	Mixed   *mixedContent `json:"mixed"`
}

// ─── Outbound body types ───────────────────────────────────────────────────

type streamBody struct {
	MsgType string     `json:"msgtype"`
	Stream  streamItem `json:"stream"`
}

type streamItem struct {
	ID      string `json:"id"`
	Finish  bool   `json:"finish"`
	Content string `json:"content"`
}

// sendMsgBody is the body of an aibot_send_msg frame.
// chatid must be snake_case to match the WeCom wire protocol.
type sendMsgBody struct {
	ChatID  string          `json:"chatid"`
	MsgType string          `json:"msgtype"`
	Image   *mediaIDContent `json:"image,omitempty"`
	Voice   *mediaIDContent `json:"voice,omitempty"`
	Video   *mediaIDContent `json:"video,omitempty"`
	File    *mediaIDContent `json:"file,omitempty"`
}

type mediaIDContent struct {
	MediaID string `json:"media_id"`
}

// ─── Upload body types ─────────────────────────────────────────────────────

type uploadInitBody struct {
	Type        string `json:"type"`
	Filename    string `json:"filename"`
	TotalSize   int    `json:"total_size"`
	TotalChunks int    `json:"total_chunks"`
	MD5         string `json:"md5"`
}

type uploadChunkBody struct {
	UploadID   string `json:"upload_id"`
	ChunkIndex int    `json:"chunk_index"`
	Base64Data string `json:"base64_data"`
}

type uploadFinishBody struct {
	UploadID string `json:"upload_id"`
}

// ─── sendReplyFn type ──────────────────────────────────────────────────────

// sendReplyFn matches WsConn.SendReply — injected for testing.
type sendReplyFn func(reqID, cmd string, body any) (WsFrame, error)

// ─── WeComPlatform ─────────────────────────────────────────────────────────

// WeComPlatform implements platform.Platform for WeCom AI Bot.
type WeComPlatform struct {
	receiver      *WeComReceiver
	sender        *WeComSender
	pendingStreams sync.Map // key: msgId → value: streamId
}

// NewPlatform constructs a WeComPlatform from configuration.
func NewPlatform(cfg config.WeComConfig, mediaSvc *media.Service) platform.Platform {
	wsConn := NewWsConn(WsConnConfig{
		BotID:  cfg.BotID,
		Secret: cfg.Secret,
		URL:    cfg.WebSocketURL,
	})
	p := &WeComPlatform{}
	p.receiver = &WeComReceiver{
		cfg:           cfg,
		pendingStreams: &p.pendingStreams,
		mediaSvc:      mediaSvc,
		wsConn:        wsConn,
		sendReplyFn:   wsConn.SendReply,
	}
	p.sender = &WeComSender{
		pendingStreams: &p.pendingStreams,
		wsConn:        wsConn,
		sendReplyFn:   wsConn.SendReply,
	}
	return p
}

func (p *WeComPlatform) ID() string                                 { return "wecom" }
func (p *WeComPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *WeComPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }

// ─── WeComReceiver ─────────────────────────────────────────────────────────

// WeComReceiver connects to WeCom via WebSocket and dispatches inbound messages.
type WeComReceiver struct {
	cfg           config.WeComConfig
	pendingStreams *sync.Map
	mediaSvc      *media.Service
	wsConn        *WsConn
	sendReplyFn   sendReplyFn // injectable for testing
}

// Start begins receiving messages and blocks until ctx is cancelled.
func (r *WeComReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	r.wsConn.OnMessage = func(frame WsFrame) {
		go r.processMessage(frame, dispatch)
	}
	slog.Info("WeCom bot starting", "component", "wecom")
	return r.wsConn.Connect(ctx)
}

// processMessage extracts content from a callback frame, sends the thinking
// indicator, stores the stream ID, and dispatches the inbound message.
func (r *WeComReceiver) processMessage(frame WsFrame, dispatch func(platform.InboundMessage)) {
	var body messageBody
	if err := json.Unmarshal(frame.Body, &body); err != nil {
		slog.Warn("wecom: failed to parse message body", "component", "wecom", "error", err)
		return
	}

	chatID := body.From.UserID
	if body.ChatType == "group" {
		chatID = body.ChatID
	}
	senderID := body.From.UserID

	rawText, content := r.extractContent(context.Background(), &body)
	if content == "" {
		return
	}

	// Send thinking indicator.
	streamID := generateReqID("stream")
	thinking := streamBody{
		MsgType: "stream",
		Stream:  streamItem{ID: streamID, Finish: false, Content: "<think></think>"},
	}
	if _, err := r.sendReplyFn(frame.Headers.ReqID, WsCmdResponse, thinking); err != nil {
		slog.Warn("wecom: failed to send thinking message", "component", "wecom", "error", err)
	}

	// Store stream ID with TTL cleanup to prevent leaks when the downstream
	// pipeline drops a message without ever calling Sender.Send.
	r.pendingStreams.Store(body.MsgID, streamID)
	time.AfterFunc(10*time.Minute, func() { r.pendingStreams.Delete(body.MsgID) })

	rawBytes, _ := json.Marshal(frame)
	dispatch(platform.InboundMessage{
		Platform:          "wecom",
		SenderID:          senderID,
		SessionKey:        "wecom:" + chatID + ":" + senderID,
		Content:           content,
		RawContent:        rawText,
		Raw:               string(rawBytes),
		PlatformMessageID: body.MsgID,
		MessageTime:       body.CreateTime * 1000, // WeCom create_time is Unix seconds
	})
}

// extractContent determines rawText and content from the message body.
// rawText is plain text only (no media placeholders); content may include placeholders.
func (r *WeComReceiver) extractContent(ctx context.Context, body *messageBody) (rawText, content string) {
	switch body.MsgType {
	case "text":
		if body.Text == nil {
			return "", ""
		}
		rawText = body.Text.Content
		content = rawText

	case "voice":
		if body.Voice == nil {
			return "", ""
		}
		rawText = body.Voice.Content
		content = rawText

	case "image":
		if body.Image == nil {
			return "", r.mediaSvc.BuildPlaceholder("image", "", "")
		}
		content = r.downloadDecryptSave(ctx, body.Image.URL, body.Image.AesKey, "image", "")

	case "file":
		if body.File == nil {
			return "", r.mediaSvc.BuildPlaceholder("document", "", "")
		}
		content = r.downloadDecryptSave(ctx, body.File.URL, body.File.AesKey, "document", "")

	case "mixed":
		if body.Mixed == nil {
			return "", ""
		}
		rawText, content = r.extractMixedContent(ctx, body.Mixed.MsgItem)

	default:
		slog.Warn("wecom: skipping unsupported msgtype", "component", "wecom", "msgtype", body.MsgType)
		return "", ""
	}

	// Append quoted content if present.
	if body.Quote != nil {
		qContent := r.extractQuoteContent(ctx, body.Quote)
		if qContent != "" {
			if content != "" {
				content = content + "\n" + qContent
			} else {
				content = qContent
			}
		}
	}

	return rawText, content
}

// extractMixedContent processes a mixed message's items in parallel for images.
// rawText = joined plain text parts; content = text + image placeholders in order.
func (r *WeComReceiver) extractMixedContent(ctx context.Context, items []mixedItem) (rawText, content string) {
	type result struct {
		text  string
		image string
	}
	results := make([]result, len(items))

	g, gCtx := errgroup.WithContext(ctx)
	for i, item := range items {
		i, item := i, item
		switch item.MsgType {
		case "text":
			if item.Text != nil {
				results[i].text = item.Text.Content
			}
		case "image":
			if item.Image != nil {
				url, key := item.Image.URL, item.Image.AesKey
				g.Go(func() error {
					results[i].image = r.downloadDecryptSave(gCtx, url, key, "image", "")
					return nil
				})
			}
		}
	}
	_ = g.Wait()

	var textParts, allParts []string
	for _, res := range results {
		if res.text != "" {
			textParts = append(textParts, res.text)
			allParts = append(allParts, res.text)
		}
		if res.image != "" {
			allParts = append(allParts, res.image)
		}
	}
	return strings.Join(textParts, "\n"), strings.Join(allParts, "\n")
}

// extractQuoteContent extracts displayable content from a quote block.
func (r *WeComReceiver) extractQuoteContent(ctx context.Context, q *quoteContent) string {
	if q == nil {
		return ""
	}
	switch q.MsgType {
	case "text":
		if q.Text != nil {
			return q.Text.Content
		}
	case "voice":
		if q.Voice != nil {
			return q.Voice.Content
		}
	case "image":
		if q.Image != nil {
			return r.downloadDecryptSave(ctx, q.Image.URL, q.Image.AesKey, "image", "")
		}
	case "file":
		if q.File != nil {
			return r.downloadDecryptSave(ctx, q.File.URL, q.File.AesKey, "document", "")
		}
	case "mixed":
		if q.Mixed != nil {
			_, content := r.extractMixedContent(ctx, q.Mixed.MsgItem)
			return content
		}
	}
	return ""
}

// downloadDecryptSave downloads an encrypted WeCom media file, decrypts it,
// saves it via mediaSvc, and returns a placeholder string.
// On any error it logs and returns a placeholder without a path.
func (r *WeComReceiver) downloadDecryptSave(ctx context.Context, url, aesKey, mediaType, filename string) string {
	if url == "" {
		return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		slog.Error("wecom: create download request failed", "component", "wecom", "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("wecom: download media failed", "component", "wecom", "url", url, "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("wecom: read media body failed", "component", "wecom", "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
	}

	if aesKey != "" {
		data, err = DecryptFile(data, aesKey)
		if err != nil {
			slog.Error("wecom: decrypt media failed", "component", "wecom", "error", err)
			return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
		}
	}

	mime := r.mediaSvc.DetectMIME(data, filename)
	ext := r.mediaSvc.ExtensionFromMIME(mime)
	if ext == "" && filename != "" {
		ext = filepath.Ext(filename)
	}

	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		slog.Error("wecom: save media failed", "component", "wecom", "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
	}
	return r.mediaSvc.BuildPlaceholder(mediaType, path, filename)
}

// ─── WeComSender ───────────────────────────────────────────────────────────

// WeComSender sends replies via the WeCom AI Bot WebSocket.
type WeComSender struct {
	pendingStreams *sync.Map
	wsConn        *WsConn
	sendReplyFn   sendReplyFn
}

// Send delivers a reply to WeCom. Text replies use aibot_respond_msg streaming;
// media replies use 3-step upload + aibot_send_msg.
func (s *WeComSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var frame WsFrame
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &frame); err != nil {
		return fmt.Errorf("wecom sender: unmarshal raw frame: %w", err)
	}
	var body messageBody
	if err := json.Unmarshal(frame.Body, &body); err != nil {
		return fmt.Errorf("wecom sender: unmarshal message body: %w", err)
	}

	reqID := frame.Headers.ReqID
	chatID := body.From.UserID
	if body.ChatType == "group" {
		chatID = body.ChatID
	}
	msgID := body.MsgID

	streamID := generateReqID("stream") // fallback
	if val, ok := s.pendingStreams.LoadAndDelete(msgID); ok {
		streamID = val.(string)
	}

	if msg.MediaPath != "" {
		return s.sendMedia(ctx, msg.MediaPath, chatID, reqID, streamID)
	}
	return s.sendText(ctx, msg.Content, reqID, streamID)
}

// sendText sends a streaming text reply.
func (s *WeComSender) sendText(_ context.Context, content, reqID, streamID string) error {
	body := streamBody{
		MsgType: "stream",
		Stream:  streamItem{ID: streamID, Finish: true, Content: content},
	}
	_, err := s.sendReplyFn(reqID, WsCmdResponse, body)
	return err
}

// sendMedia uploads a media file and sends it via aibot_send_msg.
func (s *WeComSender) sendMedia(ctx context.Context, mediaPath, chatID, reqID, streamID string) error {
	// TODO: implemented in Task 5
	_ = ctx
	slog.Warn("wecom: sendMedia not yet implemented", "component", "wecom", "path", mediaPath)
	// Finish thinking stream with a placeholder so the user sees something.
	return s.finishStream(reqID, streamID, "⚠️ 媒体发送暂不支持。")
}

// finishStream closes the thinking stream with the given text.
func (s *WeComSender) finishStream(reqID, streamID, text string) error {
	body := streamBody{
		MsgType: "stream",
		Stream:  streamItem{ID: streamID, Finish: true, Content: text},
	}
	_, err := s.sendReplyFn(reqID, WsCmdResponse, body)
	return err
}

// ─── Compile-time interface assertions ─────────────────────────────────────

var _ platform.Platform                = (*WeComPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*WeComReceiver)(nil)
var _ platform.PlatformSenderAdapter   = (*WeComSender)(nil)
```

- [ ] **Step 4.4: Run handler tests — all pass**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... -v -run TestProcessMessage
```
Expected: 6 tests PASS.

- [ ] **Step 4.5: Full build check**

```bash
cd /Users/tengteng/work/robobee/core && go build ./...
```
Expected: no errors.

- [ ] **Step 4.6: Commit**

```bash
git add internal/platform/wecom/handler.go internal/platform/wecom/handler_test.go
git commit -m "feat(wecom): add handler skeleton with text/voice receiver and text sender"
```

---

### Task 5: Handler — inbound media tests (image, file, mixed, quote)

**Files:**
- Modify: `internal/platform/wecom/handler_test.go`

The media-downloading parts of `processMessage` call `downloadDecryptSave`, which makes real HTTP requests. We verify that:
1. When the URL is empty the placeholder is returned without a path.
2. The content field is non-empty for each media type.
3. Quote content is appended correctly.

We mock `downloadDecryptSave` by providing a test receiver that overrides it.

- [ ] **Step 5.1: Add media and quote tests to `handler_test.go`**

Add to `handler_test.go`:

```go
// newTestReceiverWithDownload returns a receiver whose downloadDecryptSave
// always returns a predictable placeholder (no real HTTP).
func newTestReceiverWithDownload(mock *mockWsConn) *WeComReceiver {
	r := newTestReceiver(mock)
	r.downloadFn = func(_ context.Context, url, _, mediaType, filename string) string {
		if url == "" {
			return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
		}
		return r.mediaSvc.BuildPlaceholder(mediaType, "/tmp/fake-"+mediaType, filename)
	}
	return r
}

func TestProcessMessage_Image(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiverWithDownload(mock)

	frame := buildFrame(t, "req-010", messageBody{
		MsgID:    "msg-010",
		ChatType: "single",
		From:     messageFrom{UserID: "u1"},
		MsgType:  "image",
		Image:    &mediaContent{URL: "https://example.com/img.jpg", AesKey: "key1"},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	require.Len(t, dispatched, 1)
	assert.Contains(t, dispatched[0].Content, "image")
	assert.Equal(t, "", dispatched[0].RawContent)
}

func TestProcessMessage_File(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiverWithDownload(mock)

	frame := buildFrame(t, "req-011", messageBody{
		MsgID:    "msg-011",
		ChatType: "single",
		From:     messageFrom{UserID: "u1"},
		MsgType:  "file",
		File:     &fileContent{URL: "https://example.com/doc.pdf", AesKey: "key2"},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	require.Len(t, dispatched, 1)
	assert.Contains(t, dispatched[0].Content, "document")
}

func TestProcessMessage_Mixed(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiverWithDownload(mock)

	frame := buildFrame(t, "req-012", messageBody{
		MsgID:    "msg-012",
		ChatType: "single",
		From:     messageFrom{UserID: "u1"},
		MsgType:  "mixed",
		Mixed: &mixedContent{MsgItem: []mixedItem{
			{MsgType: "text", Text: &textContent{Content: "look at this:"}},
			{MsgType: "image", Image: &mediaContent{URL: "https://example.com/x.png", AesKey: "key3"}},
		}},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	require.Len(t, dispatched, 1)
	assert.Contains(t, dispatched[0].Content, "look at this:")
	assert.Contains(t, dispatched[0].Content, "image")
	assert.Equal(t, "look at this:", dispatched[0].RawContent)
}

func TestProcessMessage_TextWithQuote(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiverWithDownload(mock)

	frame := buildFrame(t, "req-013", messageBody{
		MsgID:    "msg-013",
		ChatType: "single",
		From:     messageFrom{UserID: "u1"},
		MsgType:  "text",
		Text:     &textContent{Content: "my reply"},
		Quote: &quoteContent{
			MsgType: "text",
			Text:    &textContent{Content: "quoted original"},
		},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	require.Len(t, dispatched, 1)
	assert.Contains(t, dispatched[0].Content, "my reply")
	assert.Contains(t, dispatched[0].Content, "quoted original")
}

func TestProcessMessage_QuoteFile(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiverWithDownload(mock)

	frame := buildFrame(t, "req-014", messageBody{
		MsgID:    "msg-014",
		ChatType: "single",
		From:     messageFrom{UserID: "u1"},
		MsgType:  "text",
		Text:     &textContent{Content: "see attached"},
		Quote: &quoteContent{
			MsgType: "file",
			File:    &fileContent{URL: "https://example.com/q.pdf", AesKey: "key4"},
		},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	require.Len(t, dispatched, 1)
	assert.Contains(t, dispatched[0].Content, "see attached")
	assert.Contains(t, dispatched[0].Content, "document")
}
```

- [ ] **Step 5.2: Run tests — expect compile failure (downloadFn field undefined)**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... 2>&1 | head -20
```
Expected: `unknown field downloadFn`

- [ ] **Step 5.3: Add `downloadFn` field and wire it in `handler.go`**

In `handler.go`, add to `WeComReceiver`:
```go
// downloadFn is injectable for testing; defaults to downloadDecryptSave.
downloadFn func(ctx context.Context, url, aesKey, mediaType, filename string) string
```

Replace the three direct calls to `r.downloadDecryptSave(...)` in `extractContent`, `extractMixedContent`, `extractQuoteContent` with:
```go
r.download(ctx, url, aesKey, mediaType, filename)
```

Add helper:
```go
func (r *WeComReceiver) download(ctx context.Context, url, aesKey, mediaType, filename string) string {
	if r.downloadFn != nil {
		return r.downloadFn(ctx, url, aesKey, mediaType, filename)
	}
	return r.downloadDecryptSave(ctx, url, aesKey, mediaType, filename)
}
```

- [ ] **Step 5.4: Run all handler tests**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... -v -run TestProcessMessage
```
Expected: 11 tests PASS.

- [ ] **Step 5.5: Commit**

```bash
git add internal/platform/wecom/handler.go internal/platform/wecom/handler_test.go
git commit -m "feat(wecom): add inbound media handling (image, file, mixed, quote)"
```

---

## Chunk 4: Sender Media + App Wiring

### Task 6: Complete `WeComSender` with media upload

**Files:**
- Modify: `internal/platform/wecom/handler.go`
- Modify: `internal/platform/wecom/handler_test.go`

**Background:** Media upload is 3 steps over WebSocket: init → sequential chunks (base64, 512KB each) → finish. Then send via `aibot_send_msg`. Each step uses a fresh `req_id`. Sequential chunk upload is intentional — the WeCom backend returns system errors under high concurrency.

- [ ] **Step 6.1: Add sender tests**

Add to `handler_test.go`:

```go
// mockSendReply records all SendReply calls and returns a configurable response.
type mockSendReply struct {
	mu       sync.Mutex
	calls    []sentReply
	response map[string]WsFrame // cmd → response frame to return
}

func (m *mockSendReply) fn(reqID, cmd string, body any) (WsFrame, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, sentReply{reqID: reqID, cmd: cmd, body: body})
	if resp, ok := m.response[cmd]; ok {
		return resp, nil
	}
	return WsFrame{}, nil
}

func buildUploadInitResponse(uploadID string) WsFrame {
	body, _ := json.Marshal(map[string]string{"upload_id": uploadID})
	return WsFrame{Body: body}
}

func buildUploadFinishResponse(mediaID string) WsFrame {
	body, _ := json.Marshal(map[string]string{"media_id": mediaID})
	return WsFrame{Body: body}
}

func TestSend_TextReply(t *testing.T) {
	mock := &mockSendReply{}
	var ps sync.Map
	s := &WeComSender{pendingStreams: &ps, sendReplyFn: mock.fn}

	// Pre-store stream ID
	ps.Store("msg-100", "stream-abc")

	raw := buildRawFrame(t, "req-100", messageBody{
		MsgID: "msg-100", ChatType: "single", From: messageFrom{UserID: "u1"},
	})
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hello back",
		ReplyTo: platform.InboundMessage{Raw: raw},
	})
	require.NoError(t, err)

	require.Len(t, mock.calls, 1)
	assert.Equal(t, WsCmdResponse, mock.calls[0].cmd)
	assert.Equal(t, "req-100", mock.calls[0].reqID)

	body := mock.calls[0].body.(streamBody)
	assert.True(t, body.Stream.Finish)
	assert.Equal(t, "hello back", body.Stream.Content)
	assert.Equal(t, "stream-abc", body.Stream.ID)
}

func TestSend_MediaUpload(t *testing.T) {
	// Create a small temp file
	tmpFile := t.TempDir() + "/test.png"
	require.NoError(t, os.WriteFile(tmpFile, make([]byte, 100), 0600))

	mock := &mockSendReply{
		response: map[string]WsFrame{
			WsCmdUploadMediaInit:   buildUploadInitResponse("upload-xyz"),
			WsCmdUploadMediaFinish: buildUploadFinishResponse("media-xyz"),
		},
	}
	var ps sync.Map
	ps.Store("msg-200", "stream-200")
	s := &WeComSender{pendingStreams: &ps, sendReplyFn: mock.fn}

	raw := buildRawFrame(t, "req-200", messageBody{
		MsgID: "msg-200", ChatType: "single", From: messageFrom{UserID: "u1"},
	})
	err := s.Send(context.Background(), platform.OutboundMessage{
		MediaPath: tmpFile,
		ReplyTo:   platform.InboundMessage{Raw: raw},
	})
	require.NoError(t, err)

	cmds := make([]string, len(mock.calls))
	for i, c := range mock.calls {
		cmds[i] = c.cmd
	}
	// Expected: init, chunk(s), finish, send_msg, respond_msg (thinking finish)
	assert.Contains(t, cmds, WsCmdUploadMediaInit)
	assert.Contains(t, cmds, WsCmdUploadMediaChunk)
	assert.Contains(t, cmds, WsCmdUploadMediaFinish)
	assert.Contains(t, cmds, WsCmdSendMsg)
	assert.Contains(t, cmds, WsCmdResponse)
}

func TestSend_FileTooLarge(t *testing.T) {
	var ps sync.Map
	ps.Store("msg-300", "stream-300")
	mock := &mockSendReply{}
	s := &WeComSender{pendingStreams: &ps, sendReplyFn: mock.fn}

	// Create a file reported as >20MB by injecting oversize bytes
	tmpFile := t.TempDir() + "/big.bin"
	// Write 1 byte — we'll override the size check via a helper below.
	// Instead, create exactly 20MB+1 byte.
	data := make([]byte, 20*1024*1024+1)
	require.NoError(t, os.WriteFile(tmpFile, data, 0600))

	raw := buildRawFrame(t, "req-300", messageBody{
		MsgID: "msg-300", ChatType: "single", From: messageFrom{UserID: "u1"},
	})
	err := s.Send(context.Background(), platform.OutboundMessage{
		MediaPath: tmpFile,
		ReplyTo:   platform.InboundMessage{Raw: raw},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

// buildRawFrame creates the Raw string for OutboundMessage.ReplyTo.
func buildRawFrame(t *testing.T, reqID string, body messageBody) string {
	t.Helper()
	bodyJSON, _ := json.Marshal(body)
	frame := WsFrame{
		Cmd:     WsCmdCallback,
		Headers: WsFrameHeaders{ReqID: reqID},
		Body:    bodyJSON,
	}
	raw, _ := json.Marshal(frame)
	return string(raw)
}
```

Also add `"os"` to imports in `handler_test.go`.

- [ ] **Step 6.2: Run tests — expect compile failure**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... 2>&1 | head -20
```

- [ ] **Step 6.3: Implement `sendMedia` in `handler.go`**

Replace the stub `sendMedia` with the full implementation:

```go
const (
	wecomChunkSize   = 512 * 1024       // 512 KB per chunk
	wecomMaxImage    = 10 * 1024 * 1024 // 10 MB
	wecomMaxVideo    = 10 * 1024 * 1024
	wecomMaxVoice    = 2 * 1024 * 1024  // 2 MB
	wecomMaxFile     = 20 * 1024 * 1024 // 20 MB
)

// sendMedia handles the 3-step chunked upload and aibot_send_msg dispatch.
func (s *WeComSender) sendMedia(ctx context.Context, mediaPath, chatID, reqID, streamID string) error {
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("wecom: read media file: %w", err)
	}

	mime := media.DetectMIMEFromBytes(data, filepath.Base(mediaPath))
	wecomType, err := resolveWeComMediaType(mime, len(data))
	if err != nil {
		return err
	}

	mediaID, err := s.uploadMedia(ctx, data, wecomType, filepath.Base(mediaPath))
	if err != nil {
		return err
	}

	// Build send body — chatid is snake_case (WeCom wire format).
	sendBody := sendMsgBody{ChatID: chatID, MsgType: wecomType}
	mc := &mediaIDContent{MediaID: mediaID}
	switch wecomType {
	case "image":
		sendBody.Image = mc
	case "voice":
		sendBody.Voice = mc
	case "video":
		sendBody.Video = mc
	default:
		sendBody.File = mc
	}
	if _, err := s.sendReplyFn(generateReqID(WsCmdSendMsg), WsCmdSendMsg, sendBody); err != nil {
		return fmt.Errorf("wecom: send media message: %w", err)
	}

	return s.finishStream(reqID, streamID, "📎 文件已发送，请查收。")
}

// uploadMedia executes the 3-step WeCom chunked upload protocol.
// Each step uses a distinct req_id to avoid reply-queue serialisation issues.
func (s *WeComSender) uploadMedia(ctx context.Context, data []byte, mediaType, filename string) (string, error) {
	totalSize := len(data)
	totalChunks := (totalSize + wecomChunkSize - 1) / wecomChunkSize

	md5sum := fmt.Sprintf("%x", md5.Sum(data))

	// Step 1: init
	initResp, err := s.sendReplyFn(
		generateReqID(WsCmdUploadMediaInit),
		WsCmdUploadMediaInit,
		uploadInitBody{
			Type:        mediaType,
			Filename:    platform.SanitizeFileName(filename),
			TotalSize:   totalSize,
			TotalChunks: totalChunks,
			MD5:         md5sum,
		},
	)
	if err != nil {
		return "", fmt.Errorf("wecom: upload init: %w", err)
	}
	var initResult struct {
		UploadID string `json:"upload_id"`
	}
	if err := json.Unmarshal(initResp.Body, &initResult); err != nil || initResult.UploadID == "" {
		return "", fmt.Errorf("wecom: upload init: no upload_id in response")
	}
	uploadID := initResult.UploadID

	// Step 2: sequential chunks (intentionally not parallel — WeCom backend
	// returns system errors under high concurrency; see aibot-node-sdk client.ts).
	for i := 0; i < totalChunks; i++ {
		start := i * wecomChunkSize
		end := start + wecomChunkSize
		if end > totalSize {
			end = totalSize
		}
		chunk := data[start:end]
		b64 := base64.StdEncoding.EncodeToString(chunk)

		if _, err := s.sendReplyFn(
			generateReqID(WsCmdUploadMediaChunk),
			WsCmdUploadMediaChunk,
			uploadChunkBody{UploadID: uploadID, ChunkIndex: i, Base64Data: b64},
		); err != nil {
			return "", fmt.Errorf("wecom: upload chunk %d: %w", i, err)
		}
	}

	// Step 3: finish
	finishResp, err := s.sendReplyFn(
		generateReqID(WsCmdUploadMediaFinish),
		WsCmdUploadMediaFinish,
		uploadFinishBody{UploadID: uploadID},
	)
	if err != nil {
		return "", fmt.Errorf("wecom: upload finish: %w", err)
	}
	var finishResult struct {
		MediaID string `json:"media_id"`
	}
	if err := json.Unmarshal(finishResp.Body, &finishResult); err != nil || finishResult.MediaID == "" {
		return "", fmt.Errorf("wecom: upload finish: no media_id in response")
	}
	return finishResult.MediaID, nil
}

// resolveWeComMediaType maps MIME type to WeCom media type and enforces size limits.
// Downgrades image/video >10MB and non-AMR voice to "file".
// Returns error for files >20MB.
func resolveWeComMediaType(mime string, size int) (string, error) {
	switch {
	case strings.HasPrefix(mime, "image/"):
		if size > wecomMaxImage {
			return "file", nil // downgrade
		}
		return "image", nil
	case strings.HasPrefix(mime, "video/"):
		if size > wecomMaxVideo {
			return "file", nil // downgrade
		}
		return "video", nil
	case mime == "audio/amr":
		if size > wecomMaxVoice {
			return "file", nil // downgrade
		}
		return "voice", nil
	case strings.HasPrefix(mime, "audio/"):
		return "file", nil // non-AMR audio not supported as voice
	default:
		if size > wecomMaxFile {
			return "", fmt.Errorf("wecom: file too large: %d bytes (max %d)", size, wecomMaxFile)
		}
		return "file", nil
	}
}
```

Add necessary imports to `handler.go`: `"crypto/md5"`, `"encoding/base64"`, `"os"`, `"path/filepath"`.

Also add a `DetectMIMEFromBytes` helper to `internal/media` if it doesn't exist, or use `mediaSvc.DetectMIME` — check what exists. If `media.DetectMIMEFromBytes` doesn't exist as a package-level function, use `mediaSvc.DetectMIME(data, filename)` from a `WeComSender` field. Add `mediaSvc *media.Service` to `WeComSender` and pass it from `NewPlatform`.

- [ ] **Step 6.4: Fix imports and run tests**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... -v -run TestSend
```
Expected: all 3 sender tests PASS.

- [ ] **Step 6.5: Run all wecom tests**

```bash
cd /Users/tengteng/work/robobee/core && go test ./internal/platform/wecom/... -v
```
Expected: all tests PASS.

- [ ] **Step 6.6: Commit**

```bash
git add internal/platform/wecom/handler.go internal/platform/wecom/handler_test.go
git commit -m "feat(wecom): complete media upload sender with size limits and downgrade rules"
```

---

### Task 7: Wire WeCom into `app.go`

**Files:**
- Modify: `cmd/server/app.go`

- [ ] **Step 7.1: Update `buildPlatforms` signature and call site**

In `cmd/server/app.go`, change `buildPlatforms`:

```go
func buildPlatforms(fc config.FeishuConfig, dc config.DingTalkConfig, wc config.WeComConfig, mc config.MediaConfig) []platform.Platform {
	mediaSvc := media.NewService()
	var result []platform.Platform
	if fc.Enabled {
		result = append(result, feishu.NewPlatform(fc, mediaSvc))
	}
	if dc.Enabled {
		result = append(result, dingtalk.NewPlatform(dc, mc, mediaSvc))
	}
	if wc.Enabled {
		result = append(result, wecom.NewPlatform(wc, mediaSvc))
	}
	return result
}
```

Update the call site in `buildApp` (~line 97):
```go
platforms := buildPlatforms(cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom, cfg.Bee.Media)
```

Add import at top of file:
```go
"github.com/theopenbee/openbee/internal/platform/wecom"
```

- [ ] **Step 7.2: Full build**

```bash
cd /Users/tengteng/work/robobee/core && go build ./...
```
Expected: no errors.

- [ ] **Step 7.3: Run all tests**

```bash
cd /Users/tengteng/work/robobee/core && go test ./... 2>&1 | tail -20
```
Expected: all existing tests pass, no regressions.

- [ ] **Step 7.4: Commit**

```bash
git add cmd/server/app.go
git commit -m "feat(wecom): wire WeCom platform into server app"
```

---

## Done

After Task 7 completes:
- `go build ./...` passes
- `go test ./...` passes
- `config.yaml` with `wecom.enabled: true`, a valid `bot_id`, and `secret` will start a WeCom bot
- WeCom messages are received, processed with `<think></think>` thinking indicator, and replied with the AI response
- All message types (text, voice, image, file, mixed, quote) are handled
- Media upload (image, voice/AMR, video, file) works with size limits and downgrade rules
