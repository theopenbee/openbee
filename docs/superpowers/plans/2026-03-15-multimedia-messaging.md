# Multimedia Messaging Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add inbound media reception (download, parse, extract text) and outbound media sending (upload, send) for both Feishu and DingTalk platforms.

**Architecture:** Shared `internal/media/` service handles common operations (save to disk, MIME detection, text extraction, placeholder building). Platform-specific download/upload logic stays in each platform's handler. Media info is embedded in the `Content` field as structured placeholders with file paths, so existing pipeline (DB, feeder, bee, worker) needs no changes.

**Tech Stack:** Go stdlib (`net/http`, `mime/multipart`), Lark SDK (`larksuite/oapi-sdk-go/v3`), `ledongthuc/pdf`, `nguyenthenguyen/docx`

**Spec:** `docs/superpowers/specs/2026-03-15-multimedia-messaging-design.md`

---

## Chunk 1: Media Service + Dependencies

### Task 1: Add dependencies

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add PDF and DOCX libraries**

```bash
cd /Users/tengyongzhi/work/robobee && go get github.com/ledongthuc/pdf github.com/nguyenthenguyen/docx
```

- [ ] **Step 2: Verify dependencies resolve**

Run: `go mod tidy`
Expected: Clean exit, no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add PDF and DOCX extraction libraries"
```

---

### Task 2: Media service — placeholder building and MIME utilities

**Files:**
- Create: `internal/media/service.go`
- Create: `internal/media/service_test.go`

- [ ] **Step 1: Write tests for MediaTypeFromMIME and BuildPlaceholder**

Create `internal/media/service_test.go`:

```go
package media

import "testing"

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/media/ -v -run "TestMediaTypeFromMIME|TestBuildPlaceholder|TestExtensionFromMIME|TestDetectMIME"`
Expected: Compilation failure — package/functions don't exist yet.

- [ ] **Step 3: Implement service.go**

Create `internal/media/service.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/media/ -v -run "TestMediaTypeFromMIME|TestBuildPlaceholder|TestExtensionFromMIME|TestDetectMIME"`
Expected: All PASS.

- [ ] **Step 5: Write test for SaveInbound**

Add to `internal/media/service_test.go`:

```go
import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
```

- [ ] **Step 6: Run all service tests**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/media/ -v`
Expected: All PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/media/service.go internal/media/service_test.go
git commit -m "feat(media): add media service with MIME detection, placeholder building, and file saving"
```

---

### Task 3: Text extraction — plain text, PDF, DOCX

**Files:**
- Create: `internal/media/extract.go`
- Create: `internal/media/extract_test.go`

- [ ] **Step 1: Write tests for ExtractText**

Create `internal/media/extract_test.go`:

```go
package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractText_PlainTextFile(t *testing.T) {
	s := &Service{}
	path := filepath.Join(t.TempDir(), "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	text, err := s.ExtractText(context.Background(), path)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if text != "hello world" {
		t.Errorf("got %q, want %q", text, "hello world")
	}
}

func TestExtractText_PlainTextTruncation(t *testing.T) {
	s := &Service{}
	path := filepath.Join(t.TempDir(), "big.md")
	content := strings.Repeat("a", 60000)
	os.WriteFile(path, []byte(content), 0o644)

	text, err := s.ExtractText(context.Background(), path)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if len(text) != maxExtractChars {
		t.Errorf("len = %d, want %d", len(text), maxExtractChars)
	}
}

func TestExtractText_UnsupportedExtension(t *testing.T) {
	s := &Service{}
	path := filepath.Join(t.TempDir(), "data.zip")
	os.WriteFile(path, []byte("PK\x03\x04"), 0o644)

	text, err := s.ExtractText(context.Background(), path)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if text != "" {
		t.Errorf("expected empty text for unsupported extension, got %q", text)
	}
}

func TestExtractText_GoFile(t *testing.T) {
	s := &Service{}
	path := filepath.Join(t.TempDir(), "main.go")
	os.WriteFile(path, []byte("package main\n\nfunc main() {}"), 0o644)

	text, err := s.ExtractText(context.Background(), path)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "package main") {
		t.Errorf("expected Go source content, got %q", text)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/media/ -v -run TestExtractText`
Expected: Compilation failure.

- [ ] **Step 3: Implement extract.go**

Create `internal/media/extract.go`:

```go
package media

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/nguyenthenguyen/docx"
)

const maxExtractChars = 50000

// plainTextExts lists file extensions that are treated as plain text.
var plainTextExts = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".json": true,
	".xml": true, ".yaml": true, ".yml": true, ".html": true, ".htm": true,
	".log": true, ".conf": true, ".ini": true,
	".sh": true, ".py": true, ".js": true, ".ts": true,
	".css": true, ".sql": true, ".go": true, ".java": true,
	".rs": true, ".rb": true, ".php": true,
}

// ExtractText extracts text content from a file based on its extension.
// Returns ("", nil) for unsupported file types.
func (s *Service) ExtractText(_ context.Context, path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case plainTextExts[ext]:
		return extractPlainText(path)
	case ext == ".pdf":
		return extractPDF(path)
	case ext == ".docx":
		return extractDOCX(path)
	default:
		return "", nil
	}
}

func extractPlainText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read plain text: %w", err)
	}
	text := string(data)
	if len(text) > maxExtractChars {
		text = text[:maxExtractChars]
	}
	return text, nil
}

func extractPDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open PDF: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteString("\n")
		if buf.Len() > maxExtractChars {
			break
		}
	}
	text := buf.String()
	if len(text) > maxExtractChars {
		text = text[:maxExtractChars]
	}
	return text, nil
}

func extractDOCX(path string) (string, error) {
	r, err := docx.ReadDocxFile(path)
	if err != nil {
		return "", fmt.Errorf("open DOCX: %w", err)
	}
	defer r.Close()

	doc := r.Editable()
	text := doc.GetContent()
	// The docx library returns XML-ish content; extract text between tags
	text = stripXMLTags(text)
	if len(text) > maxExtractChars {
		text = text[:maxExtractChars]
	}
	return text, nil
}

