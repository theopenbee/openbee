package wecom

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewWsConn_Defaults(t *testing.T) {
	c := NewWsConn(WsConnConfig{BotID: "bot1", Secret: "sec1"})
	assert.Equal(t, wsDefaultURL, c.cfg.URL)
	assert.Equal(t, wsDefaultHeartbeat, c.cfg.HeartbeatInterval)
	assert.Equal(t, wsDefaultMaxReconnect, c.cfg.MaxReconnectAttempts)
	assert.Equal(t, wsDefaultReconnectBase, c.cfg.ReconnectBaseDelay)
	assert.Equal(t, wsDefaultAckTimeout, c.cfg.ReplyAckTimeout)
	assert.False(t, c.IsConnected())
}

func TestNewWsConn_CustomURL(t *testing.T) {
	c := NewWsConn(WsConnConfig{
		BotID:             "bot1",
		Secret:            "sec1",
		URL:               "wss://custom.example.com",
		HeartbeatInterval: 10 * time.Second,
	})
	assert.Equal(t, "wss://custom.example.com", c.cfg.URL)
	assert.Equal(t, 10*time.Second, c.cfg.HeartbeatInterval)
}

func TestGenerateReqID_Uniqueness(t *testing.T) {
	ids := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		id := generateReqID("test")
		assert.NotContains(t, ids, id, "generated duplicate req_id")
		ids[id] = struct{}{}
	}
}

func TestGenerateReqID_Prefix(t *testing.T) {
	id := generateReqID("aibot_subscribe")
	assert.Contains(t, id, "aibot_subscribe")
}
