package local_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
)

func receiveChat(t *testing.T, ch <-chan string) local.ChatMessage {
	t.Helper()
	ev := recvEvent(t, ch)
	require.Equal(t, "message", ev.Type)
	require.NotNil(t, ev.Message)
	return *ev.Message
}

func newTestSender(hub *local.SSEHub, mediaRoot string) *local.LocalSender {
	return local.NewLocalSender(local.NewEventPublisher(hub), mediaRoot)
}

func writeAgentFile(t *testing.T, name, content string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(src, []byte(content), 0o644))
	return src
}

func TestLocalSender_Send_Broadcasts(t *testing.T) {
	hub := local.NewSSEHub()
	sender := newTestSender(hub, t.TempDir())

	ch, unsub := hub.Subscribe("sess-1")
	defer unsub()

	before := time.Now().UnixMilli()
	msg := platform.OutboundMessage{
		ID:      "out-1",
		ReplyTo: platform.InboundMessage{SessionKey: "local:sess-1"},
		Content: "Reply content",
	}
	require.NoError(t, sender.Send(context.Background(), msg))

	ev := recvEvent(t, ch)
	assert.Equal(t, "message", ev.Type)
	assert.Equal(t, "bee", ev.ConversationID)
	require.NotNil(t, ev.Message)
	assert.Equal(t, "out-1", ev.Message.ID)
	assert.Equal(t, "Reply content", ev.Message.Content)
	assert.Equal(t, local.Sender{Type: "bee"}, ev.Message.Sender)
	assert.Equal(t, "message", ev.Message.Kind)
	assert.GreaterOrEqual(t, ev.Message.TS, before)
	assert.Nil(t, ev.Message.Handoff)
}

func TestSendWorkerMessageAttribution(t *testing.T) {
	hub := local.NewSSEHub()
	sender := newTestSender(hub, t.TempDir())
	ch, unsub := hub.Subscribe("u1")
	defer unsub()

	require.NoError(t, sender.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "local:u1:worker:w1",
		Content:    "done",
		SourceType: "worker",
		SourceID:   "w1",
	}))

	ev := recvEvent(t, ch)
	assert.Equal(t, "worker:w1", ev.ConversationID)
	require.NotNil(t, ev.Message)
	assert.Equal(t, local.Sender{Type: "worker", ID: "w1"}, ev.Message.Sender)
	assert.Equal(t, "message", ev.Message.Kind)
}

func TestSendHandoffCard(t *testing.T) {
	hub := local.NewSSEHub()
	sender := newTestSender(hub, t.TempDir())
	ch, unsub := hub.Subscribe("u1")
	defer unsub()

	require.NoError(t, sender.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "local:u1",
		Content:    "handed to w1",
		Kind:       "handoff",
		Payload:    `{"worker_id":"w1","task_id":"t1","task_type":"scheduled","cron_expr":"0 9 * * *","scheduled_at":1700000000000,"conversation_id":"worker:w1"}`,
	}))

	msg := receiveChat(t, ch)
	assert.Equal(t, "handoff", msg.Kind)
	require.NotNil(t, msg.Handoff)
	scheduledAt := int64(1700000000000)
	assert.Equal(t, local.HandoffInfo{
		WorkerID:       "w1",
		TaskID:         "t1",
		TaskType:       "scheduled",
		CronExpr:       "0 9 * * *",
		ScheduledAt:    &scheduledAt,
		ConversationID: "worker:w1",
	}, *msg.Handoff)
}

func TestSendHandoffRejectsBadPayload(t *testing.T) {
	hub := local.NewSSEHub()
	sender := newTestSender(hub, t.TempDir())
	ch, unsub := hub.Subscribe("u1")
	defer unsub()

	for _, payload := range []string{"", "not json"} {
		err := sender.Send(context.Background(), platform.OutboundMessage{
			SessionKey: "local:u1",
			Kind:       "handoff",
			Payload:    payload,
		})
		require.ErrorContains(t, err, "handoff payload", payload)
	}
	assertNoEvent(t, ch)
}

func TestSendIgnoresPayloadForPlainMessages(t *testing.T) {
	hub := local.NewSSEHub()
	sender := newTestSender(hub, t.TempDir())
	ch, unsub := hub.Subscribe("u1")
	defer unsub()

	require.NoError(t, sender.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "local:u1",
		Content:    "hi",
		Payload:    `{"task_id":"t1"}`,
	}))

	assert.Nil(t, receiveChat(t, ch).Handoff)
}

func TestSendStagesWorkerChatMediaInUserDir(t *testing.T) {
	root := t.TempDir()
	hub := local.NewSSEHub()
	sender := newTestSender(hub, root)
	ch, unsub := hub.Subscribe("u1")
	defer unsub()
	src := writeAgentFile(t, "chart.png", "x")

	require.NoError(t, sender.Send(context.Background(), platform.OutboundMessage{
		ID:         "out-9",
		SessionKey: "local:u1:worker:w1",
		MediaPath:  src,
	}))

	msg := receiveChat(t, ch)
	assert.Equal(t, []string{"out-9_chart.png"}, msg.MediaPaths)
	assert.FileExists(t, filepath.Join(root, "u1", "out-9_chart.png"))
}

func TestLocalSender_Send_FallsBackToSessionKeyAndGeneratesID(t *testing.T) {
	hub := local.NewSSEHub()
	root := t.TempDir()
	sender := newTestSender(hub, root)
	src := writeAgentFile(t, "a.png", "a")

	ch, unsub := hub.Subscribe("user-1")
	defer unsub()

	require.NoError(t, sender.Send(context.Background(), platform.OutboundMessage{SessionKey: "local:user-1", MediaPath: src}))
	require.NoError(t, sender.Send(context.Background(), platform.OutboundMessage{SessionKey: "local:user-1", MediaPath: src}))

	first, second := receiveChat(t, ch), receiveChat(t, ch)
	require.NotEmpty(t, first.ID)
	require.NotEqual(t, first.ID, second.ID)
	assert.Equal(t, []string{first.ID + "_a.png"}, first.MediaPaths)
	assert.Equal(t, []string{second.ID + "_a.png"}, second.MediaPaths)
	assert.FileExists(t, filepath.Join(root, "user-1", first.ID+"_a.png"))
	assert.FileExists(t, filepath.Join(root, "user-1", second.ID+"_a.png"))
}

func TestLocalSender_Send_CopiesMediaSoLaterRewritesDoNotLeak(t *testing.T) {
	root := t.TempDir()
	sender := newTestSender(local.NewSSEHub(), root)
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
	sender := newTestSender(local.NewSSEHub(), root)
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
	sender := newTestSender(local.NewSSEHub(), root)

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
	sender := newTestSender(local.NewSSEHub(), "")
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
	sender := newTestSender(hub, t.TempDir())

	ch, unsub := hub.Subscribe("user-1")
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

func TestLocalSender_UsesSentAtAsTimestamp(t *testing.T) {
	hub := local.NewSSEHub()
	sender := newTestSender(hub, t.TempDir())
	ch, unsub := hub.Subscribe("sess-1")
	defer unsub()

	require.NoError(t, sender.Send(context.Background(), platform.OutboundMessage{
		ID: "out-ts", SessionKey: "local:sess-1", Content: "x", SentAt: 1234,
	}))

	assert.Equal(t, int64(1234), receiveChat(t, ch).TS)
}
