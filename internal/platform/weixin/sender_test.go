package weixin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform"
)

func TestSendTarget_ReplyUsesOriginMessage(t *testing.T) {
	raw := `{"from_user_id":"u1@im.wechat","to_user_id":"bot","session_id":"s1","context_token":"ctx-origin"}`

	got, err := sendTarget(platform.OutboundMessage{ReplyTo: platform.InboundMessage{Raw: raw}})
	require.NoError(t, err)
	assert.Equal(t, weixinRaw{FromUserID: "u1@im.wechat", ToUserID: "bot", SessionID: "s1", ContextToken: "ctx-origin"}, got)
}

func TestSendTarget_ProactiveUsesSessionUserAndLatestContext(t *testing.T) {
	latest := `{"from_user_id":"u1@im.wechat","to_user_id":"bot","session_id":"s9","context_token":"ctx-latest"}`

	got, err := sendTarget(platform.OutboundMessage{
		SessionKey: "weixin:u1@im.wechat:u1@im.wechat",
		ReplyTo:    platform.InboundMessage{Raw: latest},
		Proactive:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, weixinRaw{FromUserID: "u1@im.wechat", ToUserID: "bot", SessionID: "s9", ContextToken: "ctx-latest"}, got)
}

func TestSendTarget_ProactiveWithoutContext(t *testing.T) {
	got, err := sendTarget(platform.OutboundMessage{SessionKey: "weixin:u1:u1", Proactive: true})
	require.NoError(t, err)
	assert.Equal(t, weixinRaw{FromUserID: "u1"}, got)
}
