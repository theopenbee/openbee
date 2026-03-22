# WeChat (微信智能体) Platform Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add WeChat personal intelligent agent as a new messaging platform in OpenBee, enabling the AI assistant to receive and reply to messages via personal WeChat using long-polling.

**Architecture:** New `internal/platform/weixin/` package implementing `Platform`, `PlatformReceiverAdapter`, and `PlatformSenderAdapter` interfaces. Uses HTTP long-polling against `ilinkai.weixin.qq.com` API, AES-128-ECB for CDN media encryption, and QR code login integrated into `openbee config`. Follows the exact same patterns as the existing Telegram/Feishu/WeCom/DingTalk integrations.

**Tech Stack:** Go 1.25, `crypto/aes` (ECB), `github.com/mdp/qrterminal` (QR rendering), `github.com/AlecAivazis/survey/v2` (CLI prompts)

**Spec:** `docs/superpowers/specs/2026-03-22-weixin-platform-integration-design.md`

---

## File Map

### New files (create)

| File | Responsibility |
|------|---------------|
| `internal/platform/weixin/types.go` | Protocol types: `WeixinMessage`, `MessageItem`, `CDNMedia`, constants |
| `internal/platform/weixin/types_test.go` | Type marshaling tests |
| `internal/platform/weixin/crypto.go` | AES-128-ECB encrypt/decrypt, PKCS#7 padding |
| `internal/platform/weixin/crypto_test.go` | Roundtrip, known-vector, and edge-case tests |
| `internal/platform/weixin/markdown.go` | `markdownToPlainText` conversion |
| `internal/platform/weixin/markdown_test.go` | Table-driven markdown conversion tests |
| `internal/platform/weixin/session.go` | Session pause manager (errcode -14, 1hr cooldown) |
| `internal/platform/weixin/session_test.go` | Pause/resume timing tests |
| `internal/platform/weixin/api.go` | `WeixinAPIClient` — all HTTP API calls |
| `internal/platform/weixin/api_test.go` | API client tests with httptest server |
| `internal/platform/weixin/cdn.go` | CDN download+decrypt, encrypt+upload |
| `internal/platform/weixin/cdn_test.go` | CDN roundtrip tests with mock HTTP |
| `internal/platform/weixin/auth.go` | QR login flow (used by config command) |
| `internal/platform/weixin/handler.go` | `WeixinPlatform`, `WeixinReceiver`, `WeixinSender` |
| `internal/platform/weixin/handler_test.go` | Receiver/sender integration tests |

### Existing files (modify)

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `WeixinConfig` struct, add to `PlatformsConfig`, add defaults |
| `internal/config/config.yaml.tmpl` | Add `weixin` section |
| `internal/app/app.go` | Add weixin to `buildPlatforms`, add import |
| `cmd/openbee/config.go` | Add "微信" to platform multi-select, QR scan flow |
| `go.mod` / `go.sum` | Add `github.com/mdp/qrterminal` dependency |

---

## Task 1: Protocol Types

**Files:**
- Create: `internal/platform/weixin/types.go`
- Create: `internal/platform/weixin/types_test.go`

- [ ] **Step 1: Create package directory**

```bash
mkdir -p internal/platform/weixin
```

- [ ] **Step 2: Write types_test.go — test JSON marshaling**

```go
package weixin

import (
	"encoding/json"
	"testing"
)

func TestWeixinMessageMarshalRoundtrip(t *testing.T) {
	msg := WeixinMessage{
		MessageID:    12345,
		FromUserID:   "user@im.wechat",
		ToUserID:     "bot@im.wechat",
		CreateTimeMs: 1711100000000,
		MessageType:  MessageTypeUser,
		MessageState: MessageStateFinish,
		ContextToken: "ctx-token-abc",
		ItemList: []MessageItem{
			{
				Type:     MessageItemTypeText,
				TextItem: &TextItem{Text: "hello"},
			},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got WeixinMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.FromUserID != msg.FromUserID {
		t.Errorf("FromUserID = %q, want %q", got.FromUserID, msg.FromUserID)
	}
	if got.ContextToken != msg.ContextToken {
		t.Errorf("ContextToken = %q, want %q", got.ContextToken, msg.ContextToken)
	}
	if len(got.ItemList) != 1 || got.ItemList[0].TextItem == nil {
		t.Fatal("expected 1 text item")
	}
	if got.ItemList[0].TextItem.Text != "hello" {
		t.Errorf("text = %q, want %q", got.ItemList[0].TextItem.Text, "hello")
	}
}

func TestMessageItemTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		val  int
		want int
	}{
		{"text", MessageItemTypeText, 1},
		{"image", MessageItemTypeImage, 2},
		{"voice", MessageItemTypeVoice, 3},
		{"file", MessageItemTypeFile, 4},
		{"video", MessageItemTypeVideo, 5},
	}
	for _, tt := range tests {
		if tt.val != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.val, tt.want)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/platform/weixin/ -v -run TestWeixin
```

Expected: FAIL — types not defined yet.

- [ ] **Step 4: Write types.go**

```go
package weixin

// Message type constants.
const (
	MessageTypeUser = 1
	MessageTypeBot  = 2
)

// Message item type constants.
const (
	MessageItemTypeText  = 1
	MessageItemTypeImage = 2
	MessageItemTypeVoice = 3
	MessageItemTypeFile  = 4
	MessageItemTypeVideo = 5
)

// Message state constants.
const (
	MessageStateNew        = 0
	MessageStateGenerating = 1
	MessageStateFinish     = 2
)

// Typing status constants.
const (
	TypingStatusTyping = 1
	TypingStatusCancel = 2
)

// WeixinMessage represents a message in the WeChat protocol.
type WeixinMessage struct {
	Seq          int64         `json:"seq,omitempty"`
	MessageID    int64         `json:"message_id,omitempty"`
	FromUserID   string        `json:"from_user_id,omitempty"`
	ToUserID     string        `json:"to_user_id,omitempty"`
	CreateTimeMs int64         `json:"create_time_ms,omitempty"`
	MessageType  int           `json:"message_type,omitempty"`
	MessageState int           `json:"message_state,omitempty"`
	ItemList     []MessageItem `json:"item_list,omitempty"`
	ContextToken string        `json:"context_token,omitempty"`
	SessionID    string        `json:"session_id,omitempty"`
	GroupID      string        `json:"group_id,omitempty"`
	DeleteTimeMs int64         `json:"delete_time_ms,omitempty"`
}

// MessageItem represents a single content item within a message.
type MessageItem struct {
	Type         int        `json:"type,omitempty"`
	CreateTimeMs int64      `json:"create_time_ms,omitempty"`
	TextItem     *TextItem  `json:"text_item,omitempty"`
	ImageItem    *ImageItem `json:"image_item,omitempty"`
	VoiceItem    *VoiceItem `json:"voice_item,omitempty"`
	FileItem     *FileItem  `json:"file_item,omitempty"`
	VideoItem    *VideoItem `json:"video_item,omitempty"`
}

// TextItem holds text content.
type TextItem struct {
	Text string `json:"text,omitempty"`
}

// ImageItem holds image content with CDN reference.
type ImageItem struct {
	CDNMedia
}

// VoiceItem holds voice content with optional STT text.
type VoiceItem struct {
	CDNMedia
	Text string `json:"text,omitempty"` // speech-to-text result
}

// FileItem holds file content with metadata.
type FileItem struct {
	CDNMedia
	FileName string `json:"file_name,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

