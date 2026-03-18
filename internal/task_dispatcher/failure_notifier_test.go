package task_dispatcher_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
	"github.com/theopenbee/openbee/internal/task_dispatcher"
)

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

	// Insert a message to look up.
	_, err := ms.Create(ctx, "msg-1", "sess-1", "test", "hello", `{"raw":true}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	err = notifier.NotifyTaskFailure(ctx, "msg-1", "API Error: content filtered")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.sent))
	}
	msg := sender.sent[0]
	if !strings.Contains(msg.Content, "任务执行失败") {
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

	err := notifier.NotifyTaskFailure(ctx, "nonexistent-msg", "some error")
	if err == nil {
		t.Fatal("expected error for nonexistent message, got nil")
	}
	if !strings.Contains(err.Error(), "get message") {
		t.Errorf("expected 'get message' in error, got: %v", err)
	}
}

func TestPlatformFailureNotifier_UnknownPlatform(t *testing.T) {
	notifier, ms, _ := setupNotifier(t, "feishu") // only feishu sender registered
	ctx := context.Background()

	// Insert message with platform "dingtalk" which has no sender.
	_, err := ms.Create(ctx, "msg-2", "sess-2", "dingtalk", "hi", `{}`, "", 0)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	err = notifier.NotifyTaskFailure(ctx, "msg-2", "boom")
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

	// Create a reason with > 500 runes (Chinese characters to test UTF-8 safety).
	longReason := strings.Repeat("错误", 300) // 600 Chinese chars
	err = notifier.NotifyTaskFailure(ctx, "msg-3", longReason)
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
