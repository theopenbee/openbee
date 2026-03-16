package mcp_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/robobee/core/internal/config"
	"github.com/robobee/core/internal/mcp"
	"github.com/robobee/core/internal/model"
	"github.com/robobee/core/internal/platform"
	"github.com/robobee/core/internal/store"
	"github.com/robobee/core/internal/worker"
)

func setupMCPServerWithMessaging(t *testing.T) *mcp.MCPServer {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db)
	ts := store.NewTaskStore(db)
	ms := store.NewMessageStore(db)
	mgr := worker.NewManager(
		t.TempDir(),
		config.BeeConfig{Claude: config.ClaudeConfig{Path: "claude"}},
		ws, es,
	)
	senders := make(map[string]platform.PlatformSenderAdapter)
	return mcp.NewServer(ws, mgr, ts, ms, senders, nil, nil)
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestCallTool_ListWorkers_Empty(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool("list_workers", mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	workers, ok := result.([]model.Worker)
	if !ok {
		t.Fatalf("expected []model.Worker, got %T", result)
	}
	if len(workers) != 0 {
		t.Errorf("expected empty slice, got %d workers", len(workers))
	}
}

func TestCallTool_CreateWorker(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool("create_worker", mustMarshal(t, map[string]any{
		"name":        "TestBot",
		"description": "A test bot",
		"prompt":      "You are a test bot.",
	}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	w, ok := result.(model.Worker)
	if !ok {
		t.Fatalf("expected model.Worker, got %T", result)
	}
	if w.ID == "" {
		t.Error("expected non-empty worker ID")
	}
	if w.Name != "TestBot" {
		t.Errorf("expected name TestBot, got %s", w.Name)
	}
}

func TestCallTool_GetWorker(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	created, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "Bot"}))
	w := created.(model.Worker)

	result, err := s.CallTool("get_worker", mustMarshal(t, map[string]any{"worker_id": w.ID}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	fetched, ok := result.(model.Worker)
	if !ok {
		t.Fatalf("expected model.Worker, got %T", result)
	}
	if fetched.ID != w.ID {
		t.Errorf("expected ID %s, got %s", w.ID, fetched.ID)
	}
}

func TestCallTool_GetWorker_NotFound(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("get_worker", mustMarshal(t, map[string]any{"worker_id": "nonexistent"}))
	if err == nil {
		t.Error("expected error for missing worker")
	}
}

func TestCallTool_UpdateWorker(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	created, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "OldName"}))
	w := created.(model.Worker)

	result, err := s.CallTool("update_worker", mustMarshal(t, map[string]any{
		"worker_id": w.ID,
		"name":      "NewName",
		"memory":    "New memory",
	}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	updated := result.(model.Worker)
	if updated.Name != "NewName" {
		t.Errorf("expected NewName, got %s", updated.Name)
	}
	if updated.Memory != "New memory" {
		t.Errorf("expected new memory, got %s", updated.Memory)
	}
	if updated.Description != w.Description {
		t.Errorf("description changed unexpectedly: %s", updated.Description)
	}
}

func TestCallTool_DeleteWorker(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	created, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "Bot"}))
	w := created.(model.Worker)

	_, err := s.CallTool("delete_worker", mustMarshal(t, map[string]any{"worker_id": w.ID}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	_, err = s.CallTool("get_worker", mustMarshal(t, map[string]any{"worker_id": w.ID}))
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestCallTool_UnknownTool(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("nonexistent_tool", mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestToolSchemas_IncludesTaskTools(t *testing.T) {
	schemas := mcp.ToolSchemas()
	names := make(map[string]bool)
	for _, s := range schemas {
		names[s.Name] = true
	}
	for _, want := range []string{"create_task", "list_tasks", "cancel_task"} {
		if !names[want] {
			t.Errorf("missing tool schema: %s", want)
		}
	}
}

func TestListWorkers_ReturnsEmptySlice_NotNull(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool("list_workers", mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	workers := result.([]model.Worker)
	if workers == nil {
		t.Error("expected non-nil slice, got nil")
	}
}

type mockSender struct {
	sent []platform.OutboundMessage
	mu   sync.Mutex
}

func (s *mockSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, msg)
	return nil
}

func setupMCPServerWithSender(t *testing.T, senderID string, sender platform.PlatformSenderAdapter) (*mcp.MCPServer, *sql.DB) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db)
	ts := store.NewTaskStore(db)
	ms := store.NewMessageStore(db)
	mgr := worker.NewManager(
		t.TempDir(),
		config.BeeConfig{Claude: config.ClaudeConfig{Path: "claude"}},
		ws, es,
	)
	senders := map[string]platform.PlatformSenderAdapter{senderID: sender}
	return mcp.NewServer(ws, mgr, ts, ms, senders, nil, nil), db
}

// --- mark_task_success ---

func TestCallTool_MarkTaskSuccess(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()
	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-fake", "feishu:c1:u1", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	taskResult, err := s.CallTool("create_task", mustMarshal(t, map[string]any{
		"message_id":  "msg-fake",
		"worker_id":   w.ID,
		"instruction": "do something",
		"type":        "immediate",
	}))
	if err != nil {
		t.Fatalf("create_task: %v", err)
	}
	taskMap := taskResult.(map[string]string)
	taskID := taskMap["task_id"]

	result, err := s.CallTool("mark_task_success", mustMarshal(t, map[string]any{
		"task_id": taskID,
	}))
	if err != nil {
		t.Fatalf("mark_task_success: %v", err)
	}
	m := result.(map[string]string)
	if m["status"] != "completed" {
		t.Errorf("expected status=completed, got %q", m["status"])
	}
	if m["task_id"] != taskID {
		t.Errorf("expected task_id=%s, got %q", taskID, m["task_id"])
	}
}