// VideoItem holds video content with CDN reference.
type VideoItem struct {
	CDNMedia
}

// CDNMedia contains CDN download parameters and encryption key.
type CDNMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	AesKey            string `json:"aes_key,omitempty"`
	EncryptType       int    `json:"encrypt_type,omitempty"`
}

// API request/response types.

type GetUpdatesReq struct {
	GetUpdatesBuf string `json:"get_updates_buf,omitempty"`
}

type GetUpdatesResp struct {
	Ret                 int             `json:"ret"`
	ErrCode             int             `json:"errcode"`
	ErrMsg              string          `json:"errmsg,omitempty"`
	Msgs                []WeixinMessage `json:"msgs,omitempty"`
	GetUpdatesBuf       string          `json:"get_updates_buf,omitempty"`
	LongPollingTimeoutMs int64          `json:"longpolling_timeout_ms,omitempty"`
}

type SendMessageReq struct {
	Msg *WeixinMessage `json:"msg,omitempty"`
}

type SendTypingReq struct {
	IlinkUserID   string `json:"ilink_user_id,omitempty"`
	TypingTicket  string `json:"typing_ticket,omitempty"`
	Status        int    `json:"status,omitempty"`
}

type GetConfigReq struct {
	IlinkUserID  string `json:"ilink_user_id,omitempty"`
	ContextToken string `json:"context_token,omitempty"`
}

type GetConfigResp struct {
	Ret          int    `json:"ret"`
	TypingTicket string `json:"typing_ticket,omitempty"`
}

type GetUploadUrlReq struct {
	FileKey         string `json:"filekey,omitempty"`
	MediaType       int    `json:"media_type,omitempty"` // 1=image, 2=video, 3=file, 4=voice
	ToUserID        string `json:"to_user_id,omitempty"`
	RawSize         int64  `json:"rawsize,omitempty"`
	RawFileMD5      string `json:"rawfilemd5,omitempty"`
	FileSize        int64  `json:"filesize,omitempty"` // ciphertext size
	ThumbRawSize    int64  `json:"thumb_rawsize,omitempty"`
	ThumbRawFileMD5 string `json:"thumb_rawfilemd5,omitempty"`
	ThumbFileSize   int64  `json:"thumb_filesize,omitempty"`
	AesKey          string `json:"aeskey,omitempty"`
}

type GetUploadUrlResp struct {
	Ret             int    `json:"ret"`
	UploadParam     string `json:"upload_param,omitempty"`
	ThumbUploadParam string `json:"thumb_upload_param,omitempty"`
}

type GetBotQRCodeResp struct {
	Ret    int    `json:"ret"`
	QRCode string `json:"qrcode,omitempty"`
}

type GetQRCodeStatusResp struct {
	Ret      int    `json:"ret"`
	Status   int    `json:"status,omitempty"` // 0=pending, 1=scanned, 2=confirmed
	BotToken string `json:"bot_token,omitempty"`
	BotID    string `json:"ilink_bot_id,omitempty"`
	BaseUrl  string `json:"base_url,omitempty"`
	UserID   string `json:"user_id,omitempty"`
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/platform/weixin/ -v -run TestWeixin
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/platform/weixin/types.go internal/platform/weixin/types_test.go
git commit -m "feat(weixin): add protocol type definitions"
```

---

## Task 2: AES-128-ECB Crypto

**Files:**
- Create: `internal/platform/weixin/crypto.go`
- Create: `internal/platform/weixin/crypto_test.go`

- [ ] **Step 1: Write crypto_test.go**

```go
package weixin

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestPKCS7PadUnpad(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		blockSz int
	}{
		{"empty", []byte{}, 16},
		{"1 byte", []byte{0x42}, 16},
		{"15 bytes", bytes.Repeat([]byte{0xAA}, 15), 16},
		{"16 bytes exact", bytes.Repeat([]byte{0xBB}, 16), 16},
		{"17 bytes", bytes.Repeat([]byte{0xCC}, 17), 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			padded := pkcs7Pad(tt.input, tt.blockSz)
			if len(padded)%tt.blockSz != 0 {
				t.Fatalf("padded length %d not multiple of %d", len(padded), tt.blockSz)
			}
			unpadded, err := pkcs7Unpad(padded, tt.blockSz)
			if err != nil {
				t.Fatalf("unpad: %v", err)
			}
			if !bytes.Equal(unpadded, tt.input) {
				t.Errorf("roundtrip failed: got %x, want %x", unpadded, tt.input)
			}
		})
	}
}

func TestEncryptDecryptAES128ECBRoundtrip(t *testing.T) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		plain []byte
	}{
		{"short", []byte("hello world")},
		{"exact block", bytes.Repeat([]byte{0x41}, 16)},
		{"multi block", bytes.Repeat([]byte{0x42}, 100)},
		{"large", bytes.Repeat([]byte{0x43}, 4096)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cipher := encryptAES128ECB(tt.plain, key)
			if len(cipher)%16 != 0 {
				t.Fatalf("ciphertext length %d not multiple of 16", len(cipher))
			}
			plain, err := decryptAES128ECB(cipher, key)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if !bytes.Equal(plain, tt.plain) {
				t.Errorf("roundtrip failed for %s", tt.name)
			}
		})
	}
}

