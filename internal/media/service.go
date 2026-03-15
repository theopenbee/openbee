package media

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service handles media file operations: saving, MIME detection, placeholder building.
type Service struct {
	baseDir string
}

// NewService creates a Service with baseDir at ~/.robobee/media and ensures inbound/ exists.
func NewService() *Service {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".robobee", "media")
	os.MkdirAll(filepath.Join(baseDir, "inbound"), 0o755)
	return &Service{baseDir: baseDir}
}

// SaveInbound writes data to ~/.robobee/media/inbound/<timestamp>-<uuid>.<ext> and returns the path.
func (s *Service) SaveInbound(_ context.Context, data []byte, ext string) (string, error) {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	name := fmt.Sprintf("%d-%s%s", time.Now().Unix(), uuid.New().String()[:12], ext)
	path := filepath.Join(s.baseDir, "inbound", name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("save inbound media: %w", err)
	}
	return path, nil
}

// DetectMIME detects the MIME type from file content bytes, falling back to extension-based mapping.
func (s *Service) DetectMIME(data []byte, fileName string) string {
	if len(data) > 0 {
		ct := http.DetectContentType(data)
		if ct != "application/octet-stream" && ct != "text/plain; charset=utf-8" {
			return ct
		}
	}
	if fileName != "" {
		return mimeFromExtension(filepath.Ext(fileName))
	}
	if len(data) > 0 {
		return http.DetectContentType(data)
	}
	return "application/octet-stream"
}

// ExtensionFromMIME maps a MIME type to a file extension (with leading dot).
func (s *Service) ExtensionFromMIME(contentType string) string {
	ct := strings.Split(contentType, ";")[0]
	ct = strings.TrimSpace(ct)
	switch ct {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	case "image/x-icon", "image/vnd.microsoft.icon":
		return ".ico"
	case "audio/ogg", "audio/opus":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/amr":
		return ".amr"
	case "audio/aac":
		return ".aac"
	case "audio/flac":
		return ".flac"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/x-msvideo":
		return ".avi"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}

// BuildPlaceholder builds a content placeholder string for embedding in message Content.
func (s *Service) BuildPlaceholder(mediaType string, path string, fileName string) string {
	var attrs []string
	if fileName != "" {
		attrs = append(attrs, fmt.Sprintf("name=%q", fileName))
	}
	if path != "" {
		attrs = append(attrs, fmt.Sprintf("path=%q", path))
	}
	if len(attrs) == 0 {
		return fmt.Sprintf("<media:%s>", mediaType)
	}
	return fmt.Sprintf("<media:%s %s>", mediaType, strings.Join(attrs, " "))
}

// MediaTypeFromMIME maps a MIME type prefix to a media type string.
func MediaTypeFromMIME(contentType string) string {
	ct := strings.Split(contentType, ";")[0]
	ct = strings.TrimSpace(ct)
	switch {
	case strings.HasPrefix(ct, "image/"):
		return "image"
	case strings.HasPrefix(ct, "audio/"):
		return "audio"
	case strings.HasPrefix(ct, "video/"):
		return "video"
	default:
		return "document"
	}
}

// mimeFromExtension maps common file extensions to MIME types.
func mimeFromExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".pdf":
		return "application/pdf"
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}
