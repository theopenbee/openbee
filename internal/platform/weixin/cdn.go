package weixin

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	cdnDownloadClient = &http.Client{Timeout: 120 * time.Second}
	cdnUploadClient   = &http.Client{Timeout: 60 * time.Second}
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

	resp, err := cdnDownloadClient.Do(req)
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

func doUpload(ctx context.Context, uploadURL string, data []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := cdnUploadClient.Do(req)
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

// mediaCDNType maps MIME prefix to WeChat CDN media type constant.
func mediaCDNType(mimeType string) int {
	switch {
	case len(mimeType) >= 5 && mimeType[:5] == "image":
		return CDNMediaTypeImage
	case len(mimeType) >= 5 && mimeType[:5] == "video":
		return CDNMediaTypeVideo
	case len(mimeType) >= 5 && mimeType[:5] == "audio":
		return CDNMediaTypeVoice
	default:
		return CDNMediaTypeFile
	}
}
