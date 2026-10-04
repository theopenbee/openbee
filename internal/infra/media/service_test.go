package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		got := MediaTypeFromMIME(tt.mime)
		assert.Equal(t, tt.want, got, "mime=%q", tt.mime)
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
		assert.Equal(t, tt.want, got, "mediaType=%q path=%q fileName=%q", tt.mediaType, tt.path, tt.fileName)
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
		got := s.ExtensionFromMIME(tt.mime)
		assert.Equal(t, tt.want, got, "mime=%q", tt.mime)
	}
}

func TestDetectMIME(t *testing.T) {
	s := &Service{}

	// PNG magic bytes
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	assert.Equal(t, "image/png", s.DetectMIME(png, ""))

	// Fallback to extension
	assert.Equal(t, "application/pdf", s.DetectMIME([]byte("hello"), "file.pdf"))

	// Plain text with no filename — stdlib detects as text/plain
	assert.Equal(t, "text/plain; charset=utf-8", s.DetectMIME([]byte("hello"), ""))

	// OGG magic bytes (Opus audio in OGG container)
	ogg := []byte{'O', 'g', 'g', 'S', 0x00, 0x02, 0x00, 0x00}
	assert.Equal(t, "audio/ogg", s.DetectMIME(ogg, ""))

	// OGG detection should take priority even with a filename
	assert.Equal(t, "audio/ogg", s.DetectMIME(ogg, "voice.bin"))
}

func TestSaveInbound(t *testing.T) {
	dir := t.TempDir()
	s := &Service{baseDir: dir}
	os.MkdirAll(filepath.Join(dir, "inbound"), 0o755)

	path, err := s.SaveInbound(context.Background(), []byte("hello"), ".txt")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(path, filepath.Join(dir, "inbound")), "path %q not under inbound dir", path)
	assert.True(t, strings.HasSuffix(path, ".txt"), "path %q should end with .txt", path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}