// stripXMLTags removes XML/HTML tags from a string, preserving text content.
func stripXMLTags(s string) string {
	var buf strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/media/ -v -run TestExtractText`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/media/extract.go internal/media/extract_test.go
git commit -m "feat(media): add text extraction for plain text, PDF, and DOCX files"
```

---

### Task 4: Update platform interfaces

**Files:**
- Modify: `internal/platform/interfaces.go`

- [ ] **Step 1: Add MediaPath to OutboundMessage**

In `internal/platform/interfaces.go`, add `MediaPath` field to `OutboundMessage`:

```go
// OutboundMessage carries a reply to send back on a platform.
type OutboundMessage struct {
	SessionKey string
	Content    string
	ReplyTo    InboundMessage
	MediaPath  string // optional local file path to upload and send
}
```

- [ ] **Step 2: Verify project compiles**

Run: `cd /Users/tengyongzhi/work/robobee && go build ./...`
Expected: Clean build. Existing code doesn't reference `MediaPath` so nothing breaks.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/interfaces.go
git commit -m "feat(platform): add MediaPath field to OutboundMessage"
```

---

## Chunk 2: Feishu Post Parser

### Task 5: Feishu post (rich text) parser

**Files:**
- Create: `internal/platform/feishu/post.go`
- Create: `internal/platform/feishu/post_test.go`

- [ ] **Step 1: Write tests for ParsePostContent — direct format**

Create `internal/platform/feishu/post_test.go`:

```go
package feishu

import (
	"encoding/json"
	"testing"
)

func TestParsePostContent_DirectFormat(t *testing.T) {
	content := `{
		"title": "Test Title",
		"content": [[
			{"tag": "text", "text": "Hello "},
			{"tag": "text", "text": "world", "style": ["bold"]},
			{"tag": "a", "text": "link", "href": "https://example.com"}
		]]
	}`
	result, err := ParsePostContent(content)
	if err != nil {
		t.Fatalf("ParsePostContent: %v", err)
	}
	if result.TextContent == "" {
		t.Fatal("expected non-empty text")
	}
	if len(result.ImageKeys) != 0 {
		t.Errorf("expected 0 image keys, got %d", len(result.ImageKeys))
	}
	// Should contain title, text, bold, and link
	want := "Test Title\nHello **world**[link](https://example.com)"
	if result.TextContent != want {
		t.Errorf("TextContent = %q, want %q", result.TextContent, want)
	}
}

func TestParsePostContent_LocaleFormat(t *testing.T) {
	content := `{
		"zh_cn": {
			"title": "中文标题",
			"content": [[{"tag": "text", "text": "你好"}]]
		}
	}`
	result, err := ParsePostContent(content)
	if err != nil {
		t.Fatalf("ParsePostContent: %v", err)
	}
	if result.TextContent != "中文标题\n你好" {
		t.Errorf("TextContent = %q", result.TextContent)
	}
}

func TestParsePostContent_DoubleWrapped(t *testing.T) {
	content := `{
		"post": {
			"en_us": {
				"title": "Title",
				"content": [[{"tag": "text", "text": "hello"}]]
			}
		}
	}`
	result, err := ParsePostContent(content)
	if err != nil {
		t.Fatalf("ParsePostContent: %v", err)
	}
	if result.TextContent != "Title\nhello" {
		t.Errorf("TextContent = %q", result.TextContent)
	}
}

func TestParsePostContent_WithMedia(t *testing.T) {
	content := `{
		"title": "",
		"content": [[
			{"tag": "text", "text": "see image: "},
			{"tag": "img", "image_key": "img_v3_abc"},
			{"tag": "media", "file_key": "file_v3_xyz", "file_name": "report.pdf"}
		]]
	}`
	result, err := ParsePostContent(content)
	if err != nil {
		t.Fatalf("ParsePostContent: %v", err)
	}
	if len(result.ImageKeys) != 1 || result.ImageKeys[0] != "img_v3_abc" {
		t.Errorf("ImageKeys = %v, want [img_v3_abc]", result.ImageKeys)
	}
	if len(result.MediaKeys) != 1 || result.MediaKeys[0].FileKey != "file_v3_xyz" {
		t.Errorf("MediaKeys = %v", result.MediaKeys)
	}
}

func TestParsePostContent_AllElementTypes(t *testing.T) {
	content := `{
		"title": "",
		"content": [[
			{"tag": "text", "text": "normal "},
			{"tag": "text", "text": "italic", "style": ["italic"]},
			{"tag": "text", "text": "code", "style": ["code"]},
			{"tag": "text", "text": "strike", "style": ["strikethrough"]},
			{"tag": "at", "user_name": "Alice"},
			{"tag": "code_block", "text": "fmt.Println()", "language": "go"},
			{"tag": "code", "text": "inline"},
			{"tag": "emotion", "emoji_type": "SMILE"},
			{"tag": "br"},
			{"tag": "hr"}
		]]
	}`
	result, err := ParsePostContent(content)
	if err != nil {
		t.Fatalf("ParsePostContent: %v", err)
	}
	_ = result // If it parses without error, element handling is exercised
	t.Logf("TextContent:\n%s", result.TextContent)
}

func TestParsePostContent_EmptyContent(t *testing.T) {
	_, err := ParsePostContent("")
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/platform/feishu/ -v -run TestParsePostContent`
Expected: Compilation failure — `ParsePostContent` doesn't exist.

- [ ] **Step 3: Implement post.go**

Create `internal/platform/feishu/post.go`:

```go
package feishu

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PostParseResult contains the parsed output of a Feishu post (rich text) message.
type PostParseResult struct {
	TextContent string
	ImageKeys   []string
	MediaKeys   []MediaKeyInfo
}

// MediaKeyInfo holds a file key and optional file name from a post media element.
type MediaKeyInfo struct {
	FileKey  string
	FileName string
}

// postBody represents the title + content structure found inside post payloads.
type postBody struct {
	Title   string          `json:"title"`
	Content json.RawMessage `json:"content"`
}

// ParsePostContent parses a Feishu post message content string into structured data.
// Tries three formats: direct, locale-wrapped, double-wrapped (post > locale).
func ParsePostContent(content string) (*PostParseResult, error) {
	if content == "" {
		return nil, fmt.Errorf("empty post content")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("parse post JSON: %w", err)
	}

	// Try format 1: direct {"title": "...", "content": [[...]]}
	if _, hasContent := raw["content"]; hasContent {
		var body postBody
		if err := json.Unmarshal([]byte(content), &body); err == nil {
			return parsePostBody(body)
		}
	}

	// Try format 2: locale-wrapped {"zh_cn": {"title": "...", "content": [[...]]}}
	body, found := findLocaleBody(raw)
	if found {
		return parsePostBody(body)
	}

	// Try format 3: double-wrapped {"post": {"zh_cn": {...}}}
	if postRaw, ok := raw["post"]; ok {
		var postMap map[string]json.RawMessage
		if err := json.Unmarshal(postRaw, &postMap); err == nil {
			body, found := findLocaleBody(postMap)
			if found {
				return parsePostBody(body)
			}
		}
	}

	return nil, fmt.Errorf("unrecognized post content format")
}

// findLocaleBody tries to find a postBody in a map of locale keys.
func findLocaleBody(m map[string]json.RawMessage) (postBody, bool) {
	for _, v := range m {
		var body postBody
		if err := json.Unmarshal(v, &body); err == nil && body.Content != nil {
			return body, true
		}
	}
	return postBody{}, false
}

// parsePostBody renders a postBody into a PostParseResult.
func parsePostBody(body postBody) (*PostParseResult, error) {
	var paragraphs []json.RawMessage
	if err := json.Unmarshal(body.Content, &paragraphs); err != nil {
		return nil, fmt.Errorf("parse content paragraphs: %w", err)
	}

	result := &PostParseResult{}
	var textParts []string

	if body.Title != "" {
		textParts = append(textParts, body.Title)
	}

	for _, paraRaw := range paragraphs {
		var elements []map[string]any
		if err := json.Unmarshal(paraRaw, &elements); err != nil {
			continue
		}
		var paraText strings.Builder
		for _, elem := range elements {
			tag, _ := elem["tag"].(string)
			switch tag {
			case "text":
				text, _ := elem["text"].(string)
				text = applyStyles(text, elem)
				paraText.WriteString(text)

			case "a":
				text, _ := elem["text"].(string)
				href, _ := elem["href"].(string)
				paraText.WriteString(fmt.Sprintf("[%s](%s)", text, href))

			case "at":
				name, _ := elem["user_name"].(string)
				if name == "" {
					name, _ = elem["user_id"].(string)
				}
				paraText.WriteString("@" + name)

			case "img":
				key, _ := elem["image_key"].(string)
				if key != "" {
					result.ImageKeys = append(result.ImageKeys, key)
				}

			case "media":
				fileKey, _ := elem["file_key"].(string)
				fileName, _ := elem["file_name"].(string)
				if fileKey != "" {
					result.MediaKeys = append(result.MediaKeys, MediaKeyInfo{
						FileKey:  fileKey,
						FileName: fileName,
					})
				}

			case "code_block", "pre":
				text, _ := elem["text"].(string)
				if text == "" {
					// some versions use "content" instead of "text"
					text, _ = elem["content"].(string)
				}
				lang, _ := elem["language"].(string)
				paraText.WriteString(fmt.Sprintf("\n```%s\n%s\n```\n", lang, text))

			case "code":
				text, _ := elem["text"].(string)
				paraText.WriteString("`" + text + "`")

			case "emotion":
				emoji, _ := elem["emoji_type"].(string)
				if emoji == "" {
					emoji, _ = elem["emoji"].(string)
				}
				paraText.WriteString(emoji)

			case "br":
				paraText.WriteString("\n")

			case "hr":
				paraText.WriteString("\n---\n")
			}
		}
		if paraText.Len() > 0 {
			textParts = append(textParts, paraText.String())
		}
	}

	result.TextContent = strings.Join(textParts, "\n")
	return result, nil
}

// applyStyles wraps text with markdown formatting based on the style array.
func applyStyles(text string, elem map[string]any) string {
	styles, ok := elem["style"].([]any)
	if !ok {
		return text
	}
	for _, s := range styles {
		style, _ := s.(string)
		switch style {
		case "bold":
			text = "**" + text + "**"
		case "italic":
			text = "*" + text + "*"
		case "code":
			text = "`" + text + "`"
		case "strikethrough":
			text = "~~" + text + "~~"
		}
	}
	return text
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/platform/feishu/ -v -run TestParsePostContent`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/feishu/post.go internal/platform/feishu/post_test.go
git commit -m "feat(feishu): add post (rich text) parser with element rendering and media key extraction"
```

---

## Chunk 3: DingTalk Token Refactor

### Task 6: Extract DingTalk token management to token.go

**Files:**
- Create: `internal/platform/dingtalk/token.go`
- Modify: `internal/platform/dingtalk/handler.go`

- [ ] **Step 1: Create token.go with moved getAccessToken and new getOAPIToken**

Create `internal/platform/dingtalk/token.go`. Move `getAccessToken`, `tokenCache`, and `tokenMu` from `handler.go` into this file, and add `getOAPIToken`:

```go
package dingtalk

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
	"bytes"
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
```

- [ ] **Step 2: Remove old token code from handler.go**

Remove the following from `internal/platform/dingtalk/handler.go`:
- The `tokenCache` var block (lines 114-120)
- The `getAccessToken` function (lines 122-155)
- Remove `"bytes"` from the import block (it's only used by `getAccessToken` and `buildEmojiPayload`; check if `buildEmojiPayload` still needs it — yes it does, keep it)

Actually `bytes` is also used by `buildEmojiPayload`, so keep it in imports. Just remove:
- Lines 114-120: `tokenCache` var block
- Lines 122-155: `getAccessToken` function

- [ ] **Step 3: Verify project compiles and existing tests pass**

Run: `cd /Users/tengyongzhi/work/robobee && go build ./... && go test ./internal/platform/dingtalk/ -v`
Expected: Clean build, all tests pass. `getAccessToken` is still callable from the same package.

- [ ] **Step 4: Commit**

```bash
git add internal/platform/dingtalk/token.go internal/platform/dingtalk/handler.go
git commit -m "refactor(dingtalk): extract token management to token.go, add getOAPIToken for media upload"
```

---

## Chunk 4: Feishu Inbound Media

### Task 7: Feishu receiver — handle all message types with media download

**Files:**
- Modify: `internal/platform/feishu/handler.go`

This is a larger change. The receiver currently filters for `msg_type == "text"` only. We need to handle all message types and download media.

- [ ] **Step 1: Add media service dependency to FeishuPlatform**

Update `NewPlatform` in `handler.go` to accept and pass a `*media.Service`:

```go
import (
	"github.com/robobee/core/internal/media"
)

// FeishuPlatform — add mediaSvc field
type FeishuPlatform struct {
	receiver         *FeishuReceiver
	sender           *FeishuSender
	pendingReactions sync.Map
}

func NewPlatform(cfg config.FeishuConfig, mediaSvc *media.Service) platform.Platform {
	larkClient := lark.NewClient(cfg.AppID, cfg.AppSecret)
	p := &FeishuPlatform{}
	p.receiver = &FeishuReceiver{larkClient: larkClient, cfg: cfg, pendingReactions: &p.pendingReactions, mediaSvc: mediaSvc}
	p.sender = &FeishuSender{larkClient: larkClient, pendingReactions: &p.pendingReactions}
	return p
}
```

Add `mediaSvc` field to `FeishuReceiver`:

```go
type FeishuReceiver struct {
	larkClient       *lark.Client
	cfg              config.FeishuConfig
	pendingReactions *sync.Map
	mediaSvc         *media.Service
}
```

- [ ] **Step 2: Update app.go to pass media service**

In `cmd/server/app.go`, update `buildPlatforms` to create `media.Service` and pass it to Feishu only (DingTalk signature is updated in Task 8):

```go
import "github.com/robobee/core/internal/media"

func buildPlatforms(fc config.FeishuConfig, dc config.DingTalkConfig) []platform.Platform {
	mediaSvc := media.NewService()
	var result []platform.Platform
	if fc.Enabled {
		result = append(result, feishu.NewPlatform(fc, mediaSvc))
	}
	if dc.Enabled {
		result = append(result, dingtalk.NewPlatform(dc)) // unchanged until Task 8
	}
	return result
}
```

- [ ] **Step 3: Replace text-only filter with multi-type handler**

Replace the body of the `OnP2MessageReceiveV1` callback in `FeishuReceiver.Start`. The new logic:

1. Extract message metadata (sender, chatId, etc.) — keep existing code
2. Instead of filtering `msg_type == "text"`, switch on message type
3. For text: keep existing behavior
4. For media types: parse keys, download via SDK, save, build placeholder
5. For post: parse with `ParsePostContent`, download embedded media
6. For unrecognized types: log and skip

The key helper functions to add to `handler.go`:

```go
import (
	"io"
	"regexp"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// validFeishuKey matches safe image_key/file_key values.
var validFeishuKey = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)

// parseMediaKeys extracts image_key, file_key, and file_name from message content JSON.
func parseMediaKeys(contentJSON string, msgType string) (imageKey, fileKey, fileName string) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(contentJSON), &parsed); err != nil {
		return
	}
	switch msgType {
	case "image":
		imageKey, _ = parsed["image_key"].(string)
	case "file":
		fileKey, _ = parsed["file_key"].(string)
		fileName, _ = parsed["file_name"].(string)
	case "audio":
		fileKey, _ = parsed["file_key"].(string)
	case "video", "media":
		imageKey, _ = parsed["image_key"].(string)
		fileKey, _ = parsed["file_key"].(string)
	case "sticker":
		fileKey, _ = parsed["file_key"].(string)
	}
	return
}

// resourceType returns "image" for image messages, "file" for everything else.
func resourceType(msgType string) string {
	if msgType == "image" {
		return "image"
	}
	return "file"
}

// downloadMessageResource downloads a media resource from Feishu using the Lark SDK.
func (r *FeishuReceiver) downloadMessageResource(ctx context.Context, messageID, fileKey, resType string) ([]byte, error) {
	if !validFeishuKey.MatchString(fileKey) {
		return nil, fmt.Errorf("invalid file key: %s", fileKey)
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := r.larkClient.Im.MessageResource.Get(ctx,
		larkim.NewGetMessageResourceReqBuilder().
			MessageId(messageID).
			FileKey(fileKey).
			Type(resType).
			Build())
	if err != nil {
		return nil, fmt.Errorf("download resource: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("download resource failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	defer resp.File.Close()
	return io.ReadAll(resp.File)
}

// resolveMediaContent downloads media, saves to disk, and returns the content placeholder string.
func (r *FeishuReceiver) resolveMediaContent(ctx context.Context, messageID, msgType, contentJSON string) string {
	imageKey, fileKey, fileName := parseMediaKeys(contentJSON, msgType)

	key := fileKey
	if key == "" {
		key = imageKey
	}
	if key == "" {
		return r.mediaSvc.BuildPlaceholder(mediaTypeForMsgType(msgType), "", "")
	}

	data, err := r.downloadMessageResource(ctx, messageID, key, resourceType(msgType))
	if err != nil {
		slog.Error("download media failed", "component", "feishu", "msgType", msgType, "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaTypeForMsgType(msgType), "", fileName)
	}

	mime := r.mediaSvc.DetectMIME(data, fileName)
	ext := r.mediaSvc.ExtensionFromMIME(mime)
	if fileName != "" {
		// Prefer the original extension from the file name
		if origExt := filepath.Ext(fileName); origExt != "" {
			ext = origExt
		}
	}

	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		slog.Error("save media failed", "component", "feishu", "error", err)
		return r.mediaSvc.BuildPlaceholder(mediaTypeForMsgType(msgType), "", fileName)
	}

	mediaType := mediaTypeForMsgType(msgType)
	placeholder := r.mediaSvc.BuildPlaceholder(mediaType, path, fileName)

	// For files, try text extraction
	if mediaType == "document" {
		extracted, err := r.mediaSvc.ExtractText(ctx, path)
		if err != nil {
			slog.Warn("text extraction failed", "component", "feishu", "path", path, "error", err)
		}
		if extracted != "" {
			return placeholder + "\n" + extracted
		}
	}

	return placeholder
}

// mediaTypeForMsgType maps Feishu msg_type to media type string.
func mediaTypeForMsgType(msgType string) string {
	switch msgType {
	case "image":
		return "image"
	case "audio":
		return "audio"
	case "video", "media":
		return "video"
	case "sticker":
		return "sticker"
	default:
		return "document"
	}
}

// resolvePostContent parses a post message, downloads embedded media, and returns content.
func (r *FeishuReceiver) resolvePostContent(ctx context.Context, messageID, contentJSON string) string {
	result, err := ParsePostContent(contentJSON)
	if err != nil {
		slog.Error("parse post failed", "component", "feishu", "error", err)
		return "[富文本消息]"
	}

	content := result.TextContent

	// Download embedded images
	for _, imageKey := range result.ImageKeys {
		if !validFeishuKey.MatchString(imageKey) {
			continue
		}
		data, err := r.downloadMessageResource(ctx, messageID, imageKey, "image")
		if err != nil {
			slog.Error("download post image failed", "component", "feishu", "imageKey", imageKey, "error", err)
			content += "\n" + r.mediaSvc.BuildPlaceholder("image", "", "")
			continue
		}
		mime := r.mediaSvc.DetectMIME(data, "")
		ext := r.mediaSvc.ExtensionFromMIME(mime)
		path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
		if err != nil {
			slog.Error("save post image failed", "component", "feishu", "error", err)
			content += "\n" + r.mediaSvc.BuildPlaceholder("image", "", "")
			continue
		}
		content += "\n" + r.mediaSvc.BuildPlaceholder("image", path, "")
	}

	// Download embedded media files
	for _, mk := range result.MediaKeys {
		if !validFeishuKey.MatchString(mk.FileKey) {
			continue
		}
		data, err := r.downloadMessageResource(ctx, messageID, mk.FileKey, "file")
		if err != nil {
			slog.Error("download post media failed", "component", "feishu", "fileKey", mk.FileKey, "error", err)
			content += "\n" + r.mediaSvc.BuildPlaceholder("document", "", mk.FileName)
			continue
		}
		mime := r.mediaSvc.DetectMIME(data, mk.FileName)
		ext := r.mediaSvc.ExtensionFromMIME(mime)
		if mk.FileName != "" {
			if origExt := filepath.Ext(mk.FileName); origExt != "" {
				ext = origExt
			}
		}
		path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
		if err != nil {
			slog.Error("save post media failed", "component", "feishu", "error", err)
			content += "\n" + r.mediaSvc.BuildPlaceholder("document", "", mk.FileName)
			continue
		}
		placeholder := r.mediaSvc.BuildPlaceholder("document", path, mk.FileName)
		extracted, _ := r.mediaSvc.ExtractText(ctx, path)
		if extracted != "" {
			placeholder += "\n" + extracted
		}
		content += "\n" + placeholder
	}

	return content
}
```

Then update the `OnP2MessageReceiveV1` callback to use these functions. Replace the text-only filter block (lines 64-73) with:

```go
msgType := utils.DerefStr(msg.MessageType)
contentJSON := utils.DerefStr(msg.Content)
messageID := utils.DerefStr(msg.MessageId)

var textContent string
switch msgType {
case "text":
	var content map[string]string
	if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
		return nil
	}
	textContent = content["text"]
case "image", "file", "audio", "video", "media", "sticker":
	textContent = r.resolveMediaContent(ctx, messageID, msgType, contentJSON)
case "post":
	textContent = r.resolvePostContent(ctx, messageID, contentJSON)
default:
	slog.Warn("skipping unsupported message type", "component", "feishu", "msgType", msgType)
	return nil
}

if textContent == "" {
	return nil
}
```

Also add `"path/filepath"` and `"fmt"` and `"io"` and `"regexp"` to the imports.

- [ ] **Step 4: Verify project compiles**

Run: `cd /Users/tengyongzhi/work/robobee && go build ./...`
Expected: Clean build.

- [ ] **Step 5: Run all existing tests**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./... 2>&1 | tail -30`
Expected: All pass. The feishu handler test setup may need updating if `NewPlatform` signature changed.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/feishu/handler.go cmd/server/app.go
git commit -m "feat(feishu): handle all message types with media download and text extraction"
```

---

## Chunk 5: DingTalk Inbound Media

### Task 8: Update DingTalk NewPlatform to accept media service

**Files:**
- Modify: `internal/platform/dingtalk/handler.go`

- [ ] **Step 1: Add media service dependency**

Update `NewPlatform` and `DingTalkReceiver` to accept `*media.Service`:

```go
import "github.com/robobee/core/internal/media"

type DingTalkReceiver struct {
	cfg           config.DingTalkConfig
	pendingEmojis *sync.Map
	mediaSvc      *media.Service
}

func NewPlatform(cfg config.DingTalkConfig, mediaSvc *media.Service) platform.Platform {
	p := &DingTalkPlatform{}
	p.receiver = &DingTalkReceiver{cfg: cfg, pendingEmojis: &p.pendingEmojis, mediaSvc: mediaSvc}
	p.sender = &DingTalkSender{cfg: cfg, pendingEmojis: &p.pendingEmojis}
	return p
}
```

- [ ] **Step 2: Update app.go to pass mediaSvc to DingTalk**

In `cmd/server/app.go`, update `buildPlatforms` to pass `mediaSvc` to DingTalk:

```go
if dc.Enabled {
	result = append(result, dingtalk.NewPlatform(dc, mediaSvc))
}
```

- [ ] **Step 3: Verify project compiles**

Run: `cd /Users/tengyongzhi/work/robobee && go build ./...`
Expected: Clean build.

- [ ] **Step 4: Commit**

```bash
git add internal/platform/dingtalk/handler.go cmd/server/app.go
git commit -m "refactor(dingtalk): accept media service in NewPlatform"
```

---

### Task 9: DingTalk receiver — handle all message types

**Files:**
- Modify: `internal/platform/dingtalk/handler.go`

- [ ] **Step 1: Add media download helpers to DingTalk handler**

Add these functions to `handler.go`:

```go
import (
	"io"
	"path/filepath"
)

// exchangeDownloadCode exchanges a downloadCode for a download URL via DingTalk API.
func exchangeDownloadCode(ctx context.Context, cfg config.DingTalkConfig, downloadCode string) (string, error) {
	token, err := getAccessToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return "", err
	}

	body, _ := json.Marshal(map[string]string{
		"downloadCode": downloadCode,
		"robotCode":    cfg.ClientID,
	})

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.dingtalk.com/v1.0/robot/messageFiles/download",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchange download code: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode download URL: %w", err)
	}
	if result.DownloadURL == "" {
		return "", fmt.Errorf("empty download URL")
	}
	return result.DownloadURL, nil
}

// httpDownload downloads a file from a URL and returns its bytes and content type.
func httpDownload(ctx context.Context, url string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}
```

- [ ] **Step 2: Update the receiver callback to handle all message types**

The DingTalk stream SDK's `BotCallbackDataModel` has a `Msgtype` field and content in a dynamic structure. Update the callback:

```go
// In the RegisterChatBotCallbackRouter callback, replace the current text-only logic.

// The callback receives data *chatbot.BotCallbackDataModel
// data.Msgtype tells us the type
// For text: data.Text.Content (already handled)
// For other types: we need to parse data as raw JSON to extract content fields

msgtype := "text" // default
// The BotCallbackDataModel may expose Msgtype or we need raw JSON parsing.
// Since the SDK struct primarily supports text, we need to handle raw data for media.

// Parse the raw callback for msgtype and content fields
rawBytes, _ := json.Marshal(data)
var rawData map[string]any
json.Unmarshal(rawBytes, &rawData)

if mt, ok := rawData["msgtype"].(string); ok && mt != "" {
	msgtype = mt
}

var textContent string
switch msgtype {
case "text":
	textContent = strings.TrimSpace(data.Text.Content)

case "picture":
	textContent = r.handleDingTalkPicture(ctx, rawData)

case "richText":
	textContent = r.handleDingTalkRichText(ctx, rawData)

case "file":
	textContent = r.handleDingTalkFile(ctx, rawData)

case "audio":
	textContent = r.handleDingTalkAudio(rawData)

case "video":
	textContent = r.mediaSvc.BuildPlaceholder("video", "", "")

default:
	slog.Warn("skipping unsupported message type", "component", "dingtalk", "msgtype", msgtype)
	return []byte(""), nil
}

if textContent == "" {
	return []byte(""), nil
}
```

Add the handler methods:

```go
func (r *DingTalkReceiver) handleDingTalkPicture(ctx context.Context, raw map[string]any) string {
	content, _ := raw["content"].(map[string]any)
	if content == nil {
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}
	downloadCode, _ := content["downloadCode"].(string)
	if downloadCode == "" {
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}

	dlURL, err := exchangeDownloadCode(ctx, r.cfg, downloadCode)
	if err != nil {
		slog.Error("exchange download code failed", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}

	data, ct, err := httpDownload(ctx, dlURL)
	if err != nil {
		slog.Error("download image failed", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}

	ext := r.mediaSvc.ExtensionFromMIME(ct)
	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		slog.Error("save image failed", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("image", "", "")
	}

	return r.mediaSvc.BuildPlaceholder("image", path, "")
}

func (r *DingTalkReceiver) handleDingTalkRichText(ctx context.Context, raw map[string]any) string {
	content, _ := raw["content"].(map[string]any)
	if content == nil {
		return ""
	}
	richTextArr, _ := content["richText"].([]any)

	var textParts []string
	for _, item := range richTextArr {
		itemMap, _ := item.(map[string]any)
		if itemMap == nil {
			continue
		}
		if text, ok := itemMap["text"].(string); ok && text != "" {
			textParts = append(textParts, text)
		}
		if picURL, ok := itemMap["pictureUrl"].(string); ok && picURL != "" {
			data, ct, err := httpDownload(ctx, picURL)
			if err != nil {
				slog.Error("download richtext image", "component", "dingtalk", "error", err)
				textParts = append(textParts, r.mediaSvc.BuildPlaceholder("image", "", ""))
				continue
			}
			ext := r.mediaSvc.ExtensionFromMIME(ct)
			path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
			if err != nil {
				slog.Error("save richtext image", "component", "dingtalk", "error", err)
				textParts = append(textParts, r.mediaSvc.BuildPlaceholder("image", "", ""))
				continue
			}
			textParts = append(textParts, r.mediaSvc.BuildPlaceholder("image", path, ""))
		}
	}
	return strings.Join(textParts, "\n")
}

func (r *DingTalkReceiver) handleDingTalkFile(ctx context.Context, raw map[string]any) string {
	content, _ := raw["content"].(map[string]any)
	if content == nil {
		return r.mediaSvc.BuildPlaceholder("document", "", "")
	}
	downloadCode, _ := content["downloadCode"].(string)
	fileName, _ := content["fileName"].(string)

	if downloadCode == "" {
		return r.mediaSvc.BuildPlaceholder("document", "", fileName)
	}

	dlURL, err := exchangeDownloadCode(ctx, r.cfg, downloadCode)
	if err != nil {
		slog.Error("exchange file download code", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("document", "", fileName)
	}

	data, _, err := httpDownload(ctx, dlURL)
	if err != nil {
		slog.Error("download file", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("document", "", fileName)
	}

	ext := ".bin"
	if fileName != "" {
		if origExt := filepath.Ext(fileName); origExt != "" {
			ext = origExt
		}
	}

	path, err := r.mediaSvc.SaveInbound(ctx, data, ext)
	if err != nil {
		slog.Error("save file", "component", "dingtalk", "error", err)
		return r.mediaSvc.BuildPlaceholder("document", "", fileName)
	}

	placeholder := r.mediaSvc.BuildPlaceholder("document", path, fileName)
	extracted, _ := r.mediaSvc.ExtractText(ctx, path)
	if extracted != "" {
		return placeholder + "\n" + extracted
	}
	return placeholder
}

func (r *DingTalkReceiver) handleDingTalkAudio(raw map[string]any) string {
	content, _ := raw["content"].(map[string]any)
	if content != nil {
		if recognition, ok := content["recognition"].(string); ok && recognition != "" {
			return recognition
		}
	}
	return r.mediaSvc.BuildPlaceholder("audio", "", "")
}
```

- [ ] **Step 3: Verify project compiles**

Run: `cd /Users/tengyongzhi/work/robobee && go build ./...`
Expected: Clean build.

- [ ] **Step 4: Run all tests**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./... 2>&1 | tail -30`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/dingtalk/handler.go
git commit -m "feat(dingtalk): handle all message types with media download and text extraction"
```

---

## Chunk 6: Outbound Media — MCP Tool + Platform Senders

### Task 10: Update send_message MCP tool

**Files:**
- Modify: `internal/mcp/tools.go`

- [ ] **Step 1: Update tool schema — add media_path, make content optional**

In the `toolSchemas()` function, update the `send_message` entry:

```go
{
	Name:        toolnames.SendMessage,
	Description: "Send a message to the user on the originating platform. Use message_id from the task metadata to identify the reply target. Supports sending media files (images, documents, audio, video) by providing a local file path.",
	InputSchema: map[string]any{
		"type":     "object",
		"required": []string{"message_id"},
		"properties": map[string]any{
			"message_id": map[string]string{"type": "string", "description": "ID of the originating platform message (resolves platform and reply context)"},
			"content":    map[string]string{"type": "string", "description": "Text content to send (required unless media_path is provided)"},
			"media_path": map[string]string{"type": "string", "description": "Local file path to upload and send as media (image, file, audio, or video)"},
		},
	},
},
```

- [ ] **Step 2: Update toolSendMessage handler**

```go
func (s *MCPServer) toolSendMessage(args json.RawMessage) (any, error) {
	var params struct {
		MessageID string `json:"message_id"`
		Content   string `json:"content"`
		MediaPath string `json:"media_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.MessageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}
	if params.Content == "" && params.MediaPath == "" {
		return nil, fmt.Errorf("at least one of 'content' or 'media_path' must be provided")
	}

	stored, err := s.messageStore.GetByID(context.Background(), params.MessageID)
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}

	sender, ok := s.senders[stored.Platform]
	if !ok {
		return nil, fmt.Errorf("no sender registered for platform %q", stored.Platform)
	}

	// Send text first if both content and media_path are provided
	if params.Content != "" {
		outbound := platform.OutboundMessage{
			ReplyTo: platform.InboundMessage{
				Platform:   stored.Platform,
				SessionKey: stored.SessionKey,
				Raw:        stored.Raw,
			},
			Content: params.Content,
		}
		if err := sender.Send(context.Background(), outbound); err != nil {
			return nil, fmt.Errorf("send text message: %w", err)
		}
	}

	// Send media if media_path is provided
	if params.MediaPath != "" {
		outbound := platform.OutboundMessage{
			ReplyTo: platform.InboundMessage{
				Platform:   stored.Platform,
				SessionKey: stored.SessionKey,
				Raw:        stored.Raw,
			},
			MediaPath: params.MediaPath,
		}
		if err := sender.Send(context.Background(), outbound); err != nil {
			return nil, fmt.Errorf("send media message: %w", err)
		}
	}

	return map[string]string{"status": "sent"}, nil
}
```

- [ ] **Step 3: Verify project compiles**

Run: `cd /Users/tengyongzhi/work/robobee && go build ./...`

- [ ] **Step 4: Update existing send_message tests**

The existing `TestCallTool_SendMessage_MissingContent` test expects an error when content is empty. Update it to also require media_path to be empty:

In `tools_test.go`, find the test and verify it still works — it sends `{"message_id": "msg-x"}` with no content and no media_path, so it should still fail with the new validation message.

Run: `cd /Users/tengyongzhi/work/robobee && go test ./internal/mcp/ -v -run TestCallTool_SendMessage`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/tools.go
git commit -m "feat(mcp): add media_path parameter to send_message tool"
```

---

### Task 11: Feishu sender — media upload and send

**Files:**
- Modify: `internal/platform/feishu/handler.go`

- [ ] **Step 1: Add file type detection and upload helpers**

Add to `handler.go`:

```go
import (
	"os"
	"regexp"
	"strings"
)

var sanitizeFileNameRe = regexp.MustCompile(`[\x00-\x1f\x7f\r\n"\\]`)

// sanitizeFileName removes control characters for safe multipart upload.
func sanitizeFileName(name string) string {
	return sanitizeFileNameRe.ReplaceAllString(name, "_")
}

// fileCategory determines the media category from file extension.
func fileCategory(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".ico", ".tiff":
		return "image"
	case ".opus", ".ogg", ".mp3", ".wav", ".amr", ".aac", ".flac", ".m4a":
		return "audio"
	case ".mp4", ".mov", ".avi":
		return "video"
	default:
		return "file"
	}
}

// feishuFileType maps file extension to Feishu's file_type parameter.
func feishuFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".opus", ".ogg":
		return "opus"
	case ".mp4", ".mov", ".avi":
		return "mp4"
	case ".pdf":
		return "pdf"
	case ".doc", ".docx":
		return "doc"
	case ".xls", ".xlsx":
		return "xls"
	case ".ppt", ".pptx":
		return "ppt"
	default:
		return "stream"
	}
}

