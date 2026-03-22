package weixin

import (
	"encoding/json"
	"testing"
)

func TestWeixinMessageMarshalRoundtrip(t *testing.T) {
	msg := WeixinMessage{
		MessageID:    12345,
		FromUserID:   "user@im.wechat",
		ToUserID:     "bot@im.wechat",
		CreateTimeMs: 1711100000000,
		MessageType:  MessageTypeUser,
		MessageState: MessageStateFinish,
		ContextToken: "ctx-token-abc",
		ItemList: []MessageItem{
			{
				Type:     MessageItemTypeText,
				TextItem: &TextItem{Text: "hello"},
			},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got WeixinMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.FromUserID != msg.FromUserID {
		t.Errorf("FromUserID = %q, want %q", got.FromUserID, msg.FromUserID)
	}
	if got.ContextToken != msg.ContextToken {
		t.Errorf("ContextToken = %q, want %q", got.ContextToken, msg.ContextToken)
	}
	if len(got.ItemList) != 1 || got.ItemList[0].TextItem == nil {
		t.Fatal("expected 1 text item")
	}
	if got.ItemList[0].TextItem.Text != "hello" {
		t.Errorf("text = %q, want %q", got.ItemList[0].TextItem.Text, "hello")
	}
}

func TestMessageItemTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		val  int
		want int
	}{
		{"text", MessageItemTypeText, 1},
		{"image", MessageItemTypeImage, 2},
		{"voice", MessageItemTypeVoice, 3},
		{"file", MessageItemTypeFile, 4},
		{"video", MessageItemTypeVideo, 5},
	}
	for _, tt := range tests {
		if tt.val != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.val, tt.want)
		}
	}
}
