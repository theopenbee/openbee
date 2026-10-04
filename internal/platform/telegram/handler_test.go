package telegram

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelegramPlatformID(t *testing.T) {
	p := &TelegramPlatform{}
	assert.Equal(t, "telegram", p.ID())
}

func TestBuildSessionKey(t *testing.T) {
	tests := []struct {
		name     string
		chatID   int64
		senderID int64
		want     string
	}{
		{"private chat", 123, 123, "telegram:123:123"},
		{"group chat", -100456, 789, "telegram:-100456:789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSessionKey(tt.chatID, tt.senderID)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"<b>bold</b>", "&lt;b&gt;bold&lt;/b&gt;"},
		{"a & b", "a &amp; b"},
		{"price: 5 > 3", "price: 5 &gt; 3"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeHTML(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRaw(t *testing.T) {
	raw := `{"update_id":100,"message":{"message_id":42,"chat":{"id":-9876},"date":1700000000}}`
	chatID, msgID, err := parseRaw(raw)
	require.NoError(t, err)
	assert.EqualValues(t, -9876, chatID)
	assert.Equal(t, 42, msgID)
}

func TestParseRaw_InvalidJSON(t *testing.T) {
	_, _, err := parseRaw("not json")
	assert.Error(t, err)
}

func TestBuildPlatformMessageID(t *testing.T) {
	got := buildPlatformMessageID(100, 42)
	assert.Equal(t, "100:42", got)
}

func TestMediaTypeFromTelegram(t *testing.T) {
	tests := []struct {
		msgType string
		want    string
	}{
		{"photo", "image"},
		{"video", "video"},
		{"audio", "audio"},
		{"voice", "audio"},
		{"document", "document"},
		{"sticker", "sticker"},
		{"unknown", "document"},
	}
	for _, tt := range tests {
		t.Run(tt.msgType, func(t *testing.T) {
			got := mediaTypeFromTelegram(tt.msgType)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Ensure package compiles and interface compliance.
var _ interface{ ID() string } = (*TelegramPlatform)(nil)
