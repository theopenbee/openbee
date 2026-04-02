package task_dispatcher_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/theopenbee/openbee/internal/i18n"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
	"github.com/theopenbee/openbee/internal/task_dispatcher"
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

func setupNotifier(t *testing.T, platformID string) (*task_dispatcher.PlatformFailureNotifier, *store.MessageStore, *spySender) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ms := store.NewMessageStore(db)
	sender := &spySender{}
	senders := map[string]platform.PlatformSenderAdapter{platformID: sender}
	notifier := task_dispatcher.NewPlatformFailureNotifier(ms, senders)
	return notifier, ms, sender
}

// --- tests ---

func TestPlatformFailureNotifier_Success(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-1", "sess-1", "test", "hello", `{"raw":true}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	info := model.FailureInfo{
		Reason:     "API Error: content filtered",
		WorkerName: "my-worker",
		RetryCount: -1,
	}
	err = notifier.NotifyTaskFailure(ctx, "msg-1", info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.sent))
	}
	msg := sender.sent[0]
	if !strings.Contains(msg.Content, "Task execution failed") {
		t.Errorf("expected failure prefix, got: %s", msg.Content)
	}
	if !strings.Contains(msg.Content, "API Error: content filtered") {
		t.Errorf("expected reason in content, got: %s", msg.Content)
	}
	if msg.ReplyTo.Platform != "test" {
		t.Errorf("expected platform=test, got %s", msg.ReplyTo.Platform)
	}
}

func TestPlatformFailureNotifier_MessageNotFound(t *testing.T) {
	notifier, _, _ := setupNotifier(t, "test")
	ctx := context.Background()

	err := notifier.NotifyTaskFailure(ctx, "nonexistent-msg", model.FailureInfo{Reason: "some error", RetryCount: -1})
	if err == nil {
		t.Fatal("expected error for nonexistent message, got nil")
	}
	if !strings.Contains(err.Error(), "get message") {
		t.Errorf("expected 'get message' in error, got: %v", err)
	}
}

func TestPlatformFailureNotifier_UnknownPlatform(t *testing.T) {
	notifier, ms, _ := setupNotifier(t, "feishu")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-2", "sess-2", "dingtalk", "hi", `{}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	err = notifier.NotifyTaskFailure(ctx, "msg-2", model.FailureInfo{Reason: "boom", RetryCount: -1})
	if err == nil {
		t.Fatal("expected error for unknown platform, got nil")
	}
	if !strings.Contains(err.Error(), "no sender for platform") {
		t.Errorf("expected 'no sender for platform' in error, got: %v", err)
	}
}

func TestPlatformFailureNotifier_TruncatesLongMessage(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-3", "sess-3", "test", "hi", `{}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	longReason := strings.Repeat("error ", 300) // 1800 ASCII chars, exceeds 500 rune limit
	info := model.FailureInfo{
		Reason:     longReason,
		WorkerName: "w",
		RetryCount: -1,
	}
	err = notifier.NotifyTaskFailure(ctx, "msg-3", info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.sent))
	}

	content := sender.sent[0].Content
	runes := []rune(content)
	if len(runes) > 500 {
		t.Errorf("expected content truncated to <= 500 runes, got %d runes", len(runes))
	}
	if !strings.HasSuffix(content, "…") {
		t.Errorf("expected truncated content to end with '…', got: %s", content[len(content)-10:])
	}
}

func TestPlatformFailureNotifier_StructuredFormat_WithRetry(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-4", "sess-4", "test", "hi", `{}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	info := model.FailureInfo{
		Reason:     "exit status 1",
		WorkerName: "data-analyst",
		RetryCount: 3,
		MaxRetries: 3,
	}
	if err := notifier.NotifyTaskFailure(ctx, "msg-4", info); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	content := sender.sent[0].Content
	if !strings.Contains(content, "data-analyst") {
		t.Errorf("expected WorkerName in content, got: %s", content)
	}
	if !strings.Contains(content, "Retried: 3/3") {
		t.Errorf("expected retry line in content, got: %s", content)
	}
	if !strings.Contains(content, "exit status 1") {
		t.Errorf("expected Reason in content, got: %s", content)
	}
}

func TestPlatformFailureNotifier_StructuredFormat_NoRetry(t *testing.T) {
	notifier, ms, sender := setupNotifier(t, "test")
	ctx := context.Background()

	_, err := ms.Create(ctx, "msg-5", "sess-5", "test", "hi", `{}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	info := model.FailureInfo{
		Reason:     "launch failed",
		WorkerName: "worker-abc",
		RetryCount: -1,
	}
	if err := notifier.NotifyTaskFailure(ctx, "msg-5", info); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	content := sender.sent[0].Content
	if strings.Contains(content, "Retried") {
		t.Errorf("expected no retry line when RetryCount=-1, got: %s", content)
	}
	if !strings.Contains(content, "worker-abc") {
		t.Errorf("expected WorkerName in content, got: %s", content)
	}
}
