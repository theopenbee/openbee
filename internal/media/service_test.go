package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediaTypeFromMIME(t *testing.T) {
	tests := []struct {
		mime string
		want string
	}{
		{"image/png", "image"},
		{"image/jpeg", "image"},
		{"audio/mpeg", "audio"},
		{"audio/ogg", "audio"},
		{"video/mp4", "video"},
		{"video/quicktime", "video"},
		{"application/pdf", "document"},
		{"application/octet-stream", "document"},
		{"text/plain", "document"},
		{"", "document"},
	}
	for _, tt := range tests {
		if got := MediaTypeFromMIME(tt.mime); got != tt.want {
			t.Errorf("MediaTypeFromMIME(%q) = %q, want %q", tt.mime, got, tt.want)
		}
	}
}

func TestBuildPlaceholder(t *testing.T) {
	s := &Service{baseDir: "/tmp/test"}

	tests := []struct {
		mediaType string
		path      string
		fileName  string
		want      string
	}{
		{"image", "/tmp/test/inbound/x.png", "", `<media:image path="/tmp/test/inbound/x.png">`},
		{"document", "/tmp/f.pdf", "report.pdf", `<media:document name="report.pdf" path="/tmp/f.pdf">`},
		{"audio", "/tmp/a.opus", "", `<media:audio path="/tmp/a.opus">`},
		{"video", "/tmp/v.mp4", "", `<media:video path="/tmp/v.mp4">`},
		{"sticker", "/tmp/s.png", "", `<media:sticker path="/tmp/s.png">`},
		{"image", "", "", `<media:image>`},
	}
	for _, tt := range tests {
		got := s.BuildPlaceholder(tt.mediaType, tt.path, tt.fileName)
		if got != tt.want {
			t.Errorf("BuildPlaceholder(%q, %q, %q) = %q, want %q", tt.mediaType, tt.path, tt.fileName, got, tt.want)
		}
	}
}

func TestExtensionFromMIME(t *testing.T) {
	s := &Service{}
	tests := []struct {
		mime string
		want string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"audio/ogg", ".ogg"},
		{"audio/mpeg", ".mp3"},
		{"video/mp4", ".mp4"},
		{"application/pdf", ".pdf"},
		{"application/octet-stream", ".bin"},
		{"", ".bin"},
	}
	for _, tt := range tests {
		if got := s.ExtensionFromMIME(tt.mime); got != tt.want {
			t.Errorf("ExtensionFromMIME(%q) = %q, want %q", tt.mime, got, tt.want)
		}
	}
}

func TestDetectMIME(t *testing.T) {
	s := &Service{}

	// PNG magic bytes
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if got := s.DetectMIME(png, ""); got != "image/png" {
		t.Errorf("DetectMIME(png) = %q, want image/png", got)
	}

	// Fallback to extension
	if got := s.DetectMIME([]byte("hello"), "file.pdf"); got != "application/pdf" {
		t.Errorf("DetectMIME(text, file.pdf) = %q, want application/pdf", got)
	}

	// Plain text with no filename — stdlib detects as text/plain
	if got := s.DetectMIME([]byte("hello"), ""); got != "text/plain; charset=utf-8" {
		t.Errorf("DetectMIME(text, empty) = %q, want text/plain; charset=utf-8", got)
	}
}

func TestSaveInbound(t *testing.T) {
	dir := t.TempDir()
	s := &Service{baseDir: dir}
	os.MkdirAll(filepath.Join(dir, "inbound"), 0o755)

	path, err := s.SaveInbound(context.Background(), []byte("hello"), ".txt")
	if err != nil {
		t.Fatalf("SaveInbound: %v", err)
	}
	if !strings.HasPrefix(path, filepath.Join(dir, "inbound")) {
		t.Errorf("path %q not under inbound dir", path)
	}
	if !strings.HasSuffix(path, ".txt") {
		t.Errorf("path %q should end with .txt", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("file content = %q, want hello", string(data))
	}
}