// feishuMediaMsgType maps file_type to the msg_type for sending.
func feishuMediaMsgType(fileType string) string {
	switch fileType {
	case "opus":
		return "audio"
	case "mp4":
		return "media"
	default:
		return "file"
	}
}
```

- [ ] **Step 2: Update FeishuSender.Send to handle MediaPath**

Update the `Send` method. When `msg.MediaPath` is non-empty and `msg.Content` is empty, handle media upload and send:

```go
func (s *FeishuSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var event larkim.P2MessageReceiveV1
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &event); err != nil {
		slog.Error("failed to unmarshal raw", "component", "feishu", "error", err)
		return nil
	}
	imMsg := event.Event.Message
	chatID := *imMsg.ChatId
	chatType := *imMsg.ChatType
	messageID := *imMsg.MessageId

	// Recall "typing" reaction before sending reply
	if reactionID, ok := s.pendingReactions.LoadAndDelete(messageID); ok {
		recallCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		resp, err := s.larkClient.Im.MessageReaction.Delete(recallCtx,
			larkim.NewDeleteMessageReactionReqBuilder().
				MessageId(messageID).
				ReactionId(reactionID.(string)).
				Build())
		cancel()
		if err != nil || !resp.Success() {
			slog.Warn("recall reaction error", "component", "feishu", "error", err, "resp", resp)
		}
	}

	if msg.MediaPath != "" {
		return s.sendMedia(ctx, msg.MediaPath, chatID, chatType, messageID)
	}

	// Text message (existing logic)
	content, _ := json.Marshal(map[string]string{"text": msg.Content})
	return s.sendMessage(ctx, chatID, chatType, messageID, larkim.MsgTypeText, string(content))
}

