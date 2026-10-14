package telegram

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform"
)

func TestSendTarget_ReplyUsesOriginMessage(t *testing.T) {
	raw := `{"update_id":1,"message":{"message_id":77,"chat":{"id":-100123}}}`

	chatID, replyToID, err := sendTarget(platform.OutboundMessage{ReplyTo: platform.InboundMessage{Raw: raw}})
	require.NoError(t, err)
	assert.Equal(t, int64(-100123), chatID)
	assert.Equal(t, 77, replyToID)
}

func TestSendTarget_ProactiveUsesSessionChatWithoutQuoting(t *testing.T) {
	stale := `{"update_id":1,"message":{"message_id":77,"chat":{"id":-100123}}}`

	chatID, replyToID, err := sendTarget(platform.OutboundMessage{
		SessionKey: "telegram:-100123:42",
		ReplyTo:    platform.InboundMessage{Raw: stale},
		Proactive:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(-100123), chatID)
	assert.Zero(t, replyToID)
}

func TestSendTarget_ProactiveRejectsInvalidSessionKey(t *testing.T) {
	_, _, err := sendTarget(platform.OutboundMessage{SessionKey: "telegram:abc:42", Proactive: true})
	require.Error(t, err)
}
