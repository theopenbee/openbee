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
	var resp struct {
		Ret int `json:"ret"`
	}
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
	var resp struct {
		Ret int `json:"ret"`
	}
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
	if _, err := rand.Read(buf[:]); err != nil {
		return "AAAAAAA=" // fallback: non-random but valid base64
	}
	return base64.StdEncoding.EncodeToString(buf[:])
}
