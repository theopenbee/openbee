package feishu

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	require.NotEmpty(t, result.TextContent)
	assert.Empty(t, result.ImageKeys)
	want := "Test Title\nHello **world**[link](https://example.com)"
	assert.Equal(t, want, result.TextContent)
}

func TestParsePostContent_LocaleFormat(t *testing.T) {
	content := `{
		"zh_cn": {
			"title": "Chinese Title",
			"content": [[{"tag": "text", "text": "hello"}]]
		}
	}`
	result, err := ParsePostContent(content)
	require.NoError(t, err)
	assert.Equal(t, "Chinese Title\nhello", result.TextContent)
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
	require.NoError(t, err)
	assert.Equal(t, "Title\nhello", result.TextContent)
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
	require.NoError(t, err)
	assert.Equal(t, []string{"img_v3_abc"}, result.ImageKeys)
	require.Len(t, result.MediaKeys, 1) // guard: MediaKeys[0] accessed below
	assert.Equal(t, "file_v3_xyz", result.MediaKeys[0].FileKey)
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
	require.NoError(t, err)
	_ = result
	t.Logf("TextContent:\n%s", result.TextContent)
}

func TestParsePostContent_EmptyContent(t *testing.T) {
	_, err := ParsePostContent("")
	assert.Error(t, err)
}

func TestBuildPostContent(t *testing.T) {
	t.Run("basic markdown", func(t *testing.T) {
		got := BuildPostContent("## Hello\n- item")
		require.NotEmpty(t, got)
		// Must be valid JSON
		var raw map[string]any
		require.NoError(t, json.Unmarshal([]byte(got), &raw))
		// Must have zh_cn key
		_, ok := raw["zh_cn"]
		assert.True(t, ok, "expected zh_cn key")
	})

	t.Run("md tag and text preserved", func(t *testing.T) {
		markdown := "**bold** and `code`"
		got := BuildPostContent(markdown)
		// Verify the md tag and content are present
		var outer map[string]map[string]any
		require.NoError(t, json.Unmarshal([]byte(got), &outer))
		lang := outer["zh_cn"]
		paragraphs, _ := lang["content"].([]any)
		require.NotEmpty(t, paragraphs)
		elems, _ := paragraphs[0].([]any)
		require.NotEmpty(t, elems)
		elem, _ := elems[0].(map[string]any)
		assert.Equal(t, "md", elem["tag"])
		assert.Equal(t, markdown, elem["text"])
	})

	t.Run("empty string", func(t *testing.T) {
		got := BuildPostContent("")
		require.NotEmpty(t, got)
		var raw map[string]any
		require.NoError(t, json.Unmarshal([]byte(got), &raw))
	})
}
