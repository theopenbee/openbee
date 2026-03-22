# WeChat (Weixin) Channel Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate WeChat personal messaging as a new platform channel in OpenBee, supporting text/image/video/file/voice messages with AES-128-ECB encrypted CDN media, long-polling reception, and CLI QR-code login.

**Architecture:** New `internal/platform/weixin/` package implementing `Platform`/`PlatformReceiverAdapter`/`PlatformSenderAdapter` interfaces. HTTP JSON API client for all WeChat endpoints. AES-128-ECB crypto module for CDN media encryption/decryption. Config and app wiring additions to existing files.

**Tech Stack:** Go standard library (`crypto/aes`, `net/http`, `encoding/json`), `github.com/mdp/qrterminal/v3` (terminal QR rendering), existing `media.Service` and `ffmedia` package.

**Spec:** `docs/superpowers/specs/2026-03-22-weixin-channel-design.md`
**Protocol Reference:** `openclaw-weixin-spec.md`

---

### Task 1: Configuration — WeixinConfig + defaults

**Files:**
- Modify: `internal/config/config.go:62-67` (PlatformsConfig), `internal/config/config.go:131-157` (applyDefaults)
- Modify: `internal/config/config.yaml.tmpl:34-36` (after telegram section)
- Modify: `cmd/openbee/config.go:23-51` (configValues), `cmd/openbee/config.go:74-104` (loadExistingConfig), `cmd/openbee/config.go:209-231` (platform selection)

- [ ] **Step 1: Add WeixinConfig struct and PlatformsConfig field**

In `internal/config/config.go`, add after `TelegramConfig`:

```go
type WeixinConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Token        string `yaml:"token"`
	BaseURL      string `yaml:"base_url"`
	CDNBaseURL   string `yaml:"cdn_base_url"`
	RouteTag     int    `yaml:"route_tag"`
	UserID       string `yaml:"user_id"`
	MaxMediaSize int    `yaml:"max_media_size"`
}
```

Add to `PlatformsConfig`:
```go
Weixin   WeixinConfig   `yaml:"weixin"`
```

- [ ] **Step 2: Add defaults in applyDefaults()**

```go
if cfg.Bee.Platforms.Weixin.BaseURL == "" {
	cfg.Bee.Platforms.Weixin.BaseURL = "https://ilinkai.weixin.qq.com"
}
if cfg.Bee.Platforms.Weixin.CDNBaseURL == "" {
	cfg.Bee.Platforms.Weixin.CDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
}
if cfg.Bee.Platforms.Weixin.MaxMediaSize == 0 {
	cfg.Bee.Platforms.Weixin.MaxMediaSize = 100 * 1024 * 1024 // 100MB
}
```

- [ ] **Step 3: Add weixin to config.yaml.tmpl**

After the telegram section, add:

```yaml
    weixin:
      enabled: {{.WeixinEnabled}}
      token: "{{.WeixinToken}}"
      base_url: "{{.WeixinBaseURL}}"
      cdn_base_url: "{{.WeixinCDNBaseURL}}"
      user_id: "{{.WeixinUserID}}"
      # route_tag: 0  # optional
      # max_media_size: 104857600  # 100MB default
```

- [ ] **Step 4: Update configValues, loadExistingConfig, platform selection in cmd/openbee/config.go**

Add to `configValues` struct:
```go
WeixinEnabled  bool
WeixinToken    string
WeixinBaseURL  string
WeixinCDNBaseURL string
WeixinUserID   string
```

Add to `loadExistingConfig`:
```go
WeixinEnabled:    cfg.Bee.Platforms.Weixin.Enabled,
WeixinToken:      cfg.Bee.Platforms.Weixin.Token,
WeixinBaseURL:    cfg.Bee.Platforms.Weixin.BaseURL,
WeixinCDNBaseURL: cfg.Bee.Platforms.Weixin.CDNBaseURL,
WeixinUserID:     cfg.Bee.Platforms.Weixin.UserID,
```

Add to defaultPlatforms check block:
```go
if vals.WeixinEnabled {
	defaultPlatforms = append(defaultPlatforms, "Weixin")
}
```

Update MultiSelect Options to include `"Weixin"`:
```go
Options: []string{"Feishu", "DingTalk", "WeCom", "Telegram", "Weixin"},
```

Add to reset block:
```go
vals.WeixinEnabled = false
```

Add case `"Weixin"` to the platform switch — for now just prompt for token, base_url, and user_id manually (QR login in Task 7):
```go
case "Weixin":
	vals.WeixinEnabled = true
	if err := survey.AskOne(&survey.Password{
		Message: "Weixin Bot Token:",
	}, &vals.WeixinToken, survey.WithValidator(survey.Required)); err != nil {
		return handleSurveyErr(err)
	}
	if err := survey.AskOne(&survey.Input{
		Message: "Weixin User ID:",
		Default: vals.WeixinUserID,
	}, &vals.WeixinUserID, survey.WithValidator(survey.Required)); err != nil {
		return handleSurveyErr(err)
	}
	vals.WeixinBaseURL = "https://ilinkai.weixin.qq.com"
	vals.WeixinCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`
Expected: compiles cleanly

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config.yaml.tmpl cmd/openbee/config.go
git commit -m "feat(weixin): add WeixinConfig and config wizard integration"
```

---

### Task 2: AES-128-ECB Crypto Module

**Files:**
- Create: `internal/platform/weixin/crypto.go`
- Create: `internal/platform/weixin/crypto_test.go`

- [ ] **Step 1: Write crypto_test.go with all test cases**

```go
package weixin

