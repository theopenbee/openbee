package local_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
)

func TestLocalSender_Send_Broadcasts(t *testing.T) {
	hub := local.NewSSEHub()
	sender := local.NewLocalSender(hub)

	ch, unsub := hub.Subscribe("local:sess-1")
	defer unsub()

	// OutboundMessage.SessionKey is intentionally left empty — Send reads the
	// session key from ReplyTo.SessionKey only.
	msg := platform.OutboundMessage{
		ID:      "out-1",
		ReplyTo: platform.InboundMessage{SessionKey: "local:sess-1"},
		Content: "Reply content",
	}
	require.NoError(t, sender.Send(context.Background(), msg))

	select {
	case data := <-ch:
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(data), &payload))
		assert.Equal(t, "Reply content", payload["content"])
		assert.Equal(t, "out-1", payload["id"])
	default:
		t.Fatal("expected SSE broadcast but channel was empty")
	}
}
