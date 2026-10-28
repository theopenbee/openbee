package pluginchannel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionKey_RoundTrip(t *testing.T) {
	assert.Equal(t, "discord:123:456", BuildSessionKey("discord", "123", "456"))

	chat, scope, err := SplitSessionKey("discord", "discord:123:t9:456")
	require.NoError(t, err)
	assert.Equal(t, "123", chat)
	assert.Equal(t, "t9:456", scope)

	_, _, err = SplitSessionKey("discord", "slack:123:456")
	assert.Error(t, err)
	_, _, err = SplitSessionKey("discord", "discord:")
	assert.Error(t, err)
	_, _, err = SplitSessionKey("discord", "discord:123")
	assert.Error(t, err)
}

func TestEnvelope_RoundTrip(t *testing.T) {
	enc := EncodeEnvelope("r1", "m1", json.RawMessage(`{"guild_id":"9"}`))
	raw, msgID, ctx := DecodeEnvelope(enc)
	assert.Equal(t, "r1", raw)
	assert.Equal(t, "m1", msgID)
	assert.JSONEq(t, `{"guild_id":"9"}`, string(ctx))

	var probe map[string]any
	require.NoError(t, json.Unmarshal([]byte(enc), &probe))
	assert.Equal(t, float64(1), probe["v"])
}

func TestEnvelope_DecodeGarbage(t *testing.T) {
	for _, s := range []string{"", "not json", `{"v":2,"raw":"x"}`, `[]`} {
		raw, msgID, ctx := DecodeEnvelope(s)
		assert.Empty(t, raw, s)
		assert.Empty(t, msgID, s)
		assert.Empty(t, ctx, s)
	}
}

func TestExtractContextFor(t *testing.T) {
	extract := ExtractContextFor("discord")
	assert.JSONEq(t, `{"discord":{"guild_id":"9"}}`, extract(EncodeEnvelope("r", "m", json.RawMessage(`{"guild_id":"9"}`))))
	assert.Equal(t, "", extract(EncodeEnvelope("r", "m", nil)))
	assert.Equal(t, "", extract("garbage"))
}

func TestReplyTo(t *testing.T) {
	assert.Nil(t, replyTo(""))
	assert.Nil(t, replyTo(EncodeEnvelope("", "", json.RawMessage(`{"a":1}`))))
	assert.Equal(t, &ReplyTo{MessageID: "m1", Raw: "r1"}, replyTo(EncodeEnvelope("r1", "m1", nil)))
	legacy := `{"update_id":7,"message":{"message_id":3,"chat":{"id":-100}}}`
	assert.Equal(t, &ReplyTo{Raw: legacy}, replyTo(legacy))
}