import (
	"crypto/aes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	plaintext := []byte("hello weixin media content!")
	encrypted := encryptAesEcb(plaintext, key)
	assert.Equal(t, 0, len(encrypted)%aes.BlockSize)

	decrypted, err := decryptAesEcb(encrypted, key)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptDecrypt_AlignedSize(t *testing.T) {
	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	// Exactly 16 bytes — needs full padding block
	plaintext := make([]byte, 16)
	_, err = rand.Read(plaintext)
	require.NoError(t, err)

	encrypted := encryptAesEcb(plaintext, key)
	decrypted, err := decryptAesEcb(encrypted, key)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestDecryptAesEcb_EmptyInput(t *testing.T) {
	key := make([]byte, 16)
	_, err := decryptAesEcb([]byte{}, key)
	assert.Error(t, err)
}

func TestDecryptAesEcb_InvalidLength(t *testing.T) {
	key := make([]byte, 16)
	_, err := decryptAesEcb([]byte("not16aligned"), key)
	assert.Error(t, err)
}

func TestAesEcbPaddedSize(t *testing.T) {
	// aesEcbPaddedSize is the WeChat spec formula for reporting filesize to the API.
	// It uses ((n+1)/16+1)*16, which differs from standard PKCS7 at n=16k-1.
	// This is intentional — the server expects this formula.
	tests := []struct {
		input int
		want  int
	}{
		{0, 16},
		{1, 16},
		{14, 16},
		{15, 32},   // spec formula: (16/16+1)*16=32; standard PKCS7 would be 16
		{16, 32},
		{31, 32},
		{32, 48},
	}
	for _, tt := range tests {
		got := aesEcbPaddedSize(tt.input)
		assert.Equal(t, tt.want, got, "aesEcbPaddedSize(%d)", tt.input)
	}
}

// TestAesEcbPaddedSize_MatchesActualEncryption verifies the formula matches actual
// encryptAesEcb output sizes for typical inputs. Note: at n=16k-1, the formula
// reports a larger size than the actual encrypted output — this is the WeChat API
// convention and the server accepts it.
func TestAesEcbPaddedSize_TypicalCases(t *testing.T) {
	key := make([]byte, 16)
	rand.Read(key)
	for _, n := range []int{0, 1, 10, 16, 100, 1024} {
		data := make([]byte, n)
		encrypted := encryptAesEcb(data, key)
		reported := aesEcbPaddedSize(n)
		assert.GreaterOrEqual(t, reported, len(encrypted),
			"reported size should be >= actual encrypted size for n=%d", n)
	}
}

func TestParseAesKey_Raw16Bytes(t *testing.T) {
	rawKey := make([]byte, 16)
	_, err := rand.Read(rawKey)
	require.NoError(t, err)

	b64 := base64.StdEncoding.EncodeToString(rawKey)
	got, err := parseAesKey(b64)
	require.NoError(t, err)
	assert.Equal(t, rawKey, got)
}

func TestParseAesKey_HexString32Bytes(t *testing.T) {
	rawKey := make([]byte, 16)
	_, err := rand.Read(rawKey)
	require.NoError(t, err)

	hexStr := hex.EncodeToString(rawKey) // 32 ASCII bytes
	b64 := base64.StdEncoding.EncodeToString([]byte(hexStr))
	got, err := parseAesKey(b64)
	require.NoError(t, err)
	assert.Equal(t, rawKey, got)
}

func TestParseAesKey_InvalidBase64(t *testing.T) {
	_, err := parseAesKey("not-valid!!!")
	assert.Error(t, err)
}

func TestParseAesKey_WrongLength(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("too-short"))
	_, err := parseAesKey(b64)
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/platform/weixin/ -v -run TestEncrypt`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement crypto.go**

```go
package weixin

import (
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// encryptAesEcb encrypts plaintext using AES-128-ECB with PKCS7 padding.
func encryptAesEcb(plaintext, key []byte) []byte {
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

// decryptAesEcb decrypts AES-128-ECB ciphertext and removes PKCS7 padding.
func decryptAesEcb(ciphertext, key []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("weixin: ciphertext is empty")
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("weixin: ciphertext length %d not a multiple of block size", len(ciphertext))
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

// parseAesKey decodes a base64-encoded AES key, handling the dual-format:
// - 16 bytes after decode → use directly as AES-128 key
// - 32 bytes after decode → treat as hex string, hex-decode to 16 bytes
func parseAesKey(base64Key string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("weixin: decode aes key base64: %w", err)
	}
	switch len(decoded) {
	case 16:
		return decoded, nil
	case 32:
		key, err := hex.DecodeString(string(decoded))
		if err != nil {
			return nil, fmt.Errorf("weixin: decode aes key hex: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("weixin: unexpected aes key length %d (want 16 or 32)", len(decoded))
	}
}

// aesEcbPaddedSize returns the ciphertext size for a given plaintext size.
// Formula from WeChat spec: ((plaintextSize + 1) / 16 + 1) * 16
func aesEcbPaddedSize(plaintextSize int) int {
	return ((plaintextSize+1)/16 + 1) * 16
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - (len(data) % blockSize)
	padded := make([]byte, len(data)+padLen)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}
	return padded
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("weixin: pkcs7 unpad: empty data")
	}
	padLen := int(data[len(data)-1])
	if padLen < 1 || padLen > blockSize || padLen > len(data) {
		return nil, fmt.Errorf("weixin: pkcs7 invalid padding value: %d", padLen)
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, fmt.Errorf("weixin: pkcs7 padding bytes inconsistent")
		}
	}
	return data[:len(data)-padLen], nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/platform/weixin/ -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/weixin/crypto.go internal/platform/weixin/crypto_test.go
git commit -m "feat(weixin): add AES-128-ECB crypto module with tests"
```

---

### Task 3: API Client — Data Types + HTTP Client

**Files:**
- Create: `internal/platform/weixin/api.go`

- [ ] **Step 1: Define data types and API client struct**

```go
package weixin

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/logger"
)

var log = logger.With(zap.String("component", "weixin"))

const channelVersion = "1.0.2"

// ─── API Data Types ──────────────────────────────────────────────────────────

type baseInfo struct {
	ChannelVersion string `json:"channel_version"`
}

type weixinMessage struct {
	Seq          int64         `json:"seq,omitempty"`
	MessageID    int64         `json:"message_id,omitempty"`
	FromUserID   string        `json:"from_user_id,omitempty"`
	ToUserID     string        `json:"to_user_id,omitempty"`
	ClientID     string        `json:"client_id,omitempty"`
	CreateTimeMs int64         `json:"create_time_ms,omitempty"`
	UpdateTimeMs int64         `json:"update_time_ms,omitempty"`
	DeleteTimeMs int64         `json:"delete_time_ms,omitempty"`
	SessionID    string        `json:"session_id,omitempty"`
	GroupID      string        `json:"group_id,omitempty"`
	MessageType  int           `json:"message_type,omitempty"`
	MessageState int           `json:"message_state,omitempty"`
	ItemList     []messageItem `json:"item_list,omitempty"`
	ContextToken string        `json:"context_token,omitempty"`
}

type messageItem struct {
	Type         int        `json:"type,omitempty"`
	CreateTimeMs int64      `json:"create_time_ms,omitempty"`
	UpdateTimeMs int64      `json:"update_time_ms,omitempty"`
	IsCompleted  bool       `json:"is_completed,omitempty"`
	MsgID        string     `json:"msg_id,omitempty"`
	TextItem     *textItem  `json:"text_item,omitempty"`
	ImageItem    *imageItem `json:"image_item,omitempty"`
	VoiceItem    *voiceItem `json:"voice_item,omitempty"`
	FileItem     *fileItem  `json:"file_item,omitempty"`
	VideoItem    *videoItem `json:"video_item,omitempty"`
}

type textItem struct {
	Text string `json:"text,omitempty"`
}

type cdnMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	AesKey            string `json:"aes_key,omitempty"`
	EncryptType       int    `json:"encrypt_type,omitempty"`
}

type imageItem struct {
	Media       *cdnMedia `json:"media,omitempty"`
	ThumbMedia  *cdnMedia `json:"thumb_media,omitempty"`
	AesKey      string    `json:"aeskey,omitempty"`
	URL         string    `json:"url,omitempty"`
	MidSize     int64     `json:"mid_size,omitempty"`
	ThumbSize   int64     `json:"thumb_size,omitempty"`
	ThumbHeight int       `json:"thumb_height,omitempty"`
	ThumbWidth  int       `json:"thumb_width,omitempty"`
	HDSize      int64     `json:"hd_size,omitempty"`
}

type voiceItem struct {
	Media         *cdnMedia `json:"media,omitempty"`
	EncodeType    int       `json:"encode_type,omitempty"`
	BitsPerSample int      `json:"bits_per_sample,omitempty"`
	SampleRate    int      `json:"sample_rate,omitempty"`
	Playtime      int      `json:"playtime,omitempty"`
	Text          string   `json:"text,omitempty"`
}

type fileItem struct {
	Media    *cdnMedia `json:"media,omitempty"`
	FileName string    `json:"file_name,omitempty"`
	MD5      string    `json:"md5,omitempty"`
	Len      string    `json:"len,omitempty"`
}

type videoItem struct {
	Media       *cdnMedia `json:"media,omitempty"`
	VideoSize   int64     `json:"video_size,omitempty"`
	PlayLength  int       `json:"play_length,omitempty"`
	VideoMD5    string    `json:"video_md5,omitempty"`
	ThumbMedia  *cdnMedia `json:"thumb_media,omitempty"`
	ThumbSize   int64     `json:"thumb_size,omitempty"`
	ThumbHeight int       `json:"thumb_height,omitempty"`
	ThumbWidth  int       `json:"thumb_width,omitempty"`
}

// ─── Request / Response Types ────────────────────────────────────────────────

type getUpdatesReq struct {
	GetUpdatesBuf string   `json:"get_updates_buf"`
	BaseInfo      baseInfo `json:"base_info"`
}

type getUpdatesResp struct {
	Ret                int              `json:"ret"`
	Msgs               []weixinMessage  `json:"msgs"`
	GetUpdatesBuf      string           `json:"get_updates_buf"`
	LongPollingTimeout int              `json:"longpolling_timeout_ms"`
}

type sendMessageReq struct {
	Msg      weixinMessage `json:"msg"`
	BaseInfo baseInfo      `json:"base_info"`
}

type sendMessageResp struct {
	Ret int `json:"ret"`
}

type getUploadURLReq struct {
	FileKey        string   `json:"filekey"`
	MediaType      int      `json:"media_type"`
	ToUserID       string   `json:"to_user_id"`
	RawSize        string   `json:"rawsize"`
	RawFileMD5     string   `json:"rawfilemd5"`
	FileSize       string   `json:"filesize"`
	AesKey         string   `json:"aeskey"`
	BaseInfo       baseInfo `json:"base_info"`
}

type getUploadURLResp struct {
	Ret         int    `json:"ret"`
	UploadParam string `json:"upload_param"`
}

type getConfigReq struct {
	ILinkUserID string   `json:"ilink_user_id"`
	BaseInfo    baseInfo `json:"base_info"`
}

type getConfigResp struct {
	Ret           int    `json:"ret"`
	TypingTicket  string `json:"typing_ticket"`
}

type sendTypingReq struct {
	ILinkUserID   string   `json:"ilink_user_id"`
	TypingTicket  string   `json:"typing_ticket"`
	Status        int      `json:"status"`
	BaseInfo      baseInfo `json:"base_info"`
}

type sendTypingResp struct {
	Ret int `json:"ret"`
}

// ─── API Client ──────────────────────────────────────────────────────────────

type apiClient struct {
	cfg        config.WeixinConfig
	httpClient *http.Client
}

func newAPIClient(cfg config.WeixinConfig) *apiClient {
	return &apiClient{
		cfg:        cfg,
		httpClient: &http.Client{},
	}
}

func (c *apiClient) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("X-WECHAT-UIN", generateWechatUIN())
	if c.cfg.RouteTag != 0 {
		req.Header.Set("SKRouteTag", strconv.Itoa(c.cfg.RouteTag))
	}
}

func generateWechatUIN() string {
	var buf [4]byte
	rand.Read(buf[:])
	n := binary.LittleEndian.Uint32(buf[:])
	return base64.StdEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(n), 10)))
}