func (s *FeishuSender) sendMessage(ctx context.Context, chatID, chatType, messageID, msgType, content string) error {
	if chatType == "p2p" {
		resp, err := s.larkClient.Im.Message.Create(ctx,
			larkim.NewCreateMessageReqBuilder().
				ReceiveIdType(larkim.ReceiveIdTypeChatId).
				Body(larkim.NewCreateMessageReqBodyBuilder().
					MsgType(msgType).
					ReceiveId(chatID).
					Content(content).
					Build()).
				Build())
		if err != nil || !resp.Success() {
			slog.Error("send message error", "component", "feishu", "error", err, "resp", resp)
		}
	} else {
		resp, err := s.larkClient.Im.Message.Reply(ctx,
			larkim.NewReplyMessageReqBuilder().
				MessageId(messageID).
				Body(larkim.NewReplyMessageReqBodyBuilder().
					MsgType(msgType).
					Content(content).
					Build()).
				Build())
		if err != nil || !resp.Success() {
			// Fallback: if reply fails (message withdrawn), try direct send
			code := 0
			if resp != nil {
				code = resp.Code
			}
			if code == 230011 || code == 231003 {
				slog.Warn("reply failed, falling back to direct send", "component", "feishu", "code", code)
				resp2, err2 := s.larkClient.Im.Message.Create(ctx,
					larkim.NewCreateMessageReqBuilder().
						ReceiveIdType(larkim.ReceiveIdTypeChatId).
						Body(larkim.NewCreateMessageReqBodyBuilder().
							MsgType(msgType).
							ReceiveId(chatID).
							Content(content).
							Build()).
						Build())
				if err2 != nil || !resp2.Success() {
					slog.Error("fallback send error", "component", "feishu", "error", err2, "resp", resp2)
				}
			} else {
				slog.Error("reply message error", "component", "feishu", "error", err, "resp", resp)
			}
		}
	}
	return nil
}

