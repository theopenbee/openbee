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

type ssePayload struct {
	ID         string   `json:"id"`
	Content    string   `json:"content"`
	MediaPaths []string `json:"media_paths"`
}

func receivePayload(t *testing.T, ch <-chan string) ssePayload {
	t.Helper()
	select {
	case data := <-ch:
		var payload ssePayload
		require.NoError(t, json.Unmarshal([]byte(data), &payload))
		return payload
	default:
		t.Fatal("expected SSE broadcast but channel was empty")
		return ssePayload{}
	}
}

func writeAgentFile(t *testing.T, name, content string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(src, []byte(content), 0o644))
	return src
}

func TestLocalSender_Send_Broadcasts(t *testing.T) {
	hub := local.NewSSEHub()
	sender := local.NewLocalSender(hub, t.TempDir(), nil)

	ch, unsub := hub.Subscribe("local:sess-1")
	defer unsub()

	msg := platform.OutboundMessage{
		ID:      "out-1",
		ReplyTo: platform.InboundMessage{SessionKey: "local:sess-1"},
		Content: "Reply content",
	}
	require.NoError(t, sender.Send(context.Background(), msg))

	payload := receivePayload(t, ch)
	assert.Equal(t, "Reply content", payload.Content)
	assert.Equal(t, "out-1", payload.ID)
}

func TestLocalSender_Send_FallsBackToSessionKeyAndGeneratesID(t *testing.T) {
	hub := local.NewSSEHub()
	root := t.TempDir()
	sender := local.NewLocalSender(hub, root, nil)
	src := writeAgentFile(t, "a.png", "a")

	ch, unsub := hub.Subscribe("local:user-1")
	defer unsub()

	require.NoError(t, sender.Send(context.Background(), platform.OutboundMessage{SessionKey: "local:user-1", MediaPath: src}))
	require.NoError(t, sender.Send(context.Background(), platform.OutboundMessage{SessionKey: "local:user-1", MediaPath: src}))

	first, second := receivePayload(t, ch), receivePayload(t, ch)
	require.NotEmpty(t, first.ID)
	require.NotEqual(t, first.ID, second.ID)
	assert.Equal(t, []string{first.ID + "_a.png"}, first.MediaPaths)
	assert.Equal(t, []string{second.ID + "_a.png"}, second.MediaPaths)
	assert.FileExists(t, filepath.Join(root, "user-1", first.ID+"_a.png"))
	assert.FileExists(t, filepath.Join(root, "user-1", second.ID+"_a.png"))
}

func TestLocalSender_PrepareOutbound_StagesMediaIntoSessionDir(t *testing.T) {
	root := t.TempDir()
	sender := local.NewLocalSender(local.NewSSEHub(), root, nil)
	src := writeAgentFile(t, "chart.png", "png-bytes")

	msg, err := sender.PrepareOutbound(context.Background(), platform.OutboundMessage{
		ID:        "out-2",
		ReplyTo:   platform.InboundMessage{SessionKey: "local:user-1"},
		MediaPath: src,
	})

	require.NoError(t, err)
	staged := filepath.Join(root, "user-1", "out-2_chart.png")
	assert.Equal(t, staged, msg.MediaPath)
	data, err := os.ReadFile(staged)
	require.NoError(t, err)
	assert.Equal(t, "png-bytes", string(data))

	again, err := sender.PrepareOutbound(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, staged, again.MediaPath)
}

func TestLocalSender_PrepareOutbound_RedirectsClaimedLegacySession(t *testing.T) {
	root := t.TempDir()
	legacy := local.NewLegacySession(&fakeClaimStore{}, root)
	require.NoError(t, legacy.Claim(context.Background(), "owner"))
	sender := local.NewLocalSender(local.NewSSEHub(), root, legacy)
	src := writeAgentFile(t, "late.txt", "late")

	msg, err := sender.PrepareOutbound(context.Background(), platform.OutboundMessage{
		ID:         "out-4",
		SessionKey: "local:default",
		ReplyTo:    platform.InboundMessage{SessionKey: "local:default"},
		MediaPath:  src,
	})

	require.NoError(t, err)
	assert.Equal(t, "local:owner", msg.ReplyTo.SessionKey)
	assert.Equal(t, "local:owner", msg.SessionKey)
	assert.Equal(t, filepath.Join(root, "owner", "out-4_late.txt"), msg.MediaPath)
}

func TestLocalSender_Send_MissingMediaFails(t *testing.T) {
	hub := local.NewSSEHub()
	sender := local.NewLocalSender(hub, t.TempDir(), nil)

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
