package task_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/domain/task"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

func TestMain(m *testing.M) {
	// Initialize i18n with English so message assertions use English strings.
	if err := i18n.Load("en"); err != nil {
		panic("i18n.Load: " + err.Error())
	}
	os.Exit(m.Run())
}

// --- helpers ---

type spySender struct {
	mu   sync.Mutex
	sent []platform.OutboundMessage
}

func (s *spySender) Send(_ context.Context, msg platform.OutboundMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, msg)
	return nil
}

func setupNotifier(t *testing.T, platformID string) (*task.PlatformFailureNotifier, *store.MessageStore, *spySender) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	ms := store.NewMessageStore(db)
	sender := &spySender{}
	senders := map[string]platform.PlatformSenderAdapter{platformID: sender}
	notifier := task.NewPlatformFailureNotifier(ms, senders)
	return notifier, ms, sender
}

// --- tests ---

func TestPlatformFailureNotifier_MessageNotFound(t *testing.T) {
	notifier, _, _ := setupNotifier(t, "test")
	ctx := context.Background()

	err := notifier.NotifyTaskFailure(ctx, "nonexistent-msg", model.FailureInfo{Reason: "some error"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get message")
}

func TestPlatformFailureNotifier_UnknownPlatform(t *testing.T) {
	notifier, ms, _ := setupNotifier(t, "feishu")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-2", "sess-2", "dingtalk", "hi", `{}`, "", 0)
	require.NoError(t, err)

	err = notifier.NotifyTaskFailure(ctx, "msg-2", model.FailureInfo{Reason: "boom"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no sender for platform")
}

func TestPlatformFailureNotifier_TruncatesLongMessage(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-3", "sess-3", "test", "hi", `{}`, "", 0)
	require.NoError(t, err)

	longReason := strings.Repeat("error ", 300) // 1800 ASCII chars, exceeds 500 rune limit
	info := model.FailureInfo{
		Reason:     longReason,
		WorkerName: "w",
	}
	require.NoError(t, notifier.NotifyTaskFailure(ctx, "msg-3", info))

	sender.mu.Lock()
	defer sender.mu.Unlock()
	require.Len(t, sender.sent, 1)

	content := sender.sent[0].Content
	runes := []rune(content)
	assert.LessOrEqual(t, len(runes), 500, "expected content truncated to <= 500 runes")
	assert.True(t, strings.HasSuffix(content, "…"), "expected truncated content to end with '…', got: %s", content[len(content)-10:])
}

func TestPlatformFailureNotifier_CancelSuccess(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-cancel-1", "sess-cancel", "test", "cancel me", `{"raw":true}`, "", 0)
	require.NoError(t, err)

	require.NoError(t, notifier.NotifyTaskCancelled(ctx, "msg-cancel-1", "my-worker"))

	sender.mu.Lock()
	defer sender.mu.Unlock()
	require.Len(t, sender.sent, 1)
	content := sender.sent[0].Content
	assert.Contains(t, content, "Task was cancelled")
	assert.Contains(t, content, "my-worker")
	assert.NotContains(t, content, "Task execution failed", "cancel notification must not contain failure prefix")
	assert.NotContains(t, content, "Error:", "cancel notification must not contain error line")
	assert.Equal(t, "test", sender.sent[0].ReplyTo.Platform)
}

func TestPlatformFailureNotifier_StructuredFormat(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-5", "sess-5", "test", "hi", `{}`, "", 0)
	require.NoError(t, err)

	info := model.FailureInfo{
		Reason:     "launch failed",
		WorkerName: "worker-abc",
	}
	require.NoError(t, notifier.NotifyTaskFailure(ctx, "msg-5", info))

	sender.mu.Lock()
	defer sender.mu.Unlock()
	require.Len(t, sender.sent, 1)
	msg := sender.sent[0]
	assert.Contains(t, msg.Content, "Task execution failed")
	assert.Contains(t, msg.Content, "launch failed")
	assert.Contains(t, msg.Content, "worker-abc")
	assert.NotContains(t, msg.Content, "Retried")
	assert.Equal(t, "test", msg.ReplyTo.Platform)
}