func (s *FeishuSender) sendMedia(ctx context.Context, mediaPath, chatID, chatType, messageID string) error {
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("read media file: %w", err)
	}
	if len(data) > 30*1024*1024 {
		return fmt.Errorf("file too large: %d bytes (max 30MB)", len(data))
	}

	category := fileCategory(mediaPath)
	fileName := sanitizeFileName(filepath.Base(mediaPath))

	if category == "image" {
		return s.uploadAndSendImage(ctx, data, chatID, chatType, messageID)
	}
	return s.uploadAndSendFile(ctx, data, fileName, mediaPath, chatID, chatType, messageID)
}

func (s *FeishuSender) uploadAndSendImage(ctx context.Context, data []byte, chatID, chatType, messageID string) error {
	uploadCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := s.larkClient.Im.Image.Create(uploadCtx,
		larkim.NewCreateImageReqBuilder().
			Body(larkim.NewCreateImageReqBodyBuilder().
				ImageType(larkim.ImageTypeMessage).
				Image(bytes.NewReader(data)).
				Build()).
			Build())
	if err != nil || !resp.Success() {
		return fmt.Errorf("upload image: err=%v resp=%v", err, resp)
	}

	imageKey := *resp.Data.ImageKey
	content, _ := json.Marshal(map[string]string{"image_key": imageKey})
	return s.sendMessage(ctx, chatID, chatType, messageID, larkim.MsgTypeImage, string(content))
}

