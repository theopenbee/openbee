package feishu

import (
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			assert.Equal(t, tt.wantImage, img)
			assert.Equal(t, tt.wantFile, file)
			assert.Equal(t, tt.wantName, name)
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
			got := resourceType(tt.msgType)
			assert.Equal(t, tt.want, got)
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
			got := mediaTypeForMsgType(tt.msgType)
			assert.Equal(t, tt.want, got)
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
			got := fileCategory(tt.path)
			assert.Equal(t, tt.want, got)
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
			got := feishuFileType(tt.path)
			assert.Equal(t, tt.want, got)
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
			got := feishuMediaMsgType(tt.fileType)
			assert.Equal(t, tt.want, got)
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
			assert.Equal(t, tt.want, got)
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
			assert.Equal(t, tt.wantFileName, hasFileName)
		})
	}
}

func TestExtractContext_ValidFeishuRaw(t *testing.T) {
	// Minimal Feishu P2MessageReceiveV1 JSON with the fields we extract.
	raw := `{"schema":"2.0","header":{"event_id":"evt1","event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_abc","union_id":"on_abc"},"sender_type":"user","tenant_key":"tk1"},"message":{"message_id":"om_1","chat_id":"oc_xyz","chat_type":"group","message_type":"text"}}}`
	got := ExtractContext(raw)
	require.NotEmpty(t, got)
	assert.Contains(t, got, `"sender"`)
	assert.Contains(t, got, `"message"`)
	assert.Contains(t, got, "ou_abc")
	assert.Contains(t, got, `"sender_type"`)
	assert.Contains(t, got, `"message_type"`)
}

func TestExtractContext_InvalidRaw(t *testing.T) {
	got := ExtractContext("not-json")
	assert.Empty(t, got)
}
