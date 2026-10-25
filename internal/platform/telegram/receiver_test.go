package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform"
)

const topicUpdate = `{"update_id":500,"message":{"message_id":42,"message_thread_id":5,"is_topic_message":true,` +
	`"from":{"id":789,"is_bot":false,"first_name":"u"},"chat":{"id":-100456,"type":"supergroup","is_forum":true},` +
	`"date":1700000000,"text":"remind me in 5 minutes"}}`

func TestFetchUpdates_KeepsTopicFields(t *testing.T) {
	bot, api := newFakeBot(t, func(c apiCall) string {
		return `{"ok":true,"result":[` + topicUpdate + `]}`
	})

	updates, err := fetchUpdates(bot, 500, 30)
	require.NoError(t, err)

	calls := api.recorded()
	require.Len(t, calls, 1)
	assert.Equal(t, "getUpdates", calls[0].method)
	assert.Equal(t, map[string]string{"offset": "500", "timeout": "30"}, calls[0].form)

	require.Len(t, updates, 1)
	assert.Equal(t, 500, updates[0].UpdateID)
	assert.Equal(t, 5, updates[0].threadID)
	assert.Equal(t, "remind me in 5 minutes", updates[0].Message.Text)
	assert.JSONEq(t, topicUpdate, string(updates[0].raw))
}

func TestBuildInboundMessage_TopicSession(t *testing.T) {
	bot, _ := newFakeBot(t, func(apiCall) string {
		return `{"ok":true,"result":[` + topicUpdate + `]}`
	})
	updates, err := fetchUpdates(bot, 0, 0)
	require.NoError(t, err)
	require.Len(t, updates, 1)

	r := &TelegramReceiver{bot: bot}
	msg := r.buildInboundMessage(context.Background(), bot, updates[0])
	require.NotNil(t, msg)
	assert.Equal(t, "telegram:-100456:5:789", msg.SessionKey)
	assert.Equal(t, "789", msg.SenderID)
	assert.Equal(t, "500:42", msg.PlatformMessageID)

	got, err := sendTarget(platform.OutboundMessage{ReplyTo: *msg})
	require.NoError(t, err)
	assert.Equal(t, target{chatID: -100456, threadID: 5, replyToID: 42}, got)
}

func TestStart_TypingTargetsTopic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := false
	bot, api := newFakeBot(t, func(c apiCall) string {
		if c.method != "getUpdates" {
			return ""
		}
		if served {
			cancel()
			return `{"ok":true,"result":[]}`
		}
		served = true
		return `{"ok":true,"result":[` + topicUpdate + `]}`
	})
	r := &TelegramReceiver{bot: bot}

	dispatched := make(chan platform.InboundMessage, 1)
	require.NoError(t, r.Start(ctx, func(m platform.InboundMessage) { dispatched <- m }))
	assert.Equal(t, "telegram:-100456:5:789", (<-dispatched).SessionKey)

	require.Eventually(t, func() bool {
		for _, c := range api.recorded() {
			if c.method == "sendChatAction" {
				return c.form["chat_id"] == "-100456" && c.form["message_thread_id"] == "5" && c.form["action"] == "typing"
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
}

func TestStart_MigratesChatOnUpgrade(t *testing.T) {
	cases := []struct {
		name   string
		update string
	}{
		{"old group event", `{"update_id":1,"message":{"message_id":3,"from":{"id":7,"is_bot":false,"first_name":"u"},"chat":{"id":-123,"type":"group"},"date":0,"migrate_to_chat_id":-100999}}`},
		{"new supergroup event", `{"update_id":1,"message":{"message_id":1,"from":{"id":7,"is_bot":false,"first_name":"u"},"chat":{"id":-100999,"type":"supergroup"},"date":0,"migrate_from_chat_id":-123}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			served := false
			bot, _ := newFakeBot(t, func(c apiCall) string {
				if served {
					cancel()
					return `{"ok":true,"result":[]}`
				}
				served = true
				return `{"ok":true,"result":[` + tc.update + `]}`
			})
			migrator := &fakeMigrator{}
			r := &TelegramReceiver{bot: bot, migrator: migrator}

			dispatched := 0
			require.NoError(t, r.Start(ctx, func(platform.InboundMessage) { dispatched++ }))
			assert.Zero(t, dispatched)
			assert.Equal(t, []migrationCall{{"telegram:-123:", "telegram:-100999:"}}, migrator.calls)
		})
	}
}
