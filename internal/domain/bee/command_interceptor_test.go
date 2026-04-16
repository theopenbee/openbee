package bee_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/domain/bee"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

func TestMain(m *testing.M) {
	if err := i18n.Load("zh"); err != nil {
		panic("i18n.Load: " + err.Error())
	}
	os.Exit(m.Run())
}

// --- mock types ---

type mockExecStopper struct {
	mu      sync.Mutex
	stopped []string
	err     error
}

func (m *mockExecStopper) StopExecution(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = append(m.stopped, id)
	return m.err
}

type mockSessionClearer struct {
	mu      sync.Mutex
	cleared []string
}

func (m *mockSessionClearer) ClearSession(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleared = append(m.cleared, key)
}

type mockMsgCanceller struct {
	mu        sync.Mutex
	cancelled []string // session keys passed
	n         int64
	err       error
}

func (m *mockMsgCanceller) CancelReceivedBySessionKey(_ context.Context, sessionKey string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelled = append(m.cancelled, sessionKey)
	return m.n, m.err
}

type mockSender struct {
	mu     sync.Mutex
	sent   []platform.OutboundMessage
	notify chan struct{}
}

func newMockSender() *mockSender {
	return &mockSender{notify: make(chan struct{}, 10)}
}

func (m *mockSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	m.mu.Lock()
	m.sent = append(m.sent, msg)
	m.mu.Unlock()
	select {
	case m.notify <- struct{}{}:
	default:
	}
	return nil
}

func setupCommandInterceptorTest(t *testing.T) (
	*store.SessionStore,
	*store.ExecutionStore,
	*store.TaskStore,
	*mockExecStopper,
	*mockSessionClearer,
	*mockMsgCanceller,
	*mockSender,
	*bee.CommandInterceptor,
) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ss := store.NewSessionStore(db)
	es := store.NewExecutionStore(db, t.TempDir())
	ts := store.NewTaskStore(db)

	stopper := &mockExecStopper{}
	clearer := &mockSessionClearer{}
	canceller := &mockMsgCanceller{}
	sender := newMockSender()
	senders := map[string]platform.PlatformSenderAdapter{"local": sender}

	ci := bee.NewCommandInterceptor(ss, es, ts, stopper, clearer, canceller, senders, "claude-code")
	return ss, es, ts, stopper, clearer, canceller, sender, ci
}

func TestCommandInterceptor_InterceptInbound_Stop_ReturnsTrue(t *testing.T) {
	_, _, _, _, clearer, _, sender, ci := setupCommandInterceptorTest(t)

	msg := platform.InboundMessage{
		Platform:   "local",
		SessionKey: "local:1",
		Content:    "/stop",
	}
	handled := ci.InterceptInbound(context.Background(), msg)
	if !handled {
		t.Error("expected InterceptInbound to return true for /stop")
	}

	select {
	case <-sender.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for handleStop goroutine")
	}

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) == 0 || clearer.cleared[0] != "local:1" {
		t.Errorf("expected ClearSession called with 'local:1', got %v", clearer.cleared)
	}
	if len(sender.sent) == 0 {
		t.Fatal("expected reply sent")
	}
}

func TestCommandInterceptor_InterceptInbound_NonStop_ReturnsFalse(t *testing.T) {
	_, _, _, _, _, _, _, ci := setupCommandInterceptorTest(t)

	msg := platform.InboundMessage{
		Platform:   "local",
		SessionKey: "local:1",
		Content:    "hello world",
	}
	if ci.InterceptInbound(context.Background(), msg) {
		t.Error("expected InterceptInbound to return false for non-stop message")
	}
}

func TestCommandInterceptor_InterceptInbound_CaseInsensitive(t *testing.T) {
	_, _, _, _, _, _, sender, ci := setupCommandInterceptorTest(t)

	for _, content := range []string{"/STOP", "/Stop", "  /stop  "} {
		msg := platform.InboundMessage{
			Platform:   "local",
			SessionKey: "local:1",
			Content:    content,
		}
		if !ci.InterceptInbound(context.Background(), msg) {
			t.Errorf("expected handled=true for %q", content)
		}
		select {
		case <-sender.notify:
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for handleStop goroutine for %q", content)
		}
	}
}
