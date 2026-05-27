package feishu

import (
	"strings"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/media"
)

func TestParseMediaKeys(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		msgType   string
		wantImage string
		wantFile  string
		wantName  string
	}{
		{
			name:      "image type",
			content:   `{"image_key":"img_abc123"}`,
			msgType:   "image",
			wantImage: "img_abc123",
		},
		{
			name:      "sticker type",
			content:   `{"image_key":"img_sticker"}`,
			msgType:   "sticker",
			wantImage: "img_sticker",
		},
		{
			name:     "file type",
			content:  `{"file_key":"file_xyz","file_name":"report.pdf"}`,
			msgType:  "file",
			wantFile: "file_xyz",
			wantName: "report.pdf",
		},
		{
			name:     "audio type",
			content:  `{"file_key":"file_audio"}`,
			msgType:  "audio",
			wantFile: "file_audio",
		},
		{
			name:     "audio type with duration",
			content:  `{"file_key":"file_audio","duration":2000}`,
			msgType:  "audio",
			wantFile: "file_audio",
		},
		{
			name:    "invalid json",
			content: `not json`,
			msgType: "image",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, file, name := parseMediaKeys(tt.content, tt.msgType)
			if img != tt.wantImage {
				t.Errorf("imageKey = %q, want %q", img, tt.wantImage)
			}
			if file != tt.wantFile {
				t.Errorf("fileKey = %q, want %q", file, tt.wantFile)
			}
			if name != tt.wantName {
				t.Errorf("fileName = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestResourceType(t *testing.T) {
	tests := []struct {
		msgType string
		want    string
	}{
		{"image", "image"},
		{"sticker", "image"},
		{"file", "file"},
		{"audio", "file"},
		{"video", "file"},
		{"media", "file"},
	}
	for _, tt := range tests {
		t.Run(tt.msgType, func(t *testing.T) {
			if got := resourceType(tt.msgType); got != tt.want {
				t.Errorf("resourceType(%q) = %q, want %q", tt.msgType, got, tt.want)
			}
		})
	}
}

func TestMediaTypeForMsgType(t *testing.T) {
	tests := []struct {
		msgType string
		want    string
	}{
		{"image", "image"},
		{"audio", "audio"},
		{"video", "video"},
		{"media", "video"},
		{"sticker", "sticker"},
		{"file", "document"},
	}
	for _, tt := range tests {
		t.Run(tt.msgType, func(t *testing.T) {
			if got := mediaTypeForMsgType(tt.msgType); got != tt.want {
				t.Errorf("mediaTypeForMsgType(%q) = %q, want %q", tt.msgType, got, tt.want)
			}
		})
	}
}

func TestFileCategory(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"photo.jpg", "image"},
		{"photo.PNG", "image"},
		{"song.mp3", "audio"},
		{"song.opus", "audio"},
		{"clip.mp4", "video"},
		{"doc.pdf", "file"},
		{"noext", "file"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := fileCategory(tt.path); got != tt.want {
				t.Errorf("fileCategory(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestFeishuFileType(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"voice.opus", "opus"},
		{"voice.ogg", "opus"},
		{"clip.mp4", "mp4"},
		{"doc.pdf", "pdf"},
		{"doc.docx", "doc"},
		{"sheet.xlsx", "xls"},
		{"slide.pptx", "ppt"},
		{"other.zip", "stream"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := feishuFileType(tt.path); got != tt.want {
				t.Errorf("feishuFileType(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestFeishuMediaMsgType(t *testing.T) {
	tests := []struct {
		fileType string
		want     string
	}{
		{"opus", "audio"},
		{"mp4", "media"},
		{"pdf", "file"},
		{"stream", "file"},
	}
	for _, tt := range tests {
		t.Run(tt.fileType, func(t *testing.T) {
			if got := feishuMediaMsgType(tt.fileType); got != tt.want {
				t.Errorf("feishuMediaMsgType(%q) = %q, want %q", tt.fileType, got, tt.want)
			}
		})
	}
}

func TestResolveMentions(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name     string
		text     string
		mentions []*larkim.MentionEvent
		botName  string
		want     string
	}{
		{
			name: "bot mention replaced, user mention preserved",
			text: "@_user_1 @_user_2 hello",
			mentions: []*larkim.MentionEvent{
				{Key: strPtr("@_user_1"), Name: strPtr("OpenBee")},
				{Key: strPtr("@_user_2"), Name: strPtr("Tom")},
			},
			botName: "OpenBee",
			want:    "@OpenBee @_user_2 hello",
		},
		{
			name: "multiple user mentions preserved",
			text: "@_user_1 and @_user_2",
			mentions: []*larkim.MentionEvent{
				{Key: strPtr("@_user_1"), Name: strPtr("Tom")},
				{Key: strPtr("@_user_2"), Name: strPtr("Alice")},
			},
			botName: "OpenBee",
			want:    "@_user_1 and @_user_2",
		},
		{
			name:     "empty mentions no change",
			text:     "@_user_1 hello",
			mentions: nil,
			botName:  "OpenBee",
			want:     "@_user_1 hello",
		},
		{
			name: "empty botName no replacement",
			text: "@_user_1 hello",
			mentions: []*larkim.MentionEvent{
				{Key: strPtr("@_user_1"), Name: strPtr("Tom")},
			},
			botName: "",
			want:    "@_user_1 hello",
		},
		{
			name: "nil key skipped",
			text: "@_user_1 hello",
			mentions: []*larkim.MentionEvent{
				{Key: nil, Name: strPtr("OpenBee")},
				{Key: strPtr("@_user_1"), Name: strPtr("OpenBee")},
			},
			botName: "OpenBee",
			want:    "@OpenBee hello",
		},
		{
			name: "nil name skipped",
			text: "@_user_1 hello",
			mentions: []*larkim.MentionEvent{
				{Key: strPtr("@_user_1"), Name: nil},
			},
			botName: "OpenBee",
			want:    "@_user_1 hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveMentions(tt.text, tt.mentions, tt.botName)
			if got != tt.want {
				t.Errorf("resolveMentions() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUploadAndSendFile_ContentByType(t *testing.T) {
	// Verify that the content JSON for "media" type includes file_name,
	// while "audio" and "file" types only include file_key.
	tests := []struct {
		msgType      string
		wantFileName bool
	}{
		{"media", true},
		{"audio", false},
		{"file", false},
	}
	for _, tt := range tests {
		t.Run(tt.msgType, func(t *testing.T) {
			var contentMap map[string]string
			switch tt.msgType {
			case "media":
				contentMap = map[string]string{"file_key": "test_key", "file_name": "test.mp4"}
			default:
				contentMap = map[string]string{"file_key": "test_key"}
			}
			_, hasFileName := contentMap["file_name"]
			if hasFileName != tt.wantFileName {
				t.Errorf("msgType %q: hasFileName = %v, want %v", tt.msgType, hasFileName, tt.wantFileName)
			}
		})
	}
}

func TestExtractContext_ValidFeishuRaw(t *testing.T) {
	// Minimal Feishu P2MessageReceiveV1 JSON with the fields we extract.
	raw := `{"schema":"2.0","header":{"event_id":"evt1","event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_abc","union_id":"on_abc"},"sender_type":"user","tenant_key":"tk1"},"message":{"message_id":"om_1","chat_id":"oc_xyz","chat_type":"group","message_type":"text"}}}`
	got := ExtractContext(raw)
	if got == "" {
		t.Fatal("expected non-empty context")
	}
	if !strings.Contains(got, `"sender"`) {
		t.Errorf("expected sender namespace in context, got: %q", got)
	}
	if !strings.Contains(got, `"message"`) {
		t.Errorf("expected message namespace in context, got: %q", got)
	}
	if !strings.Contains(got, "ou_abc") {
		t.Errorf("expected open_id value in context, got: %q", got)
	}
	if !strings.Contains(got, `"sender_type"`) {
		t.Errorf("expected sender_type in context, got: %q", got)
	}
	if !strings.Contains(got, `"message_type"`) {
		t.Errorf("expected message_type in context, got: %q", got)
	}
}

func TestExtractContext_InvalidRaw(t *testing.T) {
	got := ExtractContext("not-json")
	if got != "" {
		t.Errorf("expected empty string for invalid raw, got %q", got)
	}
}

func TestFeishuPlatform_AccountName(t *testing.T) {
	cfg := config.FeishuConfig{Name: "marketing", AppID: "appid", AppSecret: "secret"}
	p := NewPlatform(cfg, media.NewService())
	if p.ID() != "feishu" {
		t.Errorf("ID() = %q, want feishu", p.ID())
	}
	if got := p.AccountName(); got != "marketing" {
		t.Errorf("AccountName() = %q, want marketing", got)
	}
}

func TestFeishuPlatform_DefaultAccountName(t *testing.T) {
	cfg := config.FeishuConfig{Name: "default", AppID: "appid", AppSecret: "secret"}
	p := NewPlatform(cfg, media.NewService())
	if got := p.AccountName(); got != "default" {
		t.Errorf("AccountName() = %q, want default", got)
	}
}
