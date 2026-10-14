package dingtalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/platform"
)

func TestDingTalkSender_ProactiveKeepsPendingEmojiOfLatestMessage(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	raw, err := json.Marshal(chatbot.BotCallbackDataModel{
		MsgId:                     "msg-latest",
		ConversationId:            "cid-1",
		ConversationType:          "1",
		SessionWebhook:            srv.URL,
		SessionWebhookExpiredTime: time.Now().Add(time.Hour).UnixMilli(),
	})
	require.NoError(t, err)

	pending := &sync.Map{}
	pending.Store("msg-latest", struct{}{})
	s := &DingTalkSender{cfg: config.DingTalkConfig{ClientID: "bot"}, pendingEmojis: pending}

	err = s.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "dingtalk:cid-1:staff-1",
		Content:    "daily report",
		ReplyTo:    platform.InboundMessage{Platform: PlatformID, SessionKey: "dingtalk:cid-1:staff-1", Raw: string(raw)},
		Proactive:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), hits.Load())
	_, stillPending := pending.Load("msg-latest")
	assert.True(t, stillPending)
}

func TestDingTalkSender_ProactiveRequiresConversationContext(t *testing.T) {
	s := &DingTalkSender{pendingEmojis: &sync.Map{}}

	err := s.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "dingtalk:cid-1:staff-1",
		Content:    "hi",
		ReplyTo:    platform.InboundMessage{Platform: PlatformID, SessionKey: "dingtalk:cid-1:staff-1"},
		Proactive:  true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no conversation context")
}
