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

	msg := platform.OutboundMessage{
		ReplyTo: platform.InboundMessage{SessionKey: "local:sess-1"},
		Content: "Reply content",
	}
	require.NoError(t, sender.Send(context.Background(), msg))

	select {
	case data := <-ch:
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(data), &payload))
		assert.Equal(t, "Reply content", payload["content"])
	default:
		t.Fatal("expected SSE broadcast but channel was empty")
	}
}

func TestLocalSender_Send_UsesReplyToSessionKey(t *testing.T) {
	hub := local.NewSSEHub()
	sender := local.NewLocalSender(hub)

	ch, unsub := hub.Subscribe("local:correct-session")
	defer unsub()

	// OutboundMessage.SessionKey is empty — only ReplyTo.SessionKey should be used
	msg := platform.OutboundMessage{
		SessionKey: "",
		ReplyTo:    platform.InboundMessage{SessionKey: "local:correct-session"},
		Content:    "test",
	}
	require.NoError(t, sender.Send(context.Background(), msg))

	select {
	case <-ch:
		// broadcast received on the correct session — pass
	default:
		t.Error("expected SSE broadcast on correct session but channel was empty")
	}
}
