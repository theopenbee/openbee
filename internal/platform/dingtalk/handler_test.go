package dingtalk

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/stretchr/testify/assert"
)

func TestIsWebhookExpired(t *testing.T) {
	nowMS := time.Now().UnixMilli()

	data := &chatbot.BotCallbackDataModel{}

	data.SessionWebhookExpiredTime = nowMS - 1000 // expired 1s ago
	assert.True(t, isWebhookExpired(data))

	data.SessionWebhookExpiredTime = nowMS + 60000 // expires in 60s
	assert.False(t, isWebhookExpired(data))

	data.SessionWebhookExpiredTime = 0 // zero means treat as expired
	assert.True(t, isWebhookExpired(data))
}

func TestSendTextProactive_GroupChat(t *testing.T) {
	data := &chatbot.BotCallbackDataModel{
		ConversationType: "1",
		ConversationId:   "testGroupId",
		SenderStaffId:    "testStaffId",
	}
	cfg := config.DingTalkConfig{ClientID: "testBot"}

	payload := buildProactiveTextPayload(cfg, data, "Hello World")
	assert.Equal(t, "sampleMarkdown", payload["msgKey"])
	assert.Equal(t, cfg.ClientID, payload["robotCode"])
	assert.Equal(t, "testGroupId", payload["openConversationId"])
	_, hasUserIds := payload["userIds"]
	assert.False(t, hasUserIds)
}

func TestSendTextProactive_SingleChat(t *testing.T) {
	data := &chatbot.BotCallbackDataModel{
		ConversationType: "2",
		ConversationId:   "testConvId",
		SenderStaffId:    "testStaffId",
	}
	cfg := config.DingTalkConfig{ClientID: "testBot"}

	payload := buildProactiveTextPayload(cfg, data, "Hello World")
	assert.Equal(t, "sampleMarkdown", payload["msgKey"])
	assert.Equal(t, cfg.ClientID, payload["robotCode"])
	userIds, ok := payload["userIds"]
	assert.True(t, ok)
	assert.Equal(t, []string{"testStaffId"}, userIds)
	_, hasConvId := payload["openConversationId"]
	assert.False(t, hasConvId)
}

func TestBuildProactiveMediaPayload_File(t *testing.T) {
	cfg := config.DingTalkConfig{ClientID: "testBot"}
	data := &chatbot.BotCallbackDataModel{
		ConversationType: "1",
		ConversationId:   "groupId",
	}
	payload := buildProactiveMediaPayload(cfg, data, "/tmp/report.pdf", "media123", mediaInfo{})
	assert.Equal(t, "sampleFile", payload["msgKey"])
	paramStr, _ := payload["msgParam"].(string)
	var param map[string]string
	json.Unmarshal([]byte(paramStr), &param)
	assert.Equal(t, "media123", param["mediaId"])
	assert.Equal(t, "report.pdf", param["fileName"])
	assert.Equal(t, "pdf", param["fileType"])
}

func TestBuildProactiveMediaPayload_Audio(t *testing.T) {
	cfg := config.DingTalkConfig{ClientID: "testBot"}
	data := &chatbot.BotCallbackDataModel{ConversationType: "1", ConversationId: "g"}
	payload := buildProactiveMediaPayload(cfg, data, "/tmp/voice.mp3", "audio456", mediaInfo{durationMs: 60000})
	assert.Equal(t, "sampleAudio", payload["msgKey"])
	paramStr, _ := payload["msgParam"].(string)
	var param map[string]string
	json.Unmarshal([]byte(paramStr), &param)
	assert.Equal(t, "audio456", param["mediaId"])
	assert.Equal(t, "60000", param["duration"])
}

func TestBuildProactiveMediaPayload_Video(t *testing.T) {
	cfg := config.DingTalkConfig{ClientID: "testBot"}
	data := &chatbot.BotCallbackDataModel{ConversationType: "1", ConversationId: "g"}
	payload := buildProactiveMediaPayload(cfg, data, "/tmp/clip.mp4", "vid789", mediaInfo{durationSec: 30, picMediaID: "thumb001"})
	assert.Equal(t, "sampleVideo", payload["msgKey"])
	paramStr, _ := payload["msgParam"].(string)
	var param map[string]string
	json.Unmarshal([]byte(paramStr), &param)
	assert.Equal(t, "30", param["duration"])
	assert.Equal(t, "thumb001", param["picMediaId"])
}

