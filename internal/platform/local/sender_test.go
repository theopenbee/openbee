package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
)

func TestLocalSender_Send_Broadcasts(t *testing.T) {
	hub := local.NewSSEHub()
	sender := local.NewLocalSender(hub, t.TempDir())

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

func TestLocalSender_Send_StagesMediaIntoSessionDir(t *testing.T) {
	hub := local.NewSSEHub()
	root := t.TempDir()
	sender := local.NewLocalSender(hub, root)

	src := filepath.Join(t.TempDir(), "chart.png")
	require.NoError(t, os.WriteFile(src, []byte("png-bytes"), 0o644))

	ch, unsub := hub.Subscribe("local:user-1")
	defer unsub()

	msg := platform.OutboundMessage{
		ID:        "out-2",
		ReplyTo:   platform.InboundMessage{SessionKey: "local:user-1"},
		MediaPath: src,
	}
	require.NoError(t, sender.Send(context.Background(), msg))

	select {
	case data := <-ch:
		var payload struct {
			ID         string   `json:"id"`
			MediaPaths []string `json:"media_paths"`
		}
		require.NoError(t, json.Unmarshal([]byte(data), &payload))
		assert.Equal(t, "out-2", payload.ID)
		assert.Equal(t, []string{"out-2_chart.png"}, payload.MediaPaths)
	default:
		t.Fatal("expected SSE broadcast but channel was empty")
	}

	staged, err := os.ReadFile(filepath.Join(root, "user-1", "out-2_chart.png"))
	require.NoError(t, err)
	assert.Equal(t, "png-bytes", string(staged))
}

func TestLocalSender_Send_MissingMediaFails(t *testing.T) {
	hub := local.NewSSEHub()
	sender := local.NewLocalSender(hub, t.TempDir())

	ch, unsub := hub.Subscribe("local:user-1")
	defer unsub()

	msg := platform.OutboundMessage{
		ID:        "out-3",
		ReplyTo:   platform.InboundMessage{SessionKey: "local:user-1"},
		MediaPath: filepath.Join(t.TempDir(), "missing.png"),
	}
	require.Error(t, sender.Send(context.Background(), msg))

	select {
	case data := <-ch:
		t.Fatalf("expected no broadcast, got %q", data)
	default:
	}
}