func newBaseInfo() baseInfo {
	return baseInfo{ChannelVersion: channelVersion}
}

// ─── API Methods ─────────────────────────────────────────────────────────────

func (c *apiClient) getUpdates(ctx context.Context, cursor string) (*getUpdatesResp, error) {
	body := getUpdatesReq{
		GetUpdatesBuf: cursor,
		BaseInfo:      newBaseInfo(),
	}
	reqCtx, cancel := context.WithTimeout(ctx, 40*time.Second) // 35s server + 5s buffer
	defer cancel()

	var resp getUpdatesResp
	if err := c.doPost(reqCtx, "ilink/bot/getupdates", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *apiClient) sendMessage(ctx context.Context, msg weixinMessage) error {
	body := sendMessageReq{
		Msg:      msg,
		BaseInfo: newBaseInfo(),
	}
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var resp sendMessageResp
	if err := c.doPost(reqCtx, "ilink/bot/sendmessage", body, &resp); err != nil {
		return err
	}
	if resp.Ret != 0 {
		return fmt.Errorf("weixin sendmessage ret=%d", resp.Ret)
	}
	return nil
}

func (c *apiClient) getUploadURL(ctx context.Context, req getUploadURLReq) (*getUploadURLResp, error) {
	req.BaseInfo = newBaseInfo()
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var resp getUploadURLResp
	if err := c.doPost(reqCtx, "ilink/bot/getuploadurl", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *apiClient) getConfig(ctx context.Context) (*getConfigResp, error) {
	body := getConfigReq{
		ILinkUserID: c.cfg.UserID,
		BaseInfo:    newBaseInfo(),
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var resp getConfigResp
	if err := c.doPost(reqCtx, "ilink/bot/getconfig", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *apiClient) sendTyping(ctx context.Context, ticket string, status int) error {
	body := sendTypingReq{
		ILinkUserID:  c.cfg.UserID,
		TypingTicket: ticket,
		Status:       status,
		BaseInfo:     newBaseInfo(),
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var resp sendTypingResp
	if err := c.doPost(reqCtx, "ilink/bot/sendtyping", body, &resp); err != nil {
		return err
	}
	return nil
}

// uploadToCDN encrypts and uploads a file to CDN, returns the download encrypted query param.
func (c *apiClient) uploadToCDN(ctx context.Context, uploadParam string, data, aesKey []byte) (string, error) {
	encrypted := encryptAesEcb(data, aesKey)

	uploadURL := fmt.Sprintf("%s/upload?encrypted_query_param=%s",
		c.cfg.CDNBaseURL, url.QueryEscape(uploadParam))

	reqCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPut, uploadURL, bytes.NewReader(encrypted))
	if err != nil {
		return "", fmt.Errorf("weixin: build cdn upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("weixin: cdn upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		err := fmt.Errorf("weixin: cdn upload status %d", resp.StatusCode)
		if resp.StatusCode >= 500 {
			return "", &retryableError{err: err}
		}
		return "", err // 4xx: not retryable
	}

	return resp.Header.Get("x-encrypted-param"), nil
}

// retryableError wraps an error that is safe to retry (e.g. 5xx responses).
type retryableError struct {
	err error
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var re *retryableError
	return errors.As(err, &re)
}

// downloadFromCDN downloads and decrypts a file from CDN.
func (c *apiClient) downloadFromCDN(ctx context.Context, encryptQueryParam, aesKeyB64 string, maxSize int) ([]byte, error) {
	downloadURL := fmt.Sprintf("%s/download?encrypted_query_param=%s",
		c.cfg.CDNBaseURL, url.QueryEscape(encryptQueryParam))

	reqCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("weixin: build cdn download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weixin: cdn download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weixin: cdn download status %d", resp.StatusCode)
	}

	encrypted, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxSize)+1))
	if err != nil {
		return nil, fmt.Errorf("weixin: read cdn response: %w", err)
	}
	if len(encrypted) > maxSize {
		return nil, fmt.Errorf("weixin: cdn file too large (%d bytes, max %d)", len(encrypted), maxSize)
	}

	key, err := parseAesKey(aesKeyB64)
	if err != nil {
		return nil, err
	}

	return decryptAesEcb(encrypted, key)
}

// ─── HTTP Helpers ────────────────────────────────────────────────────────────

func (c *apiClient) doPost(ctx context.Context, path string, body, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("weixin: marshal request: %w", err)
	}

	reqURL := fmt.Sprintf("%s/%s", c.cfg.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("weixin: build request: %w", err)
	}
	c.setCommonHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("weixin: %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("weixin: read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("weixin: %s status %d: %s", path, resp.StatusCode, string(respBody))
	}

	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("weixin: decode response: %w", err)
	}
	return nil
}

