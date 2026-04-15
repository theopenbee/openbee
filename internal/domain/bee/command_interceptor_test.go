package bee_test

import (
	"context"
	"sync"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/bee"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

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

type mockSender struct {
	mu   sync.Mutex
	sent []platform.OutboundMessage
}

func (m *mockSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func setupCommandInterceptorTest(t *testing.T) (
	*store.SessionStore,
	*store.ExecutionStore,
	*store.TaskStore,
	*mockExecStopper,
	*mockSessionClearer,
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
	sender := &mockSender{}
	senders := map[string]platform.PlatformSenderAdapter{"local": sender}

	ci := bee.NewCommandInterceptor(ss, es, ts, stopper, clearer, senders, "claude-code")
	return ss, es, ts, stopper, clearer, sender, ci
}

func TestCommandInterceptor_NonCommand_NotHandled(t *testing.T) {
	_, _, _, _, _, _, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: "hello world"}}
	handled, err := ci.Intercept(ctx, "local:1", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Error("expected handled=false for non-command message")
	}
}

func TestCommandInterceptor_EmptyContent_NotHandled(t *testing.T) {
	_, _, _, _, _, _, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: "   "}}
	handled, _ := ci.Intercept(ctx, "local:1", msgs)
	if handled {
		t.Error("expected handled=false for whitespace-only message")
	}
}

func TestCommandInterceptor_Stop_NoActiveTasks_SendsNoTasksReply(t *testing.T) {
	_, _, _, _, clearer, sender, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: "/stop"}}
	handled, err := ci.Intercept(ctx, "local:1", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Error("expected handled=true for /stop")
	}
	if len(clearer.cleared) == 0 || clearer.cleared[0] != "local:1" {
		t.Errorf("expected ClearSession called with 'local:1', got %v", clearer.cleared)
	}
	if len(sender.sent) == 0 {
		t.Fatal("expected reply sent")
	}
	if sender.sent[0].Content != "当前会话没有正在运行的任务" {
		t.Errorf("unexpected reply: %q", sender.sent[0].Content)
	}
}

func TestCommandInterceptor_Stop_WithRunningExecution_StopsAndReplies(t *testing.T) {
	ss, es, _, stopper, clearer, sender, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	sessionID := "sess-abc"
	if err := ss.UpsertSessionContext(ctx, "local:1", store.BeeAgentID, sessionID, "claude-code"); err != nil {
		t.Fatal(err)
	}
	exec, err := es.CreateBeeExecution(sessionID, "do something")
	if err != nil {
		t.Fatal(err)
	}
	if err := es.UpdateStatus(exec.ID, model.ExecStatusRunning); err != nil {
		t.Fatal(err)
	}

	msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: "/stop"}}
	handled, err := ci.Intercept(ctx, "local:1", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Error("expected handled=true for /stop")
	}
	stopper.mu.Lock()
	defer stopper.mu.Unlock()
	if len(stopper.stopped) == 0 || stopper.stopped[0] != exec.ID {
		t.Errorf("expected StopExecution called with %q, got %v", exec.ID, stopper.stopped)
	}
	if len(clearer.cleared) == 0 {
		t.Error("expected ClearSession called")
	}
	if len(sender.sent) == 0 {
		t.Fatal("expected reply sent")
	}
	if sender.sent[0].Content != "已停止当前会话的所有任务" {
		t.Errorf("unexpected reply: %q", sender.sent[0].Content)
	}
}

func TestCommandInterceptor_Stop_StopExecutionError_StillSendsReply(t *testing.T) {
	ss, es, _, stopper, _, sender, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	sessionID := "sess-xyz"
	if err := ss.UpsertSessionContext(ctx, "local:1", store.BeeAgentID, sessionID, "claude-code"); err != nil {
		t.Fatal(err)
	}
	exec, err := es.CreateBeeExecution(sessionID, "some task")
	if err != nil {
		t.Fatal(err)
	}
	if err := es.UpdateStatus(exec.ID, model.ExecStatusRunning); err != nil {
		t.Fatal(err)
	}
	// Simulate stop failure (process already exited)
	stopper.err = context.DeadlineExceeded

	msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: "/stop"}}
	handled, err := ci.Intercept(ctx, "local:1", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Error("expected handled=true")
	}
	// Reply is still sent even when StopExecution fails
	if len(sender.sent) == 0 {
		t.Fatal("expected reply sent despite stop error")
	}
}

func TestCommandInterceptor_Stop_CaseInsensitive(t *testing.T) {
	_, _, _, _, _, _, ci := setupCommandInterceptorTest(t)
	ctx := context.Background()

	for _, content := range []string{"/STOP", "/Stop", "  /stop  "} {
		msgs := []store.ClaimedMessage{{ID: "m1", SessionKey: "local:1", Platform: "local", Content: content}}
		handled, _ := ci.Intercept(ctx, "local:1", msgs)
		if !handled {
			t.Errorf("expected /stop to be handled regardless of case/whitespace, got false for %q", content)
		}
	}
}