func TestDecryptAES128ECBInvalidPadding(t *testing.T) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	// Create a ciphertext with corrupted padding
	plain := []byte("test data 12345")
	cipher := encryptAES128ECB(plain, key)
	// Corrupt last byte
	cipher[len(cipher)-1] ^= 0xFF

	_, err := decryptAES128ECB(cipher, key)
	if err == nil {
		t.Error("expected error for corrupted padding")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/platform/weixin/ -v -run TestPKCS7\|TestEncrypt\|TestDecrypt
```

Expected: FAIL — functions not defined.

- [ ] **Step 3: Write crypto.go**

```go
package weixin

import (
	"crypto/aes"
	"fmt"
)

// encryptAES128ECB encrypts plaintext using AES-128-ECB with PKCS#7 padding.
// Note: ECB mode is inherently insecure for multi-block data; this is dictated
// by the WeChat CDN protocol, not a design choice.
func encryptAES128ECB(plaintext []byte, key []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(fmt.Sprintf("weixin: invalid AES key: %v", err))
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(ciphertext[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return ciphertext
}

// decryptAES128ECB decrypts AES-128-ECB ciphertext and removes PKCS#7 padding.
func decryptAES128ECB(ciphertext []byte, key []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("weixin: ciphertext is empty")
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("weixin: ciphertext length %d not a multiple of %d", len(ciphertext), aes.BlockSize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("weixin: create cipher: %w", err)
	}
	plaintext := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(plaintext[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}
	return pkcs7Unpad(plaintext, aes.BlockSize)
}

// pkcs7Pad pads data to a multiple of blockSize using PKCS#7.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	padding := make([]byte, padLen)
	for i := range padding {
		padding[i] = byte(padLen)
	}
	return append(data, padding...)
}

// pkcs7Unpad removes PKCS#7 padding.
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("weixin: invalid padded data length: %d", len(data))
	}
	padLen := int(data[len(data)-1])
	if padLen < 1 || padLen > blockSize || padLen > len(data) {
		return nil, fmt.Errorf("weixin: invalid PKCS#7 padding value: %d", padLen)
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, fmt.Errorf("weixin: PKCS#7 padding bytes inconsistent")
		}
	}
	return data[:len(data)-padLen], nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/platform/weixin/ -v -run TestPKCS7\|TestEncrypt\|TestDecrypt
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/weixin/crypto.go internal/platform/weixin/crypto_test.go
git commit -m "feat(weixin): add AES-128-ECB encryption for CDN media"
```

---

## Task 3: Markdown to Plain Text

**Files:**
- Create: `internal/platform/weixin/markdown.go`
- Create: `internal/platform/weixin/markdown_test.go`

- [ ] **Step 1: Write markdown_test.go**

```go
package weixin

import "testing"

func TestMarkdownToPlainText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"bold", "this is **bold** text", "this is bold text"},
		{"italic", "this is *italic* text", "this is italic text"},
		{"inline code", "use `fmt.Println`", "use fmt.Println"},
		{"link", "visit [Google](https://google.com)", "visit Google (https://google.com)"},
		{"image", "see ![alt](https://img.png) here", "see  here"},
		{"heading", "# Title\n## Subtitle", "Title\nSubtitle"},
		{"code block", "before\n```go\nfmt.Println(\"hi\")\n```\nafter", "before\nfmt.Println(\"hi\")\nafter"},
		{"code block no lang", "```\ncode\n```", "code"},
		{"mixed", "# Title\n**bold** and *italic* with `code`", "Title\nbold and italic with code"},
		{"strikethrough", "this is ~~deleted~~ text", "this is deleted text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := markdownToPlainText(tt.input)
			if got != tt.want {
				t.Errorf("markdownToPlainText(%q)\ngot:  %q\nwant: %q", tt.input, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/platform/weixin/ -v -run TestMarkdown
```

Expected: FAIL

- [ ] **Step 3: Write markdown.go**

```go
package weixin

import (
	"regexp"
	"strings"
)

var (
	reCodeBlock     = regexp.MustCompile("(?s)```[a-zA-Z]*\n?(.*?)```")
	reImage         = regexp.MustCompile(`!\[[^\]]*\]\([^)]+\)`)
	reLink          = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reBold          = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reStrikethrough = regexp.MustCompile(`~~(.+?)~~`)
	reItalic        = regexp.MustCompile(`\*(.+?)\*`)
	reInlineCode    = regexp.MustCompile("`([^`]+)`")
	reHeading       = regexp.MustCompile(`(?m)^#{1,6}\s+`)
)

// markdownToPlainText strips Markdown formatting for WeChat's plaintext-only display.
func markdownToPlainText(text string) string {
	// Code blocks: keep content, remove fences
	text = reCodeBlock.ReplaceAllString(text, "$1")
	// Images: remove entirely
	text = reImage.ReplaceAllString(text, "")
	// Links: [text](url) → text (url)
	text = reLink.ReplaceAllString(text, "$1 ($2)")
	// Bold
	text = reBold.ReplaceAllString(text, "$1")
	// Strikethrough
	text = reStrikethrough.ReplaceAllString(text, "$1")
	// Italic
	text = reItalic.ReplaceAllString(text, "$1")
	// Inline code
	text = reInlineCode.ReplaceAllString(text, "$1")
	// Headings
	text = reHeading.ReplaceAllString(text, "")
	// Clean up extra blank lines
	text = strings.TrimSpace(text)
	return text
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/platform/weixin/ -v -run TestMarkdown
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/weixin/markdown.go internal/platform/weixin/markdown_test.go
git commit -m "feat(weixin): add markdown to plain text conversion"
```

---

## Task 4: Session Pause Manager

**Files:**
- Create: `internal/platform/weixin/session.go`
- Create: `internal/platform/weixin/session_test.go`

- [ ] **Step 1: Write session_test.go**

```go
package weixin

import (
	"testing"
	"time"
)

func TestSessionManager_NotPausedByDefault(t *testing.T) {
	sm := newSessionManager()
	if sm.isPaused() {
		t.Error("session should not be paused by default")
	}
}

func TestSessionManager_PauseAndResume(t *testing.T) {
	sm := &sessionManager{pauseDuration: 50 * time.Millisecond}
	sm.pause()
	if !sm.isPaused() {
		t.Error("session should be paused after pause()")
	}
	remaining := sm.remainingPause()
	if remaining <= 0 || remaining > 50*time.Millisecond {
		t.Errorf("remaining = %v, want (0, 50ms]", remaining)
	}
	time.Sleep(60 * time.Millisecond)
	if sm.isPaused() {
		t.Error("session should auto-resume after pause duration")
	}
}

func TestSessionManager_RemainingWhenNotPaused(t *testing.T) {
	sm := newSessionManager()
	if sm.remainingPause() != 0 {
		t.Error("remaining should be 0 when not paused")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/platform/weixin/ -v -run TestSession
```

Expected: FAIL

- [ ] **Step 3: Write session.go**

```go
package weixin

import (
	"sync"
	"time"
)

const defaultPauseDuration = 1 * time.Hour

type sessionManager struct {
	mu            sync.Mutex
	pausedAt      time.Time
	pauseDuration time.Duration
}

func newSessionManager() *sessionManager {
	return &sessionManager{pauseDuration: defaultPauseDuration}
}

func (s *sessionManager) pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pausedAt = time.Now()
}

func (s *sessionManager) isPaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pausedAt.IsZero() {
		return false
	}
	return time.Since(s.pausedAt) < s.pauseDuration
}

func (s *sessionManager) remainingPause() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pausedAt.IsZero() {
		return 0
	}
	remaining := s.pauseDuration - time.Since(s.pausedAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/platform/weixin/ -v -run TestSession
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/weixin/session.go internal/platform/weixin/session_test.go
git commit -m "feat(weixin): add session pause manager for errcode -14"
```

---

## Task 5: API Client

**Files:**
- Create: `internal/platform/weixin/api.go`
- Create: `internal/platform/weixin/api_test.go`

- [ ] **Step 1: Write api_test.go**

```go
package weixin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIClient_GetUpdates(t *testing.T) {
	want := GetUpdatesResp{
		Ret: 0,
		Msgs: []WeixinMessage{
			{MessageID: 1, FromUserID: "user1", MessageType: MessageTypeUser, MessageState: MessageStateFinish},
		},
		GetUpdatesBuf: "cursor-2",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/get_updates" {
			t.Errorf("path = %q, want /ilink/bot/get_updates", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or wrong Authorization header")
		}
		if r.Header.Get("X-WECHAT-UIN") == "" {
			t.Error("missing X-WECHAT-UIN header")
		}
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "", "test-token")
	resp, err := client.GetUpdates(context.Background(), "cursor-1")
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if resp.Ret != 0 {
		t.Errorf("ret = %d, want 0", resp.Ret)
	}
	if len(resp.Msgs) != 1 {
		t.Fatalf("msgs count = %d, want 1", len(resp.Msgs))
	}
	if resp.GetUpdatesBuf != "cursor-2" {
		t.Errorf("cursor = %q, want %q", resp.GetUpdatesBuf, "cursor-2")
	}
}

func TestAPIClient_SendMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/send_message" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var req SendMessageReq
		json.NewDecoder(r.Body).Decode(&req)
		if req.Msg == nil || req.Msg.ToUserID != "user1" {
			t.Error("expected msg with ToUserID=user1")
		}
		json.NewEncoder(w).Encode(map[string]int{"ret": 0})
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "", "test-token")
	msg := &WeixinMessage{
		ToUserID: "user1",
		ItemList: []MessageItem{{Type: MessageItemTypeText, TextItem: &TextItem{Text: "hello"}}},
	}
	err := client.SendMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
}

func TestAPIClient_GetUpdatesSessionTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(GetUpdatesResp{Ret: -1, ErrCode: -14, ErrMsg: "session timeout"})
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "", "test-token")
	resp, err := client.GetUpdates(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrCode != -14 {
		t.Errorf("errcode = %d, want -14", resp.ErrCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/platform/weixin/ -v -run TestAPIClient
```

Expected: FAIL

- [ ] **Step 3: Write api.go**

```go
package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WeixinAPIClient communicates with the WeChat intelligent agent API.
type WeixinAPIClient struct {
	baseUrl    string
	cdnBaseUrl string
	token      string
	httpClient *http.Client
	longPoll   *http.Client
}

// NewAPIClient creates a new WeChat API client.
func NewAPIClient(baseUrl, cdnBaseUrl, token string) *WeixinAPIClient {
	return &WeixinAPIClient{
		baseUrl:    baseUrl,
		cdnBaseUrl: cdnBaseUrl,
		token:      token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		longPoll:   &http.Client{Timeout: 40 * time.Second},
	}
}

// GetUpdates long-polls for new messages.
func (c *WeixinAPIClient) GetUpdates(ctx context.Context, syncBuf string) (*GetUpdatesResp, error) {
	body := GetUpdatesReq{GetUpdatesBuf: syncBuf}
	var resp GetUpdatesResp
	if err := c.doRequest(ctx, c.longPoll, "/ilink/bot/get_updates", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SendMessage sends a message.
func (c *WeixinAPIClient) SendMessage(ctx context.Context, msg *WeixinMessage) error {
	body := SendMessageReq{Msg: msg}
	var resp struct{ Ret int `json:"ret"` }
	if err := c.doRequest(ctx, c.httpClient, "/ilink/bot/send_message", body, &resp); err != nil {
		return err
	}
	if resp.Ret != 0 {
		return fmt.Errorf("weixin: send_message ret=%d", resp.Ret)
	}
	return nil
}

// SendTyping sends a typing indicator.
func (c *WeixinAPIClient) SendTyping(ctx context.Context, userID, ticket string, status int) error {
	body := SendTypingReq{IlinkUserID: userID, TypingTicket: ticket, Status: status}
	var resp struct{ Ret int `json:"ret"` }
	return c.doRequest(ctx, c.httpClient, "/ilink/bot/send_typing", body, &resp)
}

// GetConfig fetches per-user config (typing ticket etc).
func (c *WeixinAPIClient) GetConfig(ctx context.Context, userID, contextToken string) (*GetConfigResp, error) {
	body := GetConfigReq{IlinkUserID: userID, ContextToken: contextToken}
	var resp GetConfigResp
	if err := c.doRequest(ctx, c.httpClient, "/ilink/bot/get_config", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetUploadUrl requests CDN upload credentials.
func (c *WeixinAPIClient) GetUploadUrl(ctx context.Context, req GetUploadUrlReq) (*GetUploadUrlResp, error) {
	var resp GetUploadUrlResp
	if err := c.doRequest(ctx, c.httpClient, "/ilink/bot/get_upload_url", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetBotQRCode requests a QR code for login.
func (c *WeixinAPIClient) GetBotQRCode(ctx context.Context, botType string) (*GetBotQRCodeResp, error) {
	body := map[string]string{"bot_type": botType}
	var resp GetBotQRCodeResp
	if err := c.doRequest(ctx, c.longPoll, "/ilink/bot/get_bot_qrcode", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetQRCodeStatus polls QR code scan status.
func (c *WeixinAPIClient) GetQRCodeStatus(ctx context.Context, qrcode string) (*GetQRCodeStatusResp, error) {
	body := map[string]string{"qrcode": qrcode}
	var resp GetQRCodeStatusResp
	if err := c.doRequest(ctx, c.longPoll, "/ilink/bot/get_qrcode_status", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *WeixinAPIClient) doRequest(ctx context.Context, client *http.Client, path string, body any, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("weixin: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseUrl+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("weixin: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("X-WECHAT-UIN", randomWechatUin())

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("weixin: %s: %w", path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("weixin: read response: %w", err)
	}
	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("weixin: unmarshal response: %w (body: %s)", err, string(respBody))
	}
	return nil
}

// randomWechatUin generates a random 4-byte value encoded as base64, per SDK convention.
func randomWechatUin() string {
	var buf [4]byte
	rand.Read(buf[:])
	return base64.StdEncoding.EncodeToString(buf[:])
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/platform/weixin/ -v -run TestAPIClient
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/weixin/api.go internal/platform/weixin/api_test.go
git commit -m "feat(weixin): add API client with all WeChat endpoints"
```

---

## Task 6: CDN Upload/Download

**Files:**
- Create: `internal/platform/weixin/cdn.go`
- Create: `internal/platform/weixin/cdn_test.go`

- [ ] **Step 1: Write cdn_test.go**

```go
package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadAndDecrypt(t *testing.T) {
	// Prepare encrypted content
	key := make([]byte, 16)
	rand.Read(key)
	plaintext := []byte("hello weixin cdn")
	ciphertext := encryptAES128ECB(plaintext, key)
	aesKeyBase64 := base64.StdEncoding.EncodeToString(key)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(ciphertext)
	}))
	defer srv.Close()

	got, err := downloadAndDecrypt(context.Background(), srv.URL, "param=test", aesKeyBase64)
	if err != nil {
		t.Fatalf("downloadAndDecrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("got %q, want %q", got, plaintext)
	}
}

func TestEncryptAndUpload(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		json.NewEncoder(w).Encode(map[string]string{"download_param": "dl-param-123"})
	}))
	defer srv.Close()

	plaintext := []byte("upload test data")
	dlParam, aesKeyHex, err := encryptAndUpload(context.Background(), plaintext, fmt.Sprintf("%s?upload=1", srv.URL), "filekey-1", srv.URL)
	if err != nil {
		t.Fatalf("encryptAndUpload: %v", err)
	}
	if dlParam == "" {
		t.Error("expected non-empty download_param")
	}
	if aesKeyHex == "" {
		t.Error("expected non-empty aesKey")
	}
	if len(receivedBody) == 0 {
		t.Error("expected upload body")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/platform/weixin/ -v -run TestDownloadAndDecrypt\|TestEncryptAndUpload
```

Expected: FAIL

- [ ] **Step 3: Write cdn.go**

```go
package weixin

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// downloadAndDecrypt downloads encrypted content from CDN and decrypts it.
func downloadAndDecrypt(ctx context.Context, cdnBaseUrl, encryptQueryParam, aesKeyBase64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(aesKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("weixin cdn: decode aes key: %w", err)
	}

	url := cdnBaseUrl + "?" + encryptQueryParam
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("weixin cdn: create request: %w", err)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weixin cdn download: %w", err)
	}
	defer resp.Body.Close()

	ciphertext, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("weixin cdn read: %w", err)
	}

	return decryptAES128ECB(ciphertext, key)
}

// encryptAndUpload encrypts data, uploads to CDN, returns download param and hex-encoded AES key.
func encryptAndUpload(ctx context.Context, data []byte, uploadURL, filekey, cdnBaseUrl string) (downloadParam string, aesKeyHex string, err error) {
	// Generate random AES-128 key
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return "", "", fmt.Errorf("weixin cdn: generate key: %w", err)
	}

	ciphertext := encryptAES128ECB(data, key)

	// Upload with retries
	var dlParam string
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		dlParam, lastErr = doUpload(ctx, uploadURL, ciphertext)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return "", "", fmt.Errorf("weixin cdn upload after 3 attempts: %w", lastErr)
	}

	return dlParam, hex.EncodeToString(key), nil
}

func doUpload(ctx context.Context, uploadURL string, data []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		DownloadParam string `json:"download_param"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.DownloadParam, nil
}

// computeMD5 returns hex-encoded MD5 of data.
func computeMD5(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}

// mediaCDNType maps MIME prefix to WeChat media type constant.
func mediaCDNType(mimeType string) int {
	switch {
	case len(mimeType) >= 5 && mimeType[:5] == "image":
		return 1
	case len(mimeType) >= 5 && mimeType[:5] == "video":
		return 2
	case len(mimeType) >= 5 && mimeType[:5] == "audio":
		return 4
	default:
		return 3 // file
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/platform/weixin/ -v -run TestDownloadAndDecrypt\|TestEncryptAndUpload
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/weixin/cdn.go internal/platform/weixin/cdn_test.go
git commit -m "feat(weixin): add CDN upload/download with AES-128-ECB encryption"
```

---

## Task 7: Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config.yaml.tmpl`

- [ ] **Step 1: Add WeixinConfig to config.go**

In `internal/config/config.go`, add the struct after `TelegramConfig`:

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

Add `Weixin` field to `PlatformsConfig`:

```go
type PlatformsConfig struct {
	Feishu   FeishuConfig   `yaml:"feishu"`
	DingTalk DingTalkConfig `yaml:"dingtalk"`
	WeCom    WeComConfig    `yaml:"wecom"`
	Telegram TelegramConfig `yaml:"telegram"`
	Weixin   WeixinConfig   `yaml:"weixin"`
}
```

Add defaults in `applyDefaults`:

```go
if cfg.Bee.Platforms.Weixin.BaseURL == "" {
	cfg.Bee.Platforms.Weixin.BaseURL = "https://ilinkai.weixin.qq.com"
}
if cfg.Bee.Platforms.Weixin.CdnBaseURL == "" {
	cfg.Bee.Platforms.Weixin.CdnBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
}
if cfg.Bee.Platforms.Weixin.MaxMediaSize == 0 {
	cfg.Bee.Platforms.Weixin.MaxMediaSize = 50 * 1024 * 1024 // 50MB
}
```

- [ ] **Step 2: Add weixin section to config.yaml.tmpl**

After the telegram section, add:

```yaml
    weixin:
      enabled: {{.WeixinEnabled}}
      token: "{{.WeixinToken}}"
      user_id: "{{.WeixinUserID}}"
      # base_url: https://ilinkai.weixin.qq.com  # default
      # cdn_base_url: https://novac2c.cdn.weixin.qq.com/c2c  # default
      # max_media_size: 52428800  # 50MB default
```

- [ ] **Step 3: Verify existing tests still pass**

```bash
go test ./internal/config/ -v
```

Expected: PASS (or no tests — config may not have tests)

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go internal/config/config.yaml.tmpl
git commit -m "feat(weixin): add WeChat config struct and YAML template"
```

---

## Task 8: QR Login Auth Module

**Files:**
- Create: `internal/platform/weixin/auth.go`

- [ ] **Step 1: Add qrterminal dependency**

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go get github.com/mdp/qrterminal/v3
```

- [ ] **Step 2: Write auth.go**

```go
package weixin

import (
	"context"
	"fmt"
	"os"
	"time"

	qrterminal "github.com/mdp/qrterminal/v3"
)

// QRLoginResult contains the result of a successful QR code login.
type QRLoginResult struct {
	Token  string
	BotID  string
	BaseUrl string
	UserID string
}

// QRLogin performs the interactive QR code login flow.
// It renders a QR code in the terminal and waits for the user to scan it.
// Returns the login result or an error if the flow times out or fails.
func QRLogin(ctx context.Context, baseUrl string) (*QRLoginResult, error) {
	client := NewAPIClient(baseUrl, "", "")

	// Get QR code
	qrResp, err := client.GetBotQRCode(ctx, "3")
	if err != nil {
		return nil, fmt.Errorf("get QR code: %w", err)
	}
	if qrResp.QRCode == "" {
		return nil, fmt.Errorf("empty QR code in response")
	}

	// Render QR code in terminal
	fmt.Println("\nScan this QR code with WeChat to login:")
	qrterminal.GenerateWithConfig(qrResp.QRCode, qrterminal.Config{
		Level:     qrterminal.L,
		Writer:    os.Stdout,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
	})
	fmt.Println("\nWaiting for scan... (timeout: 5 minutes)")

	// Poll for scan status
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		statusResp, err := client.GetQRCodeStatus(ctx, qrResp.QRCode)
		if err != nil {
			// Timeout is expected for long-poll, retry
			continue
		}
		if statusResp.Status == 2 { // confirmed
			return &QRLoginResult{
				Token:   statusResp.BotToken,
				BotID:   statusResp.BotID,
				BaseUrl: statusResp.BaseUrl,
				UserID:  statusResp.UserID,
			}, nil
		}
		if statusResp.Status == 1 {
			fmt.Println("QR code scanned, waiting for confirmation...")
		}
	}
	return nil, fmt.Errorf("QR login timed out after 5 minutes")
}
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/platform/weixin/
```

Expected: Success

- [ ] **Step 4: Commit**

```bash
git add internal/platform/weixin/auth.go go.mod go.sum
git commit -m "feat(weixin): add QR code login flow"
```

---

## Task 9: Platform Handler (Receiver + Sender)

**Files:**
- Create: `internal/platform/weixin/handler.go`
- Create: `internal/platform/weixin/handler_test.go`

- [ ] **Step 1: Write handler_test.go — test message filtering and InboundMessage building**

```go
package weixin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/media"
	"github.com/theopenbee/openbee/internal/platform"
)

func TestReceiverFiltersMessages(t *testing.T) {
	msgs := []WeixinMessage{
		{MessageID: 1, FromUserID: "u1", MessageType: MessageTypeBot, MessageState: MessageStateFinish},   // bot msg, skip
		{MessageID: 2, FromUserID: "u2", MessageType: MessageTypeUser, MessageState: MessageStateNew},      // not finished, skip
		{MessageID: 3, FromUserID: "u3", MessageType: MessageTypeUser, MessageState: MessageStateGenerating}, // generating, skip
		{
			MessageID: 4, FromUserID: "u4", MessageType: MessageTypeUser, MessageState: MessageStateFinish,
			CreateTimeMs: 1711100000000, ContextToken: "tok",
			ItemList: []MessageItem{{Type: MessageItemTypeText, TextItem: &TextItem{Text: "hello"}}},
		},
	}

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			json.NewEncoder(w).Encode(GetUpdatesResp{Ret: 0, Msgs: msgs, GetUpdatesBuf: "c2"})
		} else {
			// Block until context cancelled
			<-r.Context().Done()
		}
	}))
	defer srv.Close()

	cfg := config.WeixinConfig{Enabled: true, Token: "tok", BaseURL: srv.URL, MaxMediaSize: 50 * 1024 * 1024}
	mediaSvc := media.NewService()
	p := NewPlatform(cfg, mediaSvc)
	recv := p.Receiver()

	var received []platform.InboundMessage
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go recv.Start(ctx, func(msg platform.InboundMessage) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	})

	// Wait for messages to be dispatched
	time.Sleep(1 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("dispatched %d messages, want 1", len(received))
	}
	msg := received[0]
	if msg.Platform != "weixin" {
		t.Errorf("platform = %q", msg.Platform)
	}
	if msg.SenderID != "u4" {
		t.Errorf("senderID = %q", msg.SenderID)
	}
	if msg.SessionKey != "weixin:u4:u4" {
		t.Errorf("sessionKey = %q", msg.SessionKey)
	}
	if msg.Content != "hello" {
		t.Errorf("content = %q", msg.Content)
	}
	if msg.PlatformMessageID != "4" {
		t.Errorf("platformMessageID = %q", msg.PlatformMessageID)
	}
}

func TestSenderSendsText(t *testing.T) {
	var sentMsg SendMessageReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/send_message":
			json.NewDecoder(r.Body).Decode(&sentMsg)
			json.NewEncoder(w).Encode(map[string]int{"ret": 0})
		default:
			json.NewEncoder(w).Encode(map[string]int{"ret": 0})
		}
	}))
	defer srv.Close()

	cfg := config.WeixinConfig{Enabled: true, Token: "tok", BaseURL: srv.URL, MaxMediaSize: 50 * 1024 * 1024}
	p := NewPlatform(cfg, media.NewService())

	rawMsg := WeixinMessage{FromUserID: "user1", ToUserID: "bot1", ContextToken: "ctx-tok"}
	rawBytes, _ := json.Marshal(rawMsg)

	err := p.Sender().Send(context.Background(), platform.OutboundMessage{
		SessionKey: "weixin:user1:user1",
		Content:    "**hello** world",
		ReplyTo: platform.InboundMessage{
			Raw: string(rawBytes),
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sentMsg.Msg == nil {
		t.Fatal("no message sent")
	}
	if sentMsg.Msg.ToUserID != "user1" {
		t.Errorf("to = %q, want user1", sentMsg.Msg.ToUserID)
	}
	// Check markdown was stripped
	if len(sentMsg.Msg.ItemList) != 1 || sentMsg.Msg.ItemList[0].TextItem == nil {
		t.Fatal("expected 1 text item")
	}
	if sentMsg.Msg.ItemList[0].TextItem.Text != "hello world" {
		t.Errorf("text = %q, want 'hello world'", sentMsg.Msg.ItemList[0].TextItem.Text)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/platform/weixin/ -v -run TestReceiver\|TestSender
```

Expected: FAIL

- [ ] **Step 3: Write handler.go**

```go
package weixin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/media"
	"github.com/theopenbee/openbee/internal/platform"
)

var log = logger.With(zap.String("component", "weixin"))

var _ platform.Platform = (*WeixinPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*WeixinReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*WeixinSender)(nil)

// WeixinPlatform implements platform.Platform for WeChat intelligent agent.
type WeixinPlatform struct {
	cfg      config.WeixinConfig
	media    *media.Service
	receiver *WeixinReceiver
	sender   *WeixinSender
	client   *WeixinAPIClient
}

// NewPlatform creates a new WeChat platform instance.
func NewPlatform(cfg config.WeixinConfig, mediaSvc *media.Service) platform.Platform {
	client := NewAPIClient(cfg.BaseURL, cfg.CdnBaseURL, cfg.Token)
	p := &WeixinPlatform{
		cfg:    cfg,
		media:  mediaSvc,
		client: client,
	}
	p.receiver = &WeixinReceiver{cfg: cfg, media: mediaSvc, client: client, session: newSessionManager()}
	p.sender = &WeixinSender{cfg: cfg, media: mediaSvc, client: client}
	return p
}

func (p *WeixinPlatform) ID() string                                  { return "weixin" }
func (p *WeixinPlatform) Receiver() platform.PlatformReceiverAdapter  { return p.receiver }
func (p *WeixinPlatform) Sender() platform.PlatformSenderAdapter      { return p.sender }

// --- Receiver ---

// WeixinReceiver implements long-polling message reception.
type WeixinReceiver struct {
	cfg     config.WeixinConfig
	media   *media.Service
	client  *WeixinAPIClient
	session *sessionManager
}

// Start begins the long-polling loop and dispatches messages.
func (r *WeixinReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	log.Info("weixin receiver starting")

	syncBuf := loadSyncBuf()
	consecutiveFailures := 0

	for {
		if ctx.Err() != nil {
			return nil
		}

		if r.session.isPaused() {
			remaining := r.session.remainingPause()
			log.Warn("session paused", zap.Duration("remaining", remaining))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(30 * time.Second):
				continue
			}
		}

		resp, err := r.client.GetUpdates(ctx, syncBuf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			consecutiveFailures++
			if consecutiveFailures >= 3 {
				log.Error("3 consecutive failures, backing off 30s", zap.Error(err))
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(30 * time.Second):
				}
			}
			continue
		}

		// Check for session timeout
		if resp.ErrCode == -14 {
			log.Warn("session timeout (errcode -14), pausing for 1 hour. Consider re-running 'openbee config' to re-login.")
			r.session.pause()
			continue
		}

		consecutiveFailures = 0

		// Persist sync cursor
		if resp.GetUpdatesBuf != "" {
			syncBuf = resp.GetUpdatesBuf
			saveSyncBuf(syncBuf)
		}

		// Process messages
		for _, msg := range resp.Msgs {
			if msg.MessageType != MessageTypeUser || msg.MessageState != MessageStateFinish {
				continue
			}
			inbound := r.buildInboundMessage(ctx, msg)
			if inbound == nil {
				continue
			}

			// Send typing indicator in background
			go r.sendTypingIndicator(ctx, msg)

			dispatch(*inbound)
		}
	}
}

func (r *WeixinReceiver) buildInboundMessage(ctx context.Context, msg WeixinMessage) *platform.InboundMessage {
	if msg.FromUserID == "" {
		log.Warn("skipping message with no sender")
		return nil
	}

	var textParts []string
	var contentParts []string

	for _, item := range msg.ItemList {
		switch item.Type {
		case MessageItemTypeText:
			if item.TextItem != nil {
				textParts = append(textParts, item.TextItem.Text)
				contentParts = append(contentParts, item.TextItem.Text)
			}

		case MessageItemTypeImage:
			if item.ImageItem != nil {
				placeholder := r.downloadMedia(ctx, item.ImageItem.CDNMedia, "image", "")
				if placeholder != "" {
					contentParts = append(contentParts, placeholder)
				}
			}

		case MessageItemTypeVoice:
			if item.VoiceItem != nil {
				placeholder := r.downloadMedia(ctx, item.VoiceItem.CDNMedia, "audio", "")
				if placeholder != "" {
					contentParts = append(contentParts, placeholder)
				}
				if item.VoiceItem.Text != "" {
					textParts = append(textParts, "[语音转文字] "+item.VoiceItem.Text)
					contentParts = append(contentParts, "[语音转文字] "+item.VoiceItem.Text)
				}
			}

		case MessageItemTypeFile:
			if item.FileItem != nil {
				placeholder := r.downloadMedia(ctx, item.FileItem.CDNMedia, "document", item.FileItem.FileName)
				if placeholder != "" {
					contentParts = append(contentParts, placeholder)
				}
			}

		case MessageItemTypeVideo:
			if item.VideoItem != nil {
				placeholder := r.downloadMedia(ctx, item.VideoItem.CDNMedia, "video", "")
				if placeholder != "" {
					contentParts = append(contentParts, placeholder)
				}
			}
		}
	}

	content := strings.Join(contentParts, "\n")
	if content == "" {
		log.Warn("skipping empty message", zap.Int64("message_id", msg.MessageID))
		return nil
	}

	rawBytes, err := json.Marshal(msg)
	if err != nil {
		log.Error("marshal raw message", zap.Error(err))
		return nil
	}

	return &platform.InboundMessage{
		Platform:          "weixin",
		SenderID:          msg.FromUserID,
		SessionKey:        fmt.Sprintf("weixin:%s:%s", msg.FromUserID, msg.FromUserID),
		Content:           content,
		RawContent:        strings.Join(textParts, "\n"),
		Raw:               string(rawBytes),
		PlatformMessageID: strconv.FormatInt(msg.MessageID, 10),
		MessageTime:       msg.CreateTimeMs,
	}
}

func (r *WeixinReceiver) downloadMedia(ctx context.Context, cdn CDNMedia, mediaType, fileName string) string {
	if cdn.EncryptQueryParam == "" || cdn.AesKey == "" {
		log.Warn("missing CDN params, skipping media download")
		return ""
	}

	data, err := downloadAndDecrypt(ctx, r.client.cdnBaseUrl, cdn.EncryptQueryParam, cdn.AesKey)
	if err != nil {
		log.Error("download media failed", zap.Error(err), zap.String("type", mediaType))
		return ""
	}

	contentType := r.media.DetectMIME(data, fileName)
	ext := r.media.ExtensionFromMIME(contentType)

	path, err := r.media.SaveInbound(ctx, data, ext)
	if err != nil {
		log.Error("save media failed", zap.Error(err))
		return ""
	}

	return r.media.BuildPlaceholder(mediaType, path, fileName)
}

func (r *WeixinReceiver) sendTypingIndicator(ctx context.Context, msg WeixinMessage) {
	configResp, err := r.client.GetConfig(ctx, msg.FromUserID, msg.ContextToken)
	if err != nil {
		log.Debug("get config for typing failed", zap.Error(err))
		return
	}
	if configResp.TypingTicket == "" {
		return
	}
	if err := r.client.SendTyping(ctx, msg.FromUserID, configResp.TypingTicket, TypingStatusTyping); err != nil {
		log.Debug("send typing failed", zap.Error(err))
	}
}

// --- Sender ---

// WeixinSender implements message sending to WeChat.
type WeixinSender struct {
	cfg    config.WeixinConfig
	media  *media.Service
	client *WeixinAPIClient
}

// Send sends an outbound message.
func (s *WeixinSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var rawMsg WeixinMessage
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &rawMsg); err != nil {
		return fmt.Errorf("weixin: parse reply context: %w", err)
	}

	toUserID := rawMsg.FromUserID
	contextToken := rawMsg.ContextToken

	// Handle media
	if msg.MediaPath != "" {
		return s.sendMedia(ctx, toUserID, contextToken, msg.MediaPath, msg.Content)
	}

	// Text only — strip markdown
	text := markdownToPlainText(msg.Content)
	return s.sendTextMessage(ctx, toUserID, contextToken, text)
}

func (s *WeixinSender) sendTextMessage(ctx context.Context, toUserID, contextToken, text string) error {
	weixinMsg := &WeixinMessage{
		ToUserID:     toUserID,
		MessageType:  MessageTypeBot,
		ContextToken: contextToken,
		ItemList: []MessageItem{
			{
				Type:     MessageItemTypeText,
				TextItem: &TextItem{Text: text},
			},
		},
	}
	return s.client.SendMessage(ctx, weixinMsg)
}

func (s *WeixinSender) sendMedia(ctx context.Context, toUserID, contextToken, mediaPath, caption string) error {
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("weixin: read media: %w", err)
	}
	if len(data) > s.cfg.MaxMediaSize {
		log.Warn("media too large, sending text only", zap.Int("size", len(data)), zap.Int("max", s.cfg.MaxMediaSize))
		return s.sendTextMessage(ctx, toUserID, contextToken, markdownToPlainText(caption))
	}

	contentType := s.media.DetectMIME(data, filepath.Base(mediaPath))
	cdnType := mediaCDNType(contentType)

	// Encrypt locally
	key := make([]byte, 16)
	if _, err := cryptoRandRead(key); err != nil {
		return fmt.Errorf("weixin: generate key: %w", err)
	}
	ciphertext := encryptAES128ECB(data, key)
	aesKeyHex := fmt.Sprintf("%x", key)

	// Get upload URL
	uploadReq := GetUploadUrlReq{
		FileKey:    fmt.Sprintf("weixin_%d", time.Now().UnixMilli()),
		MediaType:  cdnType,
		ToUserID:   toUserID,
		RawSize:    int64(len(data)),
		RawFileMD5: computeMD5(data),
		FileSize:   int64(len(ciphertext)),
		AesKey:     aesKeyHex,
	}

	uploadResp, err := s.client.GetUploadUrl(ctx, uploadReq)
	if err != nil {
		return fmt.Errorf("weixin: get upload url: %w", err)
	}

	// Upload ciphertext to CDN (with retries)
	dlParam, uploadErr := doUploadWithRetry(ctx, uploadResp.UploadParam, ciphertext)
	if uploadErr != nil {
		log.Warn("CDN upload failed, falling back to text", zap.Error(uploadErr))
		return s.sendTextMessage(ctx, toUserID, contextToken, markdownToPlainText(caption))
	}

	// Build message with media item
	cdnMedia := CDNMedia{
		EncryptQueryParam: dlParam,
		AesKey:            base64.StdEncoding.EncodeToString(key),
	}

	var item MessageItem
	switch cdnType {
	case 1: // image
		item = MessageItem{Type: MessageItemTypeImage, ImageItem: &ImageItem{CDNMedia: cdnMedia}}
	case 2: // video
		item = MessageItem{Type: MessageItemTypeVideo, VideoItem: &VideoItem{CDNMedia: cdnMedia}}
	case 4: // voice
		item = MessageItem{Type: MessageItemTypeVoice, VoiceItem: &VoiceItem{CDNMedia: cdnMedia}}
	default: // file
		item = MessageItem{Type: MessageItemTypeFile, FileItem: &FileItem{CDNMedia: cdnMedia, FileName: filepath.Base(mediaPath), FileSize: int64(len(data))}}
	}

	weixinMsg := &WeixinMessage{
		ToUserID:     toUserID,
		MessageType:  MessageTypeBot,
		ContextToken: contextToken,
		ItemList:     []MessageItem{item},
	}

	// Add text caption if present
	text := markdownToPlainText(caption)
	if text != "" {
		weixinMsg.ItemList = append([]MessageItem{
			{Type: MessageItemTypeText, TextItem: &TextItem{Text: text}},
		}, weixinMsg.ItemList...)
	}

	return s.client.SendMessage(ctx, weixinMsg)
}

func doUploadWithRetry(ctx context.Context, uploadURL string, data []byte) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		dlParam, err := doUpload(ctx, uploadURL, data)
		if err == nil {
			return dlParam, nil
		}
		lastErr = err
	}
	return "", lastErr
}

// cryptoRandRead wraps crypto/rand.Read for testability.
var cryptoRandRead = func(b []byte) (int, error) { return rand.Read(b) }

// --- Sync Buffer Persistence ---

func syncBufPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", "weixin", "sync.json")
}

func loadSyncBuf() string {
	data, err := os.ReadFile(syncBufPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveSyncBuf(buf string) {
	path := syncBufPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Error("create sync dir", zap.Error(err))
		return
	}
	if err := os.WriteFile(path, []byte(buf), 0o644); err != nil {
		log.Error("save sync buf", zap.Error(err))
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/platform/weixin/ -v -run TestReceiver\|TestSender
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/weixin/handler.go internal/platform/weixin/handler_test.go
git commit -m "feat(weixin): add platform handler with receiver and sender"
```

---

## Task 10: Platform Registration

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add import for weixin package**

In `internal/app/app.go`, add to imports:

```go
"github.com/theopenbee/openbee/internal/platform/weixin"
```

- [ ] **Step 2: Add weixin to buildPlatforms**

In the `buildPlatforms` function, add the weixin parameter and construction:

Update function signature to accept `wxc config.WeixinConfig`:

```go
func buildPlatforms(fc config.FeishuConfig, dc config.DingTalkConfig, wc config.WeComConfig, tc config.TelegramConfig, wxc config.WeixinConfig, mc config.MediaConfig) []platform.Platform {
```

Add before `return result`:

```go
if wxc.Enabled {
	result = append(result, weixin.NewPlatform(wxc, mediaSvc))
}
```

Update the call site in `BuildApp` to pass `cfg.Bee.Platforms.Weixin`:

```go
platforms := buildPlatforms(cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom, cfg.Bee.Platforms.Telegram, cfg.Bee.Platforms.Weixin, cfg.Bee.Media)
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./...
```

Expected: Success

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(weixin): register WeChat platform in app builder"
```

---

## Task 11: Config CLI — WeChat Platform Selection + QR Login

**Files:**
- Modify: `cmd/openbee/config.go`

- [ ] **Step 1: Add weixin fields to configValues struct**

Add after `TelegramToken`:

```go
WeixinEnabled bool
WeixinToken   string
WeixinUserID  string
```

- [ ] **Step 2: Load existing weixin config in loadExistingConfig**

Add mapping from existing config to configValues for weixin fields (token, user_id, enabled).

- [ ] **Step 3: Add "微信" to platform multi-select options**

Change the `Options` slice from:

```go
Options: []string{"Feishu", "DingTalk", "WeCom", "Telegram"},
```

to:

```go
Options: []string{"Feishu", "DingTalk", "WeCom", "Telegram", "微信"},
```

Also update `defaultPlatforms` building to check `vals.WeixinEnabled`.

- [ ] **Step 4: Add "微信" case in platform selection switch**

```go
case "微信":
	vals.WeixinEnabled = true
	if vals.WeixinToken != "" {
		var action string
		if err := survey.AskOne(&survey.Select{
			Message: "WeChat token already configured. What would you like to do?",
			Options: []string{"Skip (use existing token)", "Re-login with QR code"},
		}, &action); err != nil {
			return handleSurveyErr(err)
		}
		if action == "Skip (use existing token)" {
			break
		}
	}
	// QR login flow
	fmt.Println("\nStarting WeChat QR login...")
	baseUrl := "https://ilinkai.weixin.qq.com"
	result, err := weixin.QRLogin(context.Background(), baseUrl)
	if err != nil {
		fmt.Printf("WeChat login failed: %v\n", err)
		fmt.Println("You can retry later with 'openbee config'")
		vals.WeixinEnabled = false
		break
	}
	vals.WeixinToken = result.Token
	vals.WeixinUserID = result.UserID
	fmt.Println("WeChat login successful!")
```

Add import for `"context"` and `"github.com/theopenbee/openbee/internal/platform/weixin"` at the top of config.go.

- [ ] **Step 5: Reset WeixinEnabled in the reset block**

Add `vals.WeixinEnabled = false` alongside the other platform resets.

- [ ] **Step 6: Verify it compiles**

```bash
go build ./cmd/openbee/
```

Expected: Success

- [ ] **Step 7: Commit**

```bash
git add cmd/openbee/config.go
git commit -m "feat(weixin): add WeChat QR login to config wizard"
```

---

## Task 12: Final Integration Test

- [ ] **Step 1: Run all tests**

```bash
go test ./... -v -count=1
```

Expected: All PASS

- [ ] **Step 2: Verify full build**

```bash
go build -o /dev/null ./cmd/openbee/
```

Expected: Success

- [ ] **Step 3: Verify with a test config.yaml**

Create a test config with weixin enabled (but fake token) to ensure config loading works:

```bash
cd /Users/tengyongzhi/work/theopenbee/openbee && go test ./internal/config/ -v
```

- [ ] **Step 4: Final commit (if any fixes needed)**

```bash
git add -A && git commit -m "fix(weixin): address integration test issues"
```

Only if there were fixes needed. Skip if all tests passed.