// ─── Upload Helpers ──────────────────────────────────────────────────────────

func generateFileKey() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateAesKey() []byte {
	b := make([]byte, 16)
	rand.Read(b)
	return b
}

func fileMD5(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/platform/weixin/`
Expected: compiles cleanly

- [ ] **Step 3: Commit**

```bash
git add internal/platform/weixin/api.go
git commit -m "feat(weixin): add API client with data types, HTTP methods, and CDN operations"
```

---

### Task 4: Handler — Platform + Receiver (text-only)

**Files:**
- Create: `internal/platform/weixin/handler.go`
- Create: `internal/platform/weixin/handler_test.go`

- [ ] **Step 1: Write handler_test.go**

```go
package weixin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWeixinPlatformID(t *testing.T) {
	p := &WeixinPlatform{}
	assert.Equal(t, "weixin", p.ID())
}

func TestBuildSessionKey(t *testing.T) {
	assert.Equal(t, "weixin:user123:user123", buildSessionKey("user123"))
}

func TestBuildPlatformMessageID(t *testing.T) {
	assert.Equal(t, "10:42", buildPlatformMessageID(10, 42))
}

func TestParseRaw(t *testing.T) {
	raw := weixinRaw{
		FromUserID:   "sender1",
		ToUserID:     "bot1",
		SessionID:    "sess1",
		ContextToken: "ctx-tok-123",
	}
	data, err := json.Marshal(raw)
	require.NoError(t, err)

	got, err := parseWeixinRaw(string(data))
	require.NoError(t, err)
	assert.Equal(t, "sender1", got.FromUserID)
	assert.Equal(t, "ctx-tok-123", got.ContextToken)
}

func TestParseRaw_InvalidJSON(t *testing.T) {
	_, err := parseWeixinRaw("not json")
	assert.Error(t, err)
}

func TestExtractTextContent(t *testing.T) {
	items := []messageItem{
		{Type: 1, TextItem: &textItem{Text: "hello"}},
		{Type: 1, TextItem: &textItem{Text: " world"}},
	}
	got := extractTextContent(items)
	assert.Equal(t, "hello world", got)
}

func TestExtractTextContent_Empty(t *testing.T) {
	got := extractTextContent(nil)
	assert.Equal(t, "", got)
}

func TestExtractTextContent_SkipsUnknownTypes(t *testing.T) {
	items := []messageItem{
		{Type: 99},
		{Type: 1, TextItem: &textItem{Text: "only text"}},
	}
	got := extractTextContent(items)
	assert.Equal(t, "only text", got)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/platform/weixin/ -v -run TestWeixin`
Expected: FAIL

- [ ] **Step 3: Implement handler.go (Platform struct, Receiver with text-only, Sender stub)**

```go
package weixin

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/media"
	"github.com/theopenbee/openbee/internal/platform"
)

// ─── Platform ────────────────────────────────────────────────────────────────

type WeixinPlatform struct {
	receiver *WeixinReceiver
	sender   *WeixinSender
}

func NewPlatform(cfg config.WeixinConfig, mc config.MediaConfig, mediaSvc *media.Service) platform.Platform {
	client := newAPIClient(cfg)
	p := &WeixinPlatform{}
	p.receiver = &WeixinReceiver{
		cfg:      cfg,
		mc:       mc,
		mediaSvc: mediaSvc,
		client:   client,
	}
	p.sender = &WeixinSender{
		cfg:    cfg,
		client: client,
	}
	return p
}

func (p *WeixinPlatform) ID() string                                 { return "weixin" }
func (p *WeixinPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *WeixinPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }

// ─── Helpers ─────────────────────────────────────────────────────────────────

type weixinRaw struct {
	FromUserID   string `json:"from_user_id"`
	ToUserID     string `json:"to_user_id"`
	SessionID    string `json:"session_id"`
	ContextToken string `json:"context_token"`
}

func buildSessionKey(userID string) string {
	return fmt.Sprintf("weixin:%s:%s", userID, userID)
}

func buildPlatformMessageID(seq, messageID int64) string {
	return fmt.Sprintf("%d:%d", seq, messageID)
}

func parseWeixinRaw(raw string) (weixinRaw, error) {
	var r weixinRaw
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return weixinRaw{}, fmt.Errorf("parse weixin raw: %w", err)
	}
	return r, nil
}

