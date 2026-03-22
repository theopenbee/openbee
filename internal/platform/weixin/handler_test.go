package weixin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWeixinPlatformID(t *testing.T) {
	p := &WeixinPlatform{}
	assert.Equal(t, "weixin", p.ID())
}

func TestBuildSessionKey(t *testing.T) {
	assert.Equal(t, "weixin:user123:user123", buildSessionKey("user123"))
}

func TestBuildPlatformMessageID(t *testing.T) {
	assert.Equal(t, "10:42", buildPlatformMessageID(10, 42))
}

func TestParseRaw(t *testing.T) {
	raw := weixinRaw{
		FromUserID:   "sender1",
		ToUserID:     "bot1",
		SessionID:    "sess1",
		ContextToken: "ctx-tok-123",
	}
	data, err := json.Marshal(raw)
	require.NoError(t, err)

	got, err := parseWeixinRaw(string(data))
	require.NoError(t, err)
	assert.Equal(t, "sender1", got.FromUserID)
	assert.Equal(t, "ctx-tok-123", got.ContextToken)
}

func TestParseRaw_InvalidJSON(t *testing.T) {
	_, err := parseWeixinRaw("not json")
	assert.Error(t, err)
}

func TestExtractTextContent(t *testing.T) {
	items := []messageItem{
		{Type: 1, TextItem: &textItem{Text: "hello"}},
		{Type: 1, TextItem: &textItem{Text: " world"}},
	}
	got := extractTextContent(items)
	assert.Equal(t, "hello world", got)
}

func TestExtractTextContent_Empty(t *testing.T) {
	got := extractTextContent(nil)
	assert.Equal(t, "", got)
}

func TestExtractTextContent_SkipsUnknownTypes(t *testing.T) {
	items := []messageItem{
		{Type: 99},
		{Type: 1, TextItem: &textItem{Text: "only text"}},
	}
	got := extractTextContent(items)
	assert.Equal(t, "only text", got)
}