func (s *FeishuSender) uploadAndSendFile(ctx context.Context, data []byte, fileName, mediaPath, chatID, chatType, messageID string) error {
	ft := feishuFileType(mediaPath)

	uploadCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := s.larkClient.Im.File.Create(uploadCtx,
		larkim.NewCreateFileReqBuilder().
			Body(larkim.NewCreateFileReqBodyBuilder().
				FileType(ft).
				FileName(fileName).
				File(bytes.NewReader(data)).
				Build()).
			Build())
	if err != nil || !resp.Success() {
		return fmt.Errorf("upload file: err=%v resp=%v", err, resp)
	}

	fileKey := *resp.Data.FileKey
	msgType := feishuMediaMsgType(ft)
	content, _ := json.Marshal(map[string]string{"file_key": fileKey})
	return s.sendMessage(ctx, chatID, chatType, messageID, msgType, string(content))
}
```

Add `"bytes"` to the import if not already there.

- [ ] **Step 3: Verify project compiles**

Run: `cd /Users/tengyongzhi/work/robobee && go build ./...`

- [ ] **Step 4: Run all tests**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./... 2>&1 | tail -30`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/feishu/handler.go
git commit -m "feat(feishu): add media upload and send for images, files, audio, and video"
```

---

### Task 12: DingTalk sender — media upload and send

**Files:**
- Modify: `internal/platform/dingtalk/handler.go`

- [ ] **Step 1: Add upload and media send helpers**

Add to `handler.go`:

```go
import (
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
)