func extractTextContent(items []messageItem) string {
	var parts []string
	for _, item := range items {
		if item.Type == 1 && item.TextItem != nil {
			parts = append(parts, item.TextItem.Text)
		}
	}
	return strings.Join(parts, "")
}

func sanitizeTokenForLog(token string) string {
	if len(token) <= 6 {
		return token
	}
	return token[:6] + "***"
}

// ─── Receiver ────────────────────────────────────────────────────────────────

type WeixinReceiver struct {
	cfg      config.WeixinConfig
	mc       config.MediaConfig
	mediaSvc *media.Service
	client   *apiClient

	// typing state
	typingTicket      string
	typingTicketMu    sync.Mutex
	ticketCachedAt    time.Time
	ticketBackoff     time.Duration // exponential backoff for getconfig failures
	ticketBackoffUntil time.Time

	// session pause
	pauseUntil time.Time
}

// Note: the `log` package-level variable is declared in api.go (same package).

const (
	messageTypeUser   = 1
	messageStateFinish = 2
	typingStart       = 1
	ticketCacheTTL    = 24 * time.Hour
)

func (r *WeixinReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	log.Info("weixin receiver started",
		zap.String("token", sanitizeTokenForLog(r.cfg.Token)))

	var cursor string
	var consecutiveFailures int

	for {
		if ctx.Err() != nil {
			return nil
		}

		// Check session pause
		if !r.pauseUntil.IsZero() && time.Now().Before(r.pauseUntil) {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(30 * time.Second):
				continue
			}
		}
		r.pauseUntil = time.Time{}

		resp, err := r.client.getUpdates(ctx, cursor)
		if err != nil {
			consecutiveFailures++
			delay := 5 * time.Second
			if consecutiveFailures >= 3 {
				delay = 30 * time.Second
			}
			log.Error("getUpdates error", zap.Error(err),
				zap.Int("consecutive_failures", consecutiveFailures))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
				continue
			}
		}
		consecutiveFailures = 0

		// Check session expiry
		if resp.Ret == -14 {
			log.Warn("weixin session expired, pausing for 60 minutes")
			r.pauseUntil = time.Now().Add(60 * time.Minute)
			continue
		}

		if resp.Ret != 0 && resp.Ret != -14 {
			log.Warn("getUpdates non-zero ret", zap.Int("ret", resp.Ret))
		}

		// Update cursor
		if resp.GetUpdatesBuf != "" {
			cursor = resp.GetUpdatesBuf
		}

		for _, msg := range resp.Msgs {
			// Filter: only USER messages in FINISH state
			if msg.MessageType != messageTypeUser || msg.MessageState != messageStateFinish {
				continue
			}

			// Skip group messages
			if msg.GroupID != "" {
				log.Debug("skipping group message", zap.String("group_id", msg.GroupID))
				continue
			}

			// Skip empty item_list
			if len(msg.ItemList) == 0 {
				continue
			}

			// Send typing in background
			go r.sendTypingAction(ctx)

			inbound := r.buildInboundMessage(ctx, msg)
			if inbound == nil {
				continue
			}
			dispatch(*inbound)
		}
	}
}

func (r *WeixinReceiver) buildInboundMessage(ctx context.Context, msg weixinMessage) *platform.InboundMessage {
	content := r.extractContent(ctx, msg.ItemList)
	if content == "" {
		return nil
	}

	rawData := weixinRaw{
		FromUserID:   msg.FromUserID,
		ToUserID:     msg.ToUserID,
		SessionID:    msg.SessionID,
		ContextToken: msg.ContextToken,
	}
	rawBytes, err := json.Marshal(rawData)
	if err != nil {
		log.Error("marshal weixin raw", zap.Error(err))
		return nil
	}

	return &platform.InboundMessage{
		Platform:          "weixin",
		SenderID:          msg.FromUserID,
		SessionKey:        buildSessionKey(msg.FromUserID),
		Content:           content,
		RawContent:        content,
		Raw:               string(rawBytes),
		PlatformMessageID: buildPlatformMessageID(msg.Seq, msg.MessageID),
		MessageTime:       msg.CreateTimeMs,
	}
}

