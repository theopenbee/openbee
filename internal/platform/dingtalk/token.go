package dingtalk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// --- New API token (api.dingtalk.com) ---

var (
	apiTokenCache struct {
		token     string
		expiresAt time.Time
	}
	apiTokenMu sync.Mutex
)

// getAccessToken obtains an access token from the DingTalk OAuth2 API,
// caching the result to avoid redundant requests.
func getAccessToken(clientID, clientSecret string) (string, error) {
	apiTokenMu.Lock()
	defer apiTokenMu.Unlock()

	if apiTokenCache.token != "" && time.Now().Add(60*time.Second).Before(apiTokenCache.expiresAt) {
		return apiTokenCache.token, nil
	}

	body, _ := json.Marshal(map[string]string{
		"appKey":    clientID,
		"appSecret": clientSecret,
	})
	resp, err := http.Post("https://api.dingtalk.com/v1.0/oauth2/accessToken", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("request access token: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int64  `json:"expireIn"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode access token response: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}

	apiTokenCache.token = result.AccessToken
	if result.ExpireIn > 0 {
		apiTokenCache.expiresAt = time.Now().Add(time.Duration(result.ExpireIn) * time.Second)
	} else {
		apiTokenCache.expiresAt = time.Now().Add(1 * time.Hour)
	}

	return result.AccessToken, nil
}

// --- Legacy OAPI token (oapi.dingtalk.com) — for media upload ---

var (
	oapiTokenCache struct {
		token     string
		expiresAt time.Time
	}
	oapiTokenMu sync.Mutex
)

// getOAPIToken obtains a legacy OAPI access token for media upload operations.
func getOAPIToken(clientID, clientSecret string) (string, error) {
	oapiTokenMu.Lock()
	defer oapiTokenMu.Unlock()

	if oapiTokenCache.token != "" && time.Now().Add(60*time.Second).Before(oapiTokenCache.expiresAt) {
		return oapiTokenCache.token, nil
	}

	url := fmt.Sprintf("https://oapi.dingtalk.com/gettoken?appkey=%s&appsecret=%s", clientID, clientSecret)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("request OAPI token: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode OAPI token response: %w", err)
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("OAPI token error %d: %s", result.ErrCode, result.ErrMsg)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("empty OAPI access token")
	}

	oapiTokenCache.token = result.AccessToken
	if result.ExpiresIn > 0 {
		oapiTokenCache.expiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	} else {
		oapiTokenCache.expiresAt = time.Now().Add(1 * time.Hour)
	}

	return result.AccessToken, nil
}
