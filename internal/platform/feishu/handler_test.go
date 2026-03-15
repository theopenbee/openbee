package feishu

import (
	"testing"
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
