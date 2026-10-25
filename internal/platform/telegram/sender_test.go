package telegram

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/platform"
)

func TestSendTarget_ReplyUsesOriginMessage(t *testing.T) {
	raw := `{"update_id":1,"message":{"message_id":77,"chat":{"id":-100123}}}`

	got, err := sendTarget(platform.OutboundMessage{ReplyTo: platform.InboundMessage{Raw: raw}})
	require.NoError(t, err)
	assert.Equal(t, target{chatID: -100123, replyToID: 77}, got)
}

func TestSendTarget_ReplyKeepsForumTopic(t *testing.T) {
	raw := `{"update_id":1,"message":{"message_id":77,"message_thread_id":5,"is_topic_message":true,"chat":{"id":-100123}}}`

	got, err := sendTarget(platform.OutboundMessage{ReplyTo: platform.InboundMessage{Raw: raw}})
	require.NoError(t, err)
	assert.Equal(t, target{chatID: -100123, threadID: 5, replyToID: 77}, got)
}

func TestSendTarget_ProactiveUsesSessionChatWithoutQuoting(t *testing.T) {
	stale := `{"update_id":1,"message":{"message_id":77,"chat":{"id":-100123}}}`

	got, err := sendTarget(platform.OutboundMessage{
		SessionKey: "telegram:-100123:42",
		ReplyTo:    platform.InboundMessage{Raw: stale},
		Proactive:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, target{chatID: -100123}, got)
}

func TestSendTarget_ProactiveUsesSessionTopic(t *testing.T) {
	stale := `{"update_id":1,"message":{"message_id":77,"message_thread_id":9,"is_topic_message":true,"chat":{"id":-100123}}}`

	got, err := sendTarget(platform.OutboundMessage{
		SessionKey: "telegram:-100123:5:42",
		ReplyTo:    platform.InboundMessage{Raw: stale},
		Proactive:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, target{chatID: -100123, threadID: 5}, got)
}

func TestSendTarget_ProactiveRejectsInvalidSessionKey(t *testing.T) {
	for _, key := range []string{"telegram:abc:42", "telegram:-100123:abc:42"} {
		_, err := sendTarget(platform.OutboundMessage{SessionKey: key, Proactive: true})
		require.Error(t, err, key)
	}
}

func TestSend_ProactiveTextTargetsTopic(t *testing.T) {
	b, api := newFakeBot(t, nil, nil)
	s := &TelegramSender{bot: b}

	err := s.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "telegram:-100123:5:42",
		Content:    "a < b",
		Proactive:  true,
	})
	require.NoError(t, err)

	calls := api.recorded()
	require.Len(t, calls, 1)
	assert.Equal(t, "sendMessage", calls[0].method)
	assert.Equal(t, map[string]string{
		"chat_id":           "-100123",
		"message_thread_id": "5",
		"text":              "a &lt; b",
		"parse_mode":        "HTML",
	}, calls[0].form)
}

func TestSend_ProactiveTextWithoutTopicOmitsThread(t *testing.T) {
	b, api := newFakeBot(t, nil, nil)
	s := &TelegramSender{bot: b}

	err := s.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "telegram:-100123:42",
		Content:    "hi",
		Proactive:  true,
	})
	require.NoError(t, err)

	calls := api.recorded()
	require.Len(t, calls, 1)
	assert.NotContains(t, calls[0].form, "message_thread_id")
	assert.NotContains(t, calls[0].form, "reply_parameters")
}

func TestSend_ReplyTextQuotesOriginInTopic(t *testing.T) {
	b, api := newFakeBot(t, nil, nil)
	s := &TelegramSender{bot: b}
	raw := `{"update_id":1,"message":{"message_id":77,"message_thread_id":5,"is_topic_message":true,"chat":{"id":-100123}}}`

	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hi",
		ReplyTo: platform.InboundMessage{Raw: raw},
	})
	require.NoError(t, err)

	calls := api.recorded()
	require.Len(t, calls, 1)
	assert.JSONEq(t, `{"message_id":77}`, calls[0].form["reply_parameters"])
	assert.Equal(t, "5", calls[0].form["message_thread_id"])
}

func TestSend_ProactiveMediaTargetsTopic(t *testing.T) {
	cases := []struct {
		file   string
		method string
		field  string
	}{
		{"chart.png", "sendPhoto", "photo"},
		{"clip.mp4", "sendVideo", "video"},
		{"song.mp3", "sendAudio", "audio"},
		{"note.ogg", "sendVoice", "voice"},
		{"report.pdf", "sendDocument", "document"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			b, api := newFakeBot(t, nil, nil)
			s := &TelegramSender{cfg: config.TelegramConfig{MaxMediaSize: 1 << 20}, bot: b}
			path := filepath.Join(t.TempDir(), tc.file)
			require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))

			err := s.Send(context.Background(), platform.OutboundMessage{
				SessionKey: "telegram:-100123:5:42",
				MediaPath:  path,
				Proactive:  true,
			})
			require.NoError(t, err)

			calls := api.recorded()
			require.Len(t, calls, 1)
			assert.Equal(t, tc.method, calls[0].method)
			assert.Equal(t, []string{tc.field}, calls[0].files)
			assert.Equal(t, "-100123", calls[0].form["chat_id"])
			assert.Equal(t, "5", calls[0].form["message_thread_id"])
		})
	}
}

func TestSend_RetriesInMigratedSupergroup(t *testing.T) {
	b, api := newFakeBot(t, func(c apiCall) string {
		if c.form["chat_id"] == "-123" {
			return `{"ok":false,"error_code":400,"description":"Bad Request: group chat was upgraded to a supergroup chat","parameters":{"migrate_to_chat_id":-100999}}`
		}
		return ""
	}, nil)
	migrator := &fakeMigrator{}
	s := &TelegramSender{bot: b, migrator: migrator}
	raw := `{"update_id":1,"message":{"message_id":77,"chat":{"id":-123}}}`

	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hi",
		ReplyTo: platform.InboundMessage{Raw: raw},
	})
	require.NoError(t, err)

	calls := api.recorded()
	require.Len(t, calls, 2)
	assert.Equal(t, "-100999", calls[1].form["chat_id"])
	assert.NotContains(t, calls[1].form, "reply_parameters")
	assert.Equal(t, []migrationCall{{"telegram:-123:", "telegram:-100999:"}}, migrator.recorded())
}

func TestSend_ReturnsAPIError(t *testing.T) {
	b, _ := newFakeBot(t, func(apiCall) string {
		return `{"ok":false,"error_code":400,"description":"Bad Request: message thread not found"}`
	}, nil)
	s := &TelegramSender{bot: b}

	err := s.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "telegram:-100123:5:42",
		Content:    "hi",
		Proactive:  true,
	})
	require.ErrorContains(t, err, "message thread not found")
}
