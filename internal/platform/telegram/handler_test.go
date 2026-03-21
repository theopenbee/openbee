package telegram

import (
	"testing"
)

func TestTelegramPlatformID(t *testing.T) {
	p := &TelegramPlatform{}
	if p.ID() != "telegram" {
		t.Errorf("ID() = %q, want %q", p.ID(), "telegram")
	}
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
			if got != tt.want {
				t.Errorf("buildSessionKey(%d, %d) = %q, want %q",
					tt.chatID, tt.senderID, got, tt.want)
			}
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
			if got != tt.want {
				t.Errorf("escapeHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseRaw(t *testing.T) {
	raw := `{"update_id":100,"message":{"message_id":42,"chat":{"id":-9876},"date":1700000000}}`
	chatID, msgID, err := parseRaw(raw)
	if err != nil {
		t.Fatalf("parseRaw error: %v", err)
	}
	if chatID != -9876 {
		t.Errorf("chatID = %d, want -9876", chatID)
	}
	if msgID != 42 {
		t.Errorf("msgID = %d, want 42", msgID)
	}
}

func TestParseRaw_InvalidJSON(t *testing.T) {
	_, _, err := parseRaw("not json")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestBuildPlatformMessageID(t *testing.T) {
	got := buildPlatformMessageID(100, 42)
	if got != "100:42" {
		t.Errorf("buildPlatformMessageID(100, 42) = %q, want %q", got, "100:42")
	}
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
			if got != tt.want {
				t.Errorf("mediaTypeFromTelegram(%q) = %q, want %q",
					tt.msgType, got, tt.want)
			}
		})
	}
}

// Ensure package compiles and interface compliance.
var _ interface{ ID() string } = (*TelegramPlatform)(nil)
