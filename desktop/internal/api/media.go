package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

var maxUploadSize int64 = 100 << 20

func mediaBasename(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

func (c *Client) UploadMedia(ctx context.Context, name string, r io.Reader) (string, error) {
	if name == "" {
		return "", errors.New("upload file name is empty")
	}
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	part, err := form.CreateFormFile("file", name)
	if err != nil {
		return "", fmt.Errorf("encode upload: %w", err)
	}
	n, err := io.Copy(part, io.LimitReader(r, maxUploadSize+1))
	if err != nil {
		return "", fmt.Errorf("read upload: %w", err)
	}
	if n > maxUploadSize {
		return "", fmt.Errorf("file is larger than %d bytes", maxUploadSize)
	}
	if err := form.Close(); err != nil {
		return "", fmt.Errorf("encode upload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+apiPrefix+"/local/media", bytes.NewReader(buf.Bytes()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	resp, err := c.DoRaw(ctx, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Path string `json:"path"`
	}
	if err := decodeResponse(resp, &out); err != nil {
		return "", err
	}
	if out.Path == "" {
		return "", errors.New("upload response has no path")
	}
	return out.Path, nil
}

func (c *Client) DownloadMedia(ctx context.Context, path string, w io.Writer) (string, error) {
	name := mediaBasename(path)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("invalid media path %q", path)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+apiPrefix+"/local/media/"+escapeSegment(name), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "*/*")
	resp, err := c.DoRaw(ctx, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if _, err := io.Copy(w, resp.Body); err != nil {
		return "", fmt.Errorf("download media: %w", err)
	}
	return mediaType(resp.Header.Get("Content-Type")), nil
}