func TestBuildProactiveMediaPayload_Image(t *testing.T) {
	cfg := config.DingTalkConfig{ClientID: "testBot"}
	data := &chatbot.BotCallbackDataModel{ConversationType: "1", ConversationId: "g"}
	payload := buildProactiveMediaPayload(cfg, data, "/tmp/photo.jpg", "img001", mediaInfo{})
	assert.Equal(t, "sampleMarkdown", payload["msgKey"])
	paramStr, _ := payload["msgParam"].(string)
	var param map[string]string
	json.Unmarshal([]byte(paramStr), &param)
	assert.Contains(t, param["text"], "img001")
}

func TestSend_RoutesCorrectlyOnExpiry(t *testing.T) {
	nowMS := time.Now().UnixMilli()

	expiredData := chatbot.BotCallbackDataModel{
		SessionWebhookExpiredTime: nowMS - 1000,
		ConversationType:          "1",
		ConversationId:            "gid",
		SenderStaffId:             "sid",
	}
	assert.True(t, isWebhookExpired(&expiredData))

	validData := chatbot.BotCallbackDataModel{
		SessionWebhookExpiredTime: nowMS + 3600000,
		SessionWebhook:            "https://example.com/webhook",
	}
	assert.False(t, isWebhookExpired(&validData))
}

func TestExtractContext_ValidDingTalkRaw(t *testing.T) {
	raw := `{
		"conversationId": "conv1",
		"atUsers": [
			{"dingtalkId": "dt001", "staffId": "s001"},
			{"dingtalkId": "dt002", "staffId": ""}
		],
		"chatbotCorpId": "botcorp1",
		"chatbotUserId": "botuser1",
		"msgId": "msg1",
		"senderNick": "Alice",
		"isAdmin": true,
		"senderStaffId": "emp001",
		"senderCorpId": "corp1",
		"conversationType": "2",
		"senderId": "sender1",
		"conversationTitle": "Test Group",
		"msgtype": "text",
		"sessionWebhook": "https://should-be-excluded.example.com",
		"createAt": 1700000000000
	}`

	got := ExtractContext(raw)
	assert.NotEmpty(t, got)

	var wrapper map[string]any
	assert.NoError(t, json.Unmarshal([]byte(got), &wrapper))

	ctx, ok := wrapper["dingtalk"].(map[string]any)
	assert.True(t, ok, "expected dingtalk key with object value")

	// Whitelist fields present with original names
	assert.Equal(t, "conv1", ctx["conversationId"])
	assert.Equal(t, "botcorp1", ctx["chatbotCorpId"])
	assert.Equal(t, "botuser1", ctx["chatbotUserId"])
	assert.Equal(t, "msg1", ctx["msgId"])
	assert.Equal(t, "Alice", ctx["senderNick"])
	assert.Equal(t, "emp001", ctx["senderStaffId"])
	assert.Equal(t, "corp1", ctx["senderCorpId"])
	assert.Equal(t, "2", ctx["conversationType"])
	assert.Equal(t, "sender1", ctx["senderId"])
	assert.Equal(t, "Test Group", ctx["conversationTitle"])
	assert.Equal(t, "text", ctx["msgtype"])

	// isAdmin is bool, not string
	isAdmin, ok := ctx["isAdmin"].(bool)
	assert.True(t, ok, "isAdmin should be bool")
	assert.True(t, isAdmin)

	// atUsers is array
	atUsers, ok := ctx["atUsers"].([]any)
	assert.True(t, ok, "atUsers should be array")
	assert.Len(t, atUsers, 2)
	first := atUsers[0].(map[string]any)
	assert.Equal(t, "dt001", first["dingtalkId"])
	assert.Equal(t, "s001", first["staffId"])

	// Excluded fields must not appear
	assert.NotContains(t, ctx, "sessionWebhook")
	assert.NotContains(t, ctx, "createAt")
}

func TestExtractContext_InvalidDingTalkRaw(t *testing.T) {
	got := ExtractContext("not-json")
	assert.Empty(t, got)
}

func TestDingTalkPlatform_AccountName(t *testing.T) {
	cfg := config.DingTalkConfig{Name: "marketing", ClientID: "cid", ClientSecret: "sec"}
	p := NewPlatform(cfg, config.MediaConfig{}, nil)
	assert.Equal(t, "dingtalk", p.ID())
	assert.Equal(t, "marketing", p.AccountName())
}

func TestDingTalkPlatform_DefaultAccountName(t *testing.T) {
	cfg := config.DingTalkConfig{Name: "default", ClientID: "cid", ClientSecret: "sec"}
	p := NewPlatform(cfg, config.MediaConfig{}, nil)
	assert.Equal(t, "default", p.AccountName())
}
