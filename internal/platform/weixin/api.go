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
	Ret    int    `json:"ret"`
	ErrMsg string `json:"errmsg"`
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

	log.Info("sendMessage request",
		zap.String("to_user_id", msg.ToUserID),
		zap.String("session_id", msg.SessionID),
		zap.Bool("has_context_token", msg.ContextToken != ""),
		zap.Int("message_type", msg.MessageType),
		zap.Int("message_state", msg.MessageState),
		zap.String("client_id", msg.ClientID),
		zap.Int("item_count", len(msg.ItemList)))

	var resp sendMessageResp
	if err := c.doPost(reqCtx, "ilink/bot/sendmessage", body, &resp); err != nil {
		return err
	}

	log.Info("sendMessage response",
		zap.Int("ret", resp.Ret),
		zap.String("errmsg", resp.ErrMsg))

	if resp.Ret != 0 {
		return fmt.Errorf("weixin sendmessage ret=%d errmsg=%s", resp.Ret, resp.ErrMsg)
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

func generateClientID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func fileMD5(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}