func TestCallTool_MarkTaskSuccess_MissingTaskID(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("mark_task_success", mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected error for missing task_id")
	}
}

// --- send_message ---

func TestCallTool_SendMessage_CallsSender(t *testing.T) {
	mock := &mockSender{}
	s, db := setupMCPServerWithSender(t, "feishu", mock)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-send-1", "feishu:chat1:userA", "feishu", "hello", `{"event":{"message":{"chat_id":"c1","chat_type":"p2p","message_id":"m1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`, "", 0) //nolint

	result, err := s.CallTool("send_message", mustMarshal(t, map[string]any{
		"message_id": "msg-send-1",
		"content":    "Task done!",
	}))
	if err != nil {
		t.Fatalf("send_message: %v", err)
	}
	m := result.(map[string]string)
	if m["status"] != "sent" {
		t.Errorf("expected status=sent, got %q", m["status"])
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.sent) == 0 {
		t.Fatal("expected sender.Send to be called")
	}
	if mock.sent[0].Content != "Task done!" {
		t.Errorf("expected content 'Task done!', got %q", mock.sent[0].Content)
	}
}

func TestCallTool_SendMessage_MissingMessageID(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("send_message", mustMarshal(t, map[string]any{
		"content": "hello",
	}))
	if err == nil {
		t.Error("expected error for missing message_id")
	}
}

func TestCallTool_SendMessage_MissingContent(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("send_message", mustMarshal(t, map[string]any{
		"message_id": "msg-x",
	}))
	if err == nil {
		t.Error("expected error for missing content")
	}
}

func TestCallTool_SendMessage_UnknownPlatform(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-unk", "dingtalk:c1:u1", "dingtalk", "hi", `{}`, "", 0) //nolint

	_, err := s.CallTool("send_message", mustMarshal(t, map[string]any{
		"message_id": "msg-unk",
		"content":    "hello",
	}))
	if err == nil {
		t.Error("expected error for unregistered platform sender")
	}
}

func TestCallTool_SendMessage_MessageNotFound(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("send_message", mustMarshal(t, map[string]any{
		"message_id": "nonexistent-msg",
		"content":    "hello",
	}))
	if err == nil {
		t.Error("expected error for nonexistent message_id")
	}
}

// --- Schema count ---

func TestToolSchemas_Count_AfterNewTools(t *testing.T) {
	schemas := mcp.ToolSchemas()
	if len(schemas) != 11 {
		t.Errorf("expected 11 tool schemas, got %d", len(schemas))
	}
}

func TestToolSchemas_IncludesNewTools(t *testing.T) {
	schemas := mcp.ToolSchemas()
	names := make(map[string]bool)
	for _, s := range schemas {
		names[s.Name] = true
	}
	for _, want := range []string{"mark_task_success", "send_message"} {
		if !names[want] {
			t.Errorf("missing tool schema: %s", want)
		}
	}
}

// --- list_tasks session_key tests ---

func TestCallTool_ListTasks_BySessionKey(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()
	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-sk1", "session-X", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	s.CallTool("create_task", mustMarshal(t, map[string]any{
		"message_id": "msg-sk1", "worker_id": w.ID,
		"instruction": "task1", "type": "immediate",
	}))

	result, err := s.CallTool("list_tasks", mustMarshal(t, map[string]any{
		"session_key": "session-X",
	}))
	if err != nil {
		t.Fatalf("list_tasks by session_key: %v", err)
	}
	tasks := result.([]model.Task)
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
}

func TestCallTool_ListTasks_BothParams_Error(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("list_tasks", mustMarshal(t, map[string]any{
		"message_id":  "msg-1",
		"session_key": "session-X",
	}))
	if err == nil {
		t.Error("expected error when both message_id and session_key provided")
	}
}

func TestCallTool_ListTasks_NoParams_Error(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool("list_tasks", mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected error when neither message_id nor session_key provided")
	}
}

// --- Mock implementations for clear_session ---

type mockExecStopper struct {
	mu      sync.Mutex
	stopped []string
}

func (m *mockExecStopper) StopExecution(executionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = append(m.stopped, executionID)
	return nil
}

type mockSessionClearer struct {
	mu      sync.Mutex
	cleared []string
}

func (m *mockSessionClearer) ClearSession(sessionKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleared = append(m.cleared, sessionKey)
}

func setupMCPServerWithClear(t *testing.T) (*mcp.MCPServer, *sql.DB, *mockExecStopper, *mockSessionClearer) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db)
	ts := store.NewTaskStore(db)
	ms := store.NewMessageStore(db)
	mgr := worker.NewManager(
		t.TempDir(),
		config.BeeConfig{Claude: config.ClaudeConfig{Path: "claude"}},
		ws, es,
	)
	senders := make(map[string]platform.PlatformSenderAdapter)
	stopper := &mockExecStopper{}
	clearer := &mockSessionClearer{}
	return mcp.NewServer(ws, mgr, ts, ms, senders, stopper, clearer), db, stopper, clearer
}

func TestCallTool_ClearSession_NoActiveTasks(t *testing.T) {
	s, _, _, clearer := setupMCPServerWithClear(t)

	result, err := s.CallTool("clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-X",
	}))
	if err != nil {
		t.Fatalf("clear_session: %v", err)
	}
	m := result.(map[string]any)
	if m["cleared"] != true {
		t.Errorf("expected cleared=true, got %v", m["cleared"])
	}

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 1 || clearer.cleared[0] != "session-X" {
		t.Errorf("expected ClearSession called with session-X, got %v", clearer.cleared)
	}
}

func TestCallTool_ClearSession_CancelsAndStopsTasks(t *testing.T) {
	s, db, stopper, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-c1", "session-Y", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool("create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	// Create a running task with execution_id
	ts := store.NewTaskStore(db)
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "msg-c1", WorkerID: w.ID, Instruction: "long task",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})
	ts.SetExecution(ctx, id, "exec-running-1", model.TaskStatusRunning)

	// Create a pending task
	ts.Create(ctx, model.Task{
		MessageID: "msg-c1", WorkerID: w.ID, Instruction: "queued task",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: 1, UpdatedAt: 1,
	})

	result, err := s.CallTool("clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-Y",
	}))
	if err != nil {
		t.Fatalf("clear_session: %v", err)
	}
	m := result.(map[string]any)
	cancelled, ok := m["cancelled_tasks"].(int64)
	if !ok || cancelled < 1 {
		t.Errorf("expected cancelled_tasks >= 1, got %v", m["cancelled_tasks"])
	}

	// StopExecution should have been called for the running task
	stopper.mu.Lock()
	defer stopper.mu.Unlock()
	if len(stopper.stopped) != 1 || stopper.stopped[0] != "exec-running-1" {
		t.Errorf("expected StopExecution(exec-running-1), got %v", stopper.stopped)
	}

	// ClearSession should have been called
	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 1 || clearer.cleared[0] != "session-Y" {
		t.Errorf("expected ClearSession(session-Y), got %v", clearer.cleared)
	}
}

func TestCallTool_ClearSession_MissingSessionKey(t *testing.T) {
	s, _, _, _ := setupMCPServerWithClear(t)
	_, err := s.CallTool("clear_session", mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected error for missing session_key")
	}
}
