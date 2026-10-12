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
	sender := local.NewLocalSender(hub, t.TempDir())

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
	sender := local.NewLocalSender(hub, root)
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

func TestLocalSender_Send_CopiesMediaSoLaterRewritesDoNotLeak(t *testing.T) {
	root := t.TempDir()
	sender := local.NewLocalSender(local.NewSSEHub(), root)
	src := writeAgentFile(t, "chart.png", "v1")

	require.NoError(t, sender.Send(context.Background(), platform.OutboundMessage{
		ID:        "out-2",
		ReplyTo:   platform.InboundMessage{SessionKey: "local:user-1"},
		MediaPath: src,
	}))
	require.NoError(t, os.WriteFile(src, []byte("v2-overwritten"), 0o644))

	staged := filepath.Join(root, "user-1", "out-2_chart.png")
	data, err := os.ReadFile(staged)
	require.NoError(t, err)
	assert.Equal(t, "v1", string(data))
	srcInfo, err := os.Stat(src)
	require.NoError(t, err)
	stagedInfo, err := os.Stat(staged)
	require.NoError(t, err)
	assert.False(t, os.SameFile(srcInfo, stagedInfo))
}

func TestLocalSender_Send_StagesFileAlreadyInMediaDir(t *testing.T) {
	root := t.TempDir()
	sender := local.NewLocalSender(local.NewSSEHub(), root)
	userDir := filepath.Join(root, "user-1")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	src := filepath.Join(userDir, "upload_a.png")
	require.NoError(t, os.WriteFile(src, []byte("a"), 0o644))

	require.NoError(t, sender.Send(context.Background(), platform.OutboundMessage{
		ID:        "out-5",
		ReplyTo:   platform.InboundMessage{SessionKey: "local:user-1"},
		MediaPath: src,
	}))

	assert.FileExists(t, filepath.Join(userDir, "out-5_upload_a.png"))
}

func TestLocalSender_Send_RejectsUnreadableMediaSources(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user-1")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	dangling := filepath.Join(t.TempDir(), "dangling")
	require.NoError(t, os.Symlink("../missing-target", dangling))
	sender := local.NewLocalSender(local.NewSSEHub(), root)

	for name, src := range map[string]string{
		"missing in media dir": filepath.Join(userDir, "typo.png"),
		"directory":            t.TempDir(),
		"dangling symlink":     dangling,
	} {
		err := sender.Send(context.Background(), platform.OutboundMessage{
			ID:        "out-6",
			ReplyTo:   platform.InboundMessage{SessionKey: "local:user-1"},
			MediaPath: src,
		})
		assert.Error(t, err, name)
	}
	entries, err := os.ReadDir(userDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestLocalSender_Send_FailsWithoutMediaRoot(t *testing.T) {
	sender := local.NewLocalSender(local.NewSSEHub(), "")
	src := writeAgentFile(t, "a.png", "a")

	err := sender.Send(context.Background(), platform.OutboundMessage{
		ID:        "out-7",
		ReplyTo:   platform.InboundMessage{SessionKey: "local:user-1"},
		MediaPath: src,
	})

	require.ErrorContains(t, err, "media root is unavailable")
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
