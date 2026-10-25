package telegram

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform"
)

const topicUpdate = `{"update_id":500,"message":{"message_id":42,"message_thread_id":5,"is_topic_message":true,` +
	`"from":{"id":789,"is_bot":false,"first_name":"u"},"chat":{"id":-100456,"type":"supergroup","is_forum":true},` +
	`"date":1700000000,"text":"remind me in 5 minutes"}}`

func serveUpdatesOnce(update string) func(apiCall) string {
	served := false
	return func(c apiCall) string {
		if c.method != "getUpdates" {
			return ""
		}
		if served {
			time.Sleep(10 * time.Millisecond)
			return `{"ok":true,"result":[]}`
		}
		served = true
		return `{"ok":true,"result":[` + update + `]}`
	}
}

func TestBuildInboundMessage_TopicSession(t *testing.T) {
	var update models.Update
	require.NoError(t, json.Unmarshal([]byte(topicUpdate), &update))

	r := &TelegramReceiver{}
	msg := r.buildInboundMessage(context.Background(), &update)
	require.NotNil(t, msg)
	assert.Equal(t, "telegram:-100456:5:789", msg.SessionKey)
	assert.Equal(t, "789", msg.SenderID)
	assert.Equal(t, "500:42", msg.PlatformMessageID)
	assert.Equal(t, int64(1700000000000), msg.MessageTime)

	got, err := sendTarget(platform.OutboundMessage{ReplyTo: *msg})
	require.NoError(t, err)
	assert.Equal(t, target{chatID: -100456, threadID: 5, replyToID: 42}, got)
}

func TestBuildInboundMessage_ReplyThreadOutsideTopicIgnored(t *testing.T) {
	raw := `{"update_id":1,"message":{"message_id":42,"message_thread_id":40,` +
		`"from":{"id":789,"is_bot":false,"first_name":"u"},"chat":{"id":-100456,"type":"supergroup"},"date":0,"text":"hi"}}`
	var update models.Update
	require.NoError(t, json.Unmarshal([]byte(raw), &update))

	msg := (&TelegramReceiver{}).buildInboundMessage(context.Background(), &update)
	require.NotNil(t, msg)
	assert.Equal(t, "telegram:-100456:789", msg.SessionKey)
}

func TestStart_DispatchesTopicMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &TelegramReceiver{}
	b, api := newFakeBot(t, serveUpdatesOnce(topicUpdate), r.handleUpdate)
	r.bot = b

	dispatched := make(chan platform.InboundMessage, 1)
	done := make(chan error, 1)
	go func() { done <- r.Start(ctx, func(m platform.InboundMessage) { dispatched <- m }) }()

	got := <-dispatched
	assert.Equal(t, "telegram:-100456:5:789", got.SessionKey)
	assert.Equal(t, "remind me in 5 minutes", got.Content)

	require.Eventually(t, func() bool {
		for _, c := range api.recorded() {
			if c.method == "sendChatAction" {
				return c.form["chat_id"] == "-100456" && c.form["message_thread_id"] == "5" && c.form["action"] == "typing"
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-done)
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
			migrator := &fakeMigrator{onCall: cancel}
			r := &TelegramReceiver{migrator: migrator}
			b, _ := newFakeBot(t, serveUpdatesOnce(tc.update), r.handleUpdate)
			r.bot = b

			dispatched := 0
			require.NoError(t, r.Start(ctx, func(platform.InboundMessage) { dispatched++ }))
			assert.Zero(t, dispatched)
			assert.Equal(t, []migrationCall{{"telegram:-123:", "telegram:-100999:"}}, migrator.recorded())
		})
	}
}