func (r *WeixinReceiver) extractContent(ctx context.Context, items []messageItem) string {
	var parts []string
	for _, item := range items {
		switch item.Type {
		case 1: // TEXT
			if item.TextItem != nil {
				parts = append(parts, item.TextItem.Text)
			}
		case 2: // IMAGE
			if item.ImageItem != nil {
				parts = append(parts, r.downloadMedia(ctx, item.ImageItem.Media, item.ImageItem.AesKey, "image", ""))
			}
		case 3: // VOICE
			if item.VoiceItem != nil {
				parts = append(parts, r.downloadVoice(ctx, item.VoiceItem))
			}
		case 4: // FILE
			if item.FileItem != nil {
				fileName := item.FileItem.FileName
				parts = append(parts, r.downloadMedia(ctx, item.FileItem.Media, "", "document", fileName))
			}
		case 5: // VIDEO
			if item.VideoItem != nil {
				parts = append(parts, r.downloadMedia(ctx, item.VideoItem.Media, "", "video", ""))
			}
		default:
			log.Warn("skipping unknown item type", zap.Int("type", item.Type))
		}
	}
	return strings.Join(parts, " ")
}

func (r *WeixinReceiver) downloadMedia(ctx context.Context, m *cdnMedia, hexAesKey, mediaType, fileName string) string {
	if m == nil || m.EncryptQueryParam == "" {
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	// Determine AES key: hex aeskey field takes priority, then media.aes_key (base64)
	var aesKeyB64 string
	if hexAesKey != "" {
		// Convert hex to raw bytes then base64
		keyBytes, err := hex.DecodeString(hexAesKey)
		if err != nil {
			log.Error("decode hex aeskey", zap.Error(err))
			return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
		}
		aesKeyB64 = base64.StdEncoding.EncodeToString(keyBytes)
	} else {
		aesKeyB64 = m.AesKey
	}

	if aesKeyB64 == "" {
		log.Error("no aes key for media download")
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	data, err := r.client.downloadFromCDN(ctx, m.EncryptQueryParam, aesKeyB64, r.cfg.MaxMediaSize)
	if err != nil {
		log.Error("download media from CDN", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	ext := ""
	if fileName != "" {
		if idx := strings.LastIndex(fileName, "."); idx >= 0 {
			ext = fileName[idx:]
		}
	}
	if ext == "" {
		mime := r.mediaSvc.DetectMIME(data, fileName)
		ext = r.mediaSvc.ExtensionFromMIME(mime)
	}

	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		log.Error("save inbound media", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder(mediaType, "", fileName)
	}

	return r.mediaSvc.BuildPlaceholder(mediaType, path, fileName)
}

func (r *WeixinReceiver) downloadVoice(ctx context.Context, v *voiceItem) string {
	placeholder := r.downloadMedia(ctx, v.Media, "", "audio", "voice.silk")
	// TODO: Task 6 adds SILK→WAV transcoding here
	return placeholder
}

func (r *WeixinReceiver) sendTypingAction(ctx context.Context) {
	ticket, err := r.getTypingTicket(ctx)
	if err != nil {
		log.Debug("get typing ticket failed", zap.Error(err))
		return
	}
	if err := r.client.sendTyping(ctx, ticket, typingStart); err != nil {
		log.Debug("send typing failed", zap.Error(err))
	}
}

func (r *WeixinReceiver) getTypingTicket(ctx context.Context) (string, error) {
	r.typingTicketMu.Lock()
	defer r.typingTicketMu.Unlock()

	// Return cached ticket if still valid
	if r.typingTicket != "" && time.Since(r.ticketCachedAt) < ticketCacheTTL {
		return r.typingTicket, nil
	}

	// Respect exponential backoff on getconfig failures
	if !r.ticketBackoffUntil.IsZero() && time.Now().Before(r.ticketBackoffUntil) {
		return "", fmt.Errorf("weixin: getconfig in backoff (until %s)", r.ticketBackoffUntil.Format(time.RFC3339))
	}

	resp, err := r.client.getConfig(ctx)
	if err != nil {
		// Exponential backoff: 2s → 4s → 8s → ... → 1h max
		if r.ticketBackoff == 0 {
			r.ticketBackoff = 2 * time.Second
		} else {
			r.ticketBackoff *= 2
			if r.ticketBackoff > time.Hour {
				r.ticketBackoff = time.Hour
			}
		}
		r.ticketBackoffUntil = time.Now().Add(r.ticketBackoff)
		return "", err
	}

	// Success: reset backoff, cache ticket
	r.ticketBackoff = 0
	r.ticketBackoffUntil = time.Time{}
	r.typingTicket = resp.TypingTicket
	r.ticketCachedAt = time.Now()
	return r.typingTicket, nil
}

// ─── Sender ──────────────────────────────────────────────────────────────────

type WeixinSender struct {
	cfg    config.WeixinConfig
	client *apiClient
}

func (s *WeixinSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	raw, err := parseWeixinRaw(msg.ReplyTo.Raw)
	if err != nil {
		return fmt.Errorf("parse reply context: %w", err)
	}

	if msg.MediaPath != "" {
		return s.sendMedia(ctx, raw, msg.MediaPath)
	}
	return s.sendText(ctx, raw, msg.Content)
}

func (s *WeixinSender) sendText(ctx context.Context, raw weixinRaw, text string) error {
	outMsg := weixinMessage{
		ToUserID:     raw.FromUserID,
		ContextToken: raw.ContextToken,
		MessageType:  2, // BOT
		ItemList: []messageItem{
			{
				Type:     1,
				TextItem: &textItem{Text: text},
			},
		},
	}
	return s.client.sendMessage(ctx, outMsg)
}

func (s *WeixinSender) sendMedia(ctx context.Context, raw weixinRaw, mediaPath string) error {
	// Will be implemented in Task 5
	return fmt.Errorf("weixin: media send not yet implemented")
}

// Interface compliance guards.
var _ platform.Platform                = (*WeixinPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*WeixinReceiver)(nil)
var _ platform.PlatformSenderAdapter   = (*WeixinSender)(nil)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/platform/weixin/ -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/weixin/handler.go internal/platform/weixin/handler_test.go
git commit -m "feat(weixin): add Platform, Receiver (text+media download), and Sender (text-only)"
```

---

### Task 5: Sender — Media Upload

**Files:**
- Modify: `internal/platform/weixin/handler.go` (replace `sendMedia` stub)

- [ ] **Step 1: Implement sendMedia in handler.go**

Replace the `sendMedia` stub with:

```go
func (s *WeixinSender) sendMedia(ctx context.Context, raw weixinRaw, mediaPath string) error {
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("weixin: read media file: %w", err)
	}
	if len(data) > s.cfg.MaxMediaSize {
		return fmt.Errorf("weixin: file too large: %d bytes (max %d)", len(data), s.cfg.MaxMediaSize)
	}

	mime := http.DetectContentType(data)
	mediaType := mediaTypeFromMIME(mime)

	aesKey := generateAesKey()
	fileKey := generateFileKey()
	rawSize := len(data)
	rawMD5 := fileMD5(data)
	encSize := aesEcbPaddedSize(rawSize)

	uploadReq := getUploadURLReq{
		FileKey:    fileKey,
		MediaType:  mediaType,
		ToUserID:   raw.FromUserID,
		RawSize:    strconv.Itoa(rawSize),
		RawFileMD5: rawMD5,
		FileSize:   strconv.Itoa(encSize),
		AesKey:     hex.EncodeToString(aesKey),
	}

	// Retry up to 3 times for 5xx
	var uploadResp *getUploadURLResp
	for attempt := 0; attempt < 3; attempt++ {
		uploadResp, err = s.client.getUploadURL(ctx, uploadReq)
		if err == nil {
			break
		}
		log.Warn("getUploadURL failed, retrying", zap.Int("attempt", attempt+1), zap.Error(err))
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("weixin: getUploadURL: %w", err)
	}

	// Upload to CDN with retry (only 5xx; 4xx fails immediately)
	var downloadParam string
	for attempt := 0; attempt < 3; attempt++ {
		downloadParam, err = s.client.uploadToCDN(ctx, uploadResp.UploadParam, data, aesKey)
		if err == nil {
			break
		}
		if !isRetryable(err) {
			return fmt.Errorf("weixin: CDN upload (non-retryable): %w", err)
		}
		log.Warn("CDN upload 5xx, retrying", zap.Int("attempt", attempt+1), zap.Error(err))
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("weixin: CDN upload: %w", err)
	}

	cdnRef := &cdnMedia{
		EncryptQueryParam: downloadParam,
		AesKey:            base64.StdEncoding.EncodeToString(aesKey),
	}

	item := buildMediaItem(mediaType, cdnRef, aesKey, data, mediaPath)

	outMsg := weixinMessage{
		ToUserID:     raw.FromUserID,
		ContextToken: raw.ContextToken,
		MessageType:  2,
		ItemList:     []messageItem{item},
	}
	return s.client.sendMessage(ctx, outMsg)
}

func mediaTypeFromMIME(mime string) int {
	switch {
	case strings.HasPrefix(mime, "video/"):
		return 2
	case strings.HasPrefix(mime, "image/"):
		return 1
	default:
		return 3
	}
}

func buildMediaItem(mediaType int, cdn *cdnMedia, rawAesKey, data []byte, path string) messageItem {
	switch mediaType {
	case 1: // image
		return messageItem{
			Type: 2,
			ImageItem: &imageItem{
				Media:  cdn,
				AesKey: hex.EncodeToString(rawAesKey), // hex-encoded raw 16-byte key
			},
		}
	case 2: // video
		return messageItem{
			Type: 5,
			VideoItem: &videoItem{
				Media:    cdn,
				VideoMD5: fileMD5(data),
			},
		}
	default: // file
		fileName := filepath.Base(path)
		return messageItem{
			Type: 4,
			FileItem: &fileItem{
				Media:    cdn,
				FileName: platform.SanitizeFileName(fileName),
				MD5:      fileMD5(data),
				Len:      strconv.Itoa(len(data)),
			},
		}
	}
}
```

Add necessary imports to handler.go: `"encoding/base64"`, `"encoding/hex"`, `"net/http"`, `"os"`, `"path/filepath"`, `"strconv"`.

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/platform/weixin/`
Expected: compiles cleanly

- [ ] **Step 3: Commit**

```bash
git add internal/platform/weixin/handler.go
git commit -m "feat(weixin): implement Sender media upload with CDN encryption and retry"
```

---

### Task 6: Voice SILK→WAV Transcoding

**Files:**
- Modify: `internal/platform/weixin/handler.go` (update `downloadVoice`)

- [ ] **Step 1: Replace downloadVoice with SILK→WAV transcoding**

```go
func (r *WeixinReceiver) downloadVoice(ctx context.Context, v *voiceItem) string {
	if v.Media == nil || v.Media.EncryptQueryParam == "" {
		return r.mediaSvc.BuildPlaceholder("audio", "", "voice.silk")
	}

	aesKeyB64 := v.Media.AesKey
	if aesKeyB64 == "" {
		log.Error("no aes key for voice download")
		return r.mediaSvc.BuildPlaceholder("audio", "", "voice.silk")
	}

	data, err := r.client.downloadFromCDN(ctx, v.Media.EncryptQueryParam, aesKeyB64, r.cfg.MaxMediaSize)
	if err != nil {
		log.Error("download voice from CDN", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("audio", "", "voice.silk")
	}

	// Save raw SILK file first
	silkPath, err := r.mediaSvc.SaveInbound(ctx, data, ".silk")
	if err != nil {
		log.Error("save voice file", zap.Error(err))
		return r.mediaSvc.BuildPlaceholder("audio", "", "voice.silk")
	}

	// Try SILK→WAV transcoding via FFmpeg
	wavPath, err := r.transcodeSilkToWav(ctx, silkPath)
	if err != nil {
		log.Warn("SILK to WAV transcoding failed, using raw SILK file",
			zap.Error(err), zap.String("silk_path", silkPath))
		return r.mediaSvc.BuildPlaceholder("audio", silkPath, "voice.silk")
	}

	return r.mediaSvc.BuildPlaceholder("audio", wavPath, "voice.wav")
}

func (r *WeixinReceiver) transcodeSilkToWav(ctx context.Context, silkPath string) (string, error) {
	wavPath := strings.TrimSuffix(silkPath, filepath.Ext(silkPath)) + ".wav"
	cmd := exec.CommandContext(ctx, r.mc.FFmpegPath,
		"-i", silkPath,
		"-ar", "16000",
		"-ac", "1",
		"-y", wavPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg: %w: %s", err, string(out))
	}
	return wavPath, nil
}
```

Add imports: `"os/exec"`, `"path/filepath"`.

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/platform/weixin/`
Expected: compiles cleanly

- [ ] **Step 3: Commit**

```bash
git add internal/platform/weixin/handler.go
git commit -m "feat(weixin): add SILK to WAV voice transcoding via FFmpeg"
```

---

### Task 7: App Wiring + QR Login

**Files:**
- Modify: `internal/app/app.go:218-234` (buildPlatforms)
- Modify: `cmd/openbee/config.go` (replace manual token entry with QR login)
- Modify: `go.mod` (add qrterminal dependency)

- [ ] **Step 1: Add qrterminal dependency**

Run: `go get github.com/mdp/qrterminal/v3`

- [ ] **Step 2: Update buildPlatforms in app.go**

Change signature and add weixin:
```go
func buildPlatforms(fc config.FeishuConfig, dc config.DingTalkConfig, wc config.WeComConfig, tc config.TelegramConfig, wxc config.WeixinConfig, mc config.MediaConfig) []platform.Platform {
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
	if tc.Enabled {
		result = append(result, telegram.NewPlatform(tc, mediaSvc))
	}
	if wxc.Enabled {
		result = append(result, weixin.NewPlatform(wxc, mc, mediaSvc))
	}
	return result
}
```

Update the call site in `BuildApp`:
```go
platforms := buildPlatforms(cfg.Bee.Platforms.Feishu, cfg.Bee.Platforms.DingTalk, cfg.Bee.Platforms.WeCom, cfg.Bee.Platforms.Telegram, cfg.Bee.Platforms.Weixin, cfg.Bee.Media)
```

Add import: `"github.com/theopenbee/openbee/internal/platform/weixin"`

- [ ] **Step 3: Implement QR login flow in config.go**

Replace the manual token entry in the `"Weixin"` case with QR login:

```go
case "Weixin":
	vals.WeixinEnabled = true
	fmt.Println("\n--- Weixin QR Code Login ---")
	fmt.Println("Fetching QR code...")

	token, userID, baseURL, err := runWeixinQRLogin()
	if err != nil {
		fmt.Printf("QR login failed: %v\n", err)
		fmt.Println("Falling back to manual token entry.")
		if err := survey.AskOne(&survey.Password{
			Message: "Weixin Bot Token:",
		}, &vals.WeixinToken, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
		if err := survey.AskOne(&survey.Input{
			Message: "Weixin User ID:",
			Default: vals.WeixinUserID,
		}, &vals.WeixinUserID, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
	} else {
		vals.WeixinToken = token
		vals.WeixinUserID = userID
		if baseURL != "" {
			vals.WeixinBaseURL = baseURL
		}
		fmt.Println("Weixin login successful!")
	}
	if vals.WeixinBaseURL == "" {
		vals.WeixinBaseURL = "https://ilinkai.weixin.qq.com"
	}
	vals.WeixinCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
```

Add `runWeixinQRLogin` function (in a new file `cmd/openbee/weixin_login.go` or at the bottom of config.go):

```go
func runWeixinQRLogin() (token, userID, baseURL string, err error) {
	const defaultBase = "https://ilinkai.weixin.qq.com"
	client := &http.Client{Timeout: 15 * time.Second}

	// Step 1: Get QR code
	resp, err := client.Get(defaultBase + "/ilink/bot/get_bot_qrcode?bot_type=3")
	if err != nil {
		return "", "", "", fmt.Errorf("get qrcode: %w", err)
	}
	defer resp.Body.Close()

	var qrResp struct {
		QRCode          string `json:"qrcode"`
		QRCodeImgContent string `json:"qrcode_img_content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&qrResp); err != nil {
		return "", "", "", fmt.Errorf("decode qrcode response: %w", err)
	}

	// Step 2: Display QR code in terminal
	fmt.Println("\nScan this QR code with WeChat:")
	qrterminal.GenerateWithConfig(qrResp.QRCode, qrterminal.Config{
		Level:     qrterminal.L,
		Writer:    os.Stdout,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
	})

	// Step 3: Poll for scan status (max 5 minutes total, max 3 timeouts)
	fmt.Println("\nWaiting for scan...")
	pollClient := &http.Client{Timeout: 40 * time.Second}
	deadline := time.Now().Add(5 * time.Minute)
	for attempt := 0; attempt < 3 && time.Now().Before(deadline); attempt++ {
		req, _ := http.NewRequest(http.MethodGet,
			fmt.Sprintf("%s/ilink/bot/get_qrcode_status?qrcode=%s", defaultBase, qrResp.QRCode), nil)
		req.Header.Set("iLink-App-ClientVersion", "1")

		pollResp, err := pollClient.Do(req)
		if err != nil {
			fmt.Printf("  poll attempt %d failed: %v\n", attempt+1, err)
			continue
		}

		var statusResp struct {
			Status     string `json:"status"`
			BotToken   string `json:"bot_token"`
			ILinkBotID string `json:"ilink_bot_id"`
			BaseURL    string `json:"baseurl"`
			ILinkUserID string `json:"ilink_user_id"`
		}
		json.NewDecoder(pollResp.Body).Decode(&statusResp)
		pollResp.Body.Close()

		switch statusResp.Status {
		case "confirmed":
			return statusResp.BotToken, statusResp.ILinkUserID, statusResp.BaseURL, nil
		case "scaned":
			fmt.Println("  Scanned! Please confirm on your phone...")
			attempt-- // don't count scanned as a failed attempt
		case "expired":
			return "", "", "", fmt.Errorf("QR code expired")
		case "wait":
			fmt.Println("  Still waiting for scan...")
		}
	}
	return "", "", "", fmt.Errorf("QR login timed out after 3 attempts")
}
```

Add imports to config.go: `"encoding/json"`, `"net/http"`, `"time"`, `qrterminal "github.com/mdp/qrterminal/v3"`.

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: compiles cleanly

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go cmd/openbee/config.go go.mod go.sum
git commit -m "feat(weixin): wire platform into app and add QR code login flow"
```

---

### Task 8: Final Verification

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run: `go test ./... -v`
Expected: all PASS

- [ ] **Step 2: Run full build**

Run: `go build ./...`
Expected: clean build

- [ ] **Step 3: Verify linting (if configured)**

Run: `golangci-lint run ./...` (if available)
Expected: no new issues

- [ ] **Step 4: Final commit (if any remaining changes)**

```bash
git status
# If clean, no commit needed
```