// dingTalkMediaType maps file category to DingTalk upload media type.
func dingTalkMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return "image"
	case ".mp4", ".mov", ".avi":
		return "video"
	case ".mp3", ".wav", ".amr", ".ogg", ".aac", ".flac", ".m4a", ".opus":
		return "voice"
	default:
		return "file"
	}
}

// uploadMediaToDingTalk uploads a file to DingTalk's OAPI media endpoint.
func uploadMediaToDingTalk(ctx context.Context, cfg config.DingTalkConfig, filePath, mediaType string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	if len(data) > 20*1024*1024 {
		return "", fmt.Errorf("file too large: %d bytes (max 20MB)", len(data))
	}

	token, err := getOAPIToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return "", fmt.Errorf("get OAPI token: %w", err)
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	ct := "application/octet-stream"
	if mediaType == "image" {
		ct = "image/jpeg"
	}

	part, err := writer.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="media"; filename="%s"`, filepath.Base(filePath))},
		"Content-Type":        {ct},
	})
	if err != nil {
		return "", err
	}
	part.Write(data)
	writer.Close()

	url := fmt.Sprintf("https://oapi.dingtalk.com/media/upload?access_token=%s&type=%s", token, mediaType)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload media: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		MediaID string `json:"media_id"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode upload response: %w", err)
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("upload error %d: %s", result.ErrCode, result.ErrMsg)
	}
	return result.MediaID, nil
}

