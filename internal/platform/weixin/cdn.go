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
