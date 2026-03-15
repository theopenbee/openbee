package feishu

import (
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
	_ = result
	t.Logf("TextContent:\n%s", result.TextContent)
}

func TestParsePostContent_EmptyContent(t *testing.T) {
	_, err := ParsePostContent("")
	if err == nil {
		t.Error("expected error for empty content")
	}
}