// sendMediaViaDingTalk sends a media message via sessionWebhook.
func sendMediaViaDingTalk(ctx context.Context, cfg config.DingTalkConfig, webhook, filePath, mediaID string) error {
	mediaType := dingTalkMediaType(filePath)
	fileName := filepath.Base(filePath)
	fileType := strings.TrimPrefix(filepath.Ext(filePath), ".")

	var payload map[string]any
	switch mediaType {
	case "image":
		payload = map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]string{"title": "Image", "text": fmt.Sprintf("![image](%s)", mediaID)},
		}
	case "voice":
		payload = map[string]any{
			"msgtype": "voice",
			"voice":   map[string]string{"mediaId": mediaID, "duration": "60000"},
		}
	case "video":
		payload = map[string]any{
			"msgtype": "video",
			"video":   map[string]string{"duration": "0", "videoMediaId": mediaID, "videoType": "mp4", "picMediaId": ""},
		}
	default:
		payload = map[string]any{
			"msgtype": "file",
			"file":    map[string]string{"mediaId": mediaID, "fileName": fileName, "fileType": fileType},
		}
	}

	body, _ := json.Marshal(payload)

	token, err := getAccessToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send media failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}
```

- [ ] **Step 2: Update DingTalkSender.Send to handle MediaPath**

```go
func (s *DingTalkSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var data chatbot.BotCallbackDataModel
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &data); err != nil {
		slog.Error("failed to unmarshal raw", "component", "dingtalk", "error", err)
		return nil
	}
	if _, ok := s.pendingEmojis.LoadAndDelete(data.MsgId); ok {
		recallThinkingEmoji(ctx, s.cfg, &data)
	}

	if msg.MediaPath != "" {
		mediaID, err := uploadMediaToDingTalk(ctx, s.cfg, msg.MediaPath, dingTalkMediaType(msg.MediaPath))
		if err != nil {
			slog.Error("upload media failed", "component", "dingtalk", "error", err)
			return fmt.Errorf("upload media: %w", err)
		}
		if err := sendMediaViaDingTalk(ctx, s.cfg, data.SessionWebhook, msg.MediaPath, mediaID); err != nil {
			slog.Error("send media failed", "component", "dingtalk", "error", err)
			return fmt.Errorf("send media: %w", err)
		}
		return nil
	}

	// Text message (existing logic)
	replier := chatbot.NewChatbotReplier()
	slog.Info("sending reply", "component", "dingtalk", "sessionKey", msg.ReplyTo.SessionKey, "webhookLen", len(data.SessionWebhook), "contentLen", len(msg.Content))
	if err := replier.SimpleReplyMarkdown(ctx, data.SessionWebhook, []byte(markdownTitle), []byte(msg.Content)); err != nil {
		slog.Error("reply send error", "component", "dingtalk", "error", err)
		return nil
	}
	slog.Info("reply sent ok", "component", "dingtalk")
	return nil
}
```

- [ ] **Step 3: Verify project compiles**

Run: `cd /Users/tengyongzhi/work/robobee && go build ./...`

- [ ] **Step 4: Run all tests**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./... 2>&1 | tail -30`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/dingtalk/handler.go
git commit -m "feat(dingtalk): add media upload and send for images, files, audio, and video"
```

---

## Chunk 7: Final Integration + Verification

### Task 13: Run full test suite and fix any issues

**Files:** None new — this is a verification task.

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/tengyongzhi/work/robobee && go test ./... -v 2>&1 | tail -50`
Expected: All tests pass.

- [ ] **Step 2: Run go vet**

Run: `cd /Users/tengyongzhi/work/robobee && go vet ./...`
Expected: No issues.

- [ ] **Step 3: Build the binary**

Run: `cd /Users/tengyongzhi/work/robobee && go build -o /dev/null ./cmd/server/`
Expected: Clean build.

- [ ] **Step 4: Fix any issues found in steps 1-3**

If any test failures, vet warnings, or build errors: fix them and re-run.

- [ ] **Step 5: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: resolve integration issues from multimedia messaging implementation"
```
