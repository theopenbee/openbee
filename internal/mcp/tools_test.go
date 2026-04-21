package mcp_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/worker"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/mcp"
	"github.com/theopenbee/openbee/internal/platform"
	_ "modernc.org/sqlite"
)

// stubEngineAdapter is a no-op EngineAdapter for tests that don't exercise the engine.
type stubEngineAdapter struct{}

func (s *stubEngineAdapter) Prepare(_ string, _ ai.PrepareOptions) error {
	return nil
}
func (s *stubEngineAdapter) Run(_ context.Context, _, _ string, _ ai.RunOptions, _ string) (ai.RunResult, error) {
	return ai.RunResult{ExtractResult: func(string) string { return "" }}, nil
}

func setupMCPServerWithMessaging(t *testing.T) *mcp.MCPServer {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db, t.TempDir())
	ts := store.NewTaskStore(db)
	ms := store.NewMessageStore(db)
	mgr := worker.NewManager(
		t.TempDir(),
		config.BeeConfig{Engines: config.EnginesConfig{Claude: config.EngineItemConfig{Path: "claude"}}},
		ws, es,
		map[string]ai.EngineAdapter{"claude": &stubEngineAdapter{}}, enginecfg.NewStore("claude"), nil,
	)
	senders := make(map[string]platform.PlatformSenderAdapter)
	return mcp.NewBeeServer(ws, mgr, ts, ms, store.NewOutboundMessageStore(db), senders, nil, nil, es, store.NewConstraintStore(db), store.NewSessionStore(db), store.NewDepartmentStore(db))
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func decodeResult(t *testing.T, result any) map[string]any {
	t.Helper()
	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return m
}

func decodeListWorkersResult(t *testing.T, result any) (items []any, total int) {
	t.Helper()
	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal list_workers result: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal list_workers result: %v", err)
	}
	items = m["items"].([]any)
	total = int(m["total"].(float64))
	return
}

func TestCallTool_ListWorkers_Empty(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "list_workers", mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	items, total := decodeListWorkersResult(t, result)
	if len(items) != 0 {
		t.Errorf("expected empty items, got %d", len(items))
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
}

func TestCallTool_CreateWorker(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{
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
	created, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "Bot"}))
	w := created.(model.Worker)

	result, err := s.CallTool(context.Background(), "get_worker", mustMarshal(t, map[string]any{"worker_id": w.ID}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	b, _ := json.Marshal(result)
	var fetched map[string]any
	if err := json.Unmarshal(b, &fetched); err != nil {
		t.Fatalf("unmarshal get_worker result: %v", err)
	}
	if fetched["id"].(string) != w.ID {
		t.Errorf("expected ID %s, got %s", w.ID, fetched["id"])
	}
	if _, ok := fetched["departments"]; !ok {
		t.Error("expected departments field in get_worker response")
	}
}

func TestCallTool_GetWorker_NotFound(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool(context.Background(), "get_worker", mustMarshal(t, map[string]any{"worker_id": "nonexistent"}))
	if err == nil {
		t.Error("expected error for missing worker")
	}
}

func TestCallTool_UpdateWorker(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	created, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "OldName"}))
	w := created.(model.Worker)

	result, err := s.CallTool(context.Background(), "update_worker", mustMarshal(t, map[string]any{
		"worker_id":   w.ID,
		"name":        "NewName",
		"constraints": "New constraints",
	}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	updated := result.(model.Worker)
	if updated.Name != "NewName" {
		t.Errorf("expected NewName, got %s", updated.Name)
	}
	if updated.Constraints != "New constraints" {
		t.Errorf("expected new constraints, got %s", updated.Constraints)
	}
	if updated.Description != w.Description {
		t.Errorf("description changed unexpectedly: %s", updated.Description)
	}
}

func TestCallTool_DeleteWorker(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	created, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "Bot"}))
	w := created.(model.Worker)

	_, err := s.CallTool(context.Background(), "delete_worker", mustMarshal(t, map[string]any{"worker_id": w.ID}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	_, err = s.CallTool(context.Background(), "get_worker", mustMarshal(t, map[string]any{"worker_id": w.ID}))
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestCallTool_UnknownTool(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool(context.Background(), "nonexistent_tool", mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestListWorkers_ReturnsEmptySlice_NotNull(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "list_workers", mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items, _ := decodeListWorkersResult(t, result)
	if items == nil {
		t.Error("expected non-nil items, got nil")
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
	es := store.NewExecutionStore(db, t.TempDir())
	ts := store.NewTaskStore(db)
	ms := store.NewMessageStore(db)
	mgr := worker.NewManager(
		t.TempDir(),
		config.BeeConfig{Engines: config.EnginesConfig{Claude: config.EngineItemConfig{Path: "claude"}}},
		ws, es,
		map[string]ai.EngineAdapter{"claude": &stubEngineAdapter{}}, enginecfg.NewStore("claude"), nil,
	)
	senders := map[string]platform.PlatformSenderAdapter{senderID: sender}
	return mcp.NewBeeServer(ws, mgr, ts, ms, store.NewOutboundMessageStore(db), senders, nil, nil, es, store.NewConstraintStore(db), store.NewSessionStore(db), store.NewDepartmentStore(db)), db
}

// --- send_message ---

func TestCallTool_SendMessage_CallsSender(t *testing.T) {
	mock := &mockSender{}
	s, db := setupMCPServerWithSender(t, "feishu", mock)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-send-1", "feishu:chat1:userA", "feishu", "hello", `{"event":{"message":{"chat_id":"c1","chat_type":"p2p","message_id":"m1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`, "", 0) //nolint

	result, err := s.CallTool(context.Background(), "send_message", mustMarshal(t, map[string]any{
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
	_, err := s.CallTool(context.Background(), "send_message", mustMarshal(t, map[string]any{
		"content": "hello",
	}))
	if err == nil {
		t.Error("expected error for missing message_id")
	}
}

func TestCallTool_SendMessage_MissingContent(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool(context.Background(), "send_message", mustMarshal(t, map[string]any{
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

	_, err := s.CallTool(context.Background(), "send_message", mustMarshal(t, map[string]any{
		"message_id": "msg-unk",
		"content":    "hello",
	}))
	if err == nil {
		t.Error("expected error for unregistered platform sender")
	}
}

func TestCallTool_SendMessage_MessageNotFound(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool(context.Background(), "send_message", mustMarshal(t, map[string]any{
		"message_id": "nonexistent-msg",
		"content":    "hello",
	}))
	if err == nil {
		t.Error("expected error for nonexistent message_id")
	}
}

func TestCallTool_SendMessage_WorkerPrefixesContent(t *testing.T) {
	mock := &mockSender{}
	s, db := setupMCPServerWithSender(t, "feishu", mock)
	ctx := context.Background()

	// Create a worker and a message
	ws := store.NewWorkerStore(db)
	w, err := ws.Create(model.Worker{Name: "MaoMao", Description: "test worker"})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-worker-prefix", "feishu:chat1:userA", "feishu", "hello", //nolint
		`{"event":{"message":{"chat_id":"c1","chat_type":"p2p","message_id":"m1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`, "", 0)

	// Call with a context that carries the worker's ID
	workerCtx := context.WithValue(ctx, mcp.CtxWorkerIDKey, w.ID)
	result, err := s.CallTool(workerCtx, "send_message", mustMarshal(t, map[string]any{
		"message_id": "msg-worker-prefix",
		"content":    "task done",
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
	want := "MaoMao\ntask done"
	if mock.sent[0].Content != want {
		t.Errorf("expected content %q, got %q", want, mock.sent[0].Content)
	}
}

func TestCallTool_SendMessage_WorkerDeletedFallsBackToWorkerID(t *testing.T) {
	mock := &mockSender{}
	s, db := setupMCPServerWithSender(t, "feishu", mock)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-deleted-worker", "feishu:chat1:userA", "feishu", "hello", //nolint
		`{"event":{"message":{"chat_id":"c1","chat_type":"p2p","message_id":"m1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`, "", 0)

	// Use a worker ID that does not exist in the store
	workerCtx := context.WithValue(ctx, mcp.CtxWorkerIDKey, "worker-deleted-xyz")
	result, err := s.CallTool(workerCtx, "send_message", mustMarshal(t, map[string]any{
		"message_id": "msg-deleted-worker",
		"content":    "task done",
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
	want := "worker-deleted-xyz\ntask done"
	if mock.sent[0].Content != want {
		t.Errorf("expected content %q, got %q", want, mock.sent[0].Content)
	}
}

// --- list_tasks session_key tests ---

func TestCallTool_ListTasks_BySessionKey(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()
	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-sk1", "session-X", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	s.CallTool(context.Background(), "create_task", mustMarshal(t, map[string]any{
		"message_id": "msg-sk1", "worker_id": w.ID,
		"instruction": "task1", "type": "immediate",
	}))

	result, err := s.CallTool(context.Background(), "list_tasks", mustMarshal(t, map[string]any{
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
	_, err := s.CallTool(context.Background(), "list_tasks", mustMarshal(t, map[string]any{
		"message_id":  "msg-1",
		"session_key": "session-X",
	}))
	if err == nil {
		t.Error("expected error when both message_id and session_key provided")
	}
}

func TestCallTool_ListTasks_NoParams_Error(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool(context.Background(), "list_tasks", mustMarshal(t, map[string]any{}))
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
	es := store.NewExecutionStore(db, t.TempDir())
	ts := store.NewTaskStore(db)
	ms := store.NewMessageStore(db)
	mgr := worker.NewManager(
		t.TempDir(),
		config.BeeConfig{Engines: config.EnginesConfig{Claude: config.EngineItemConfig{Path: "claude"}}},
		ws, es,
		map[string]ai.EngineAdapter{"claude": &stubEngineAdapter{}}, enginecfg.NewStore("claude"), nil,
	)
	senders := make(map[string]platform.PlatformSenderAdapter)
	stopper := &mockExecStopper{}
	clearer := &mockSessionClearer{}
	return mcp.NewBeeServer(ws, mgr, ts, ms, store.NewOutboundMessageStore(db), senders, stopper, clearer, es, store.NewConstraintStore(db), store.NewSessionStore(db), store.NewDepartmentStore(db)), db, stopper, clearer
}

func TestCallTool_ClearSession_NoActiveTasks(t *testing.T) {
	s, _, _, clearer := setupMCPServerWithClear(t)

	result, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{
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

	workerResult, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "W"}))
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

	result, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-Y",
		"force":       true,
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
	_, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected error for missing session_key")
	}
}

func TestCallTool_GetWorkerStatus(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	// Create a worker first
	created, err := s.CallTool(context.Background(), utils.CreateWorker, mustMarshal(t, map[string]any{
		"name": "status-test",
	}))
	if err != nil {
		t.Fatal(err)
	}
	w := created.(model.Worker)

	result, err := s.CallTool(context.Background(), utils.GetWorkerStatus, mustMarshal(t, map[string]any{
		"worker_id": w.ID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["worker_id"] != w.ID {
		t.Errorf("expected worker_id %s, got %v", w.ID, m["worker_id"])
	}
	if m["status"] != "idle" {
		t.Errorf("expected status idle, got %v", m["status"])
	}
}

func TestCallTool_GetSystemOverview(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	result, err := s.CallTool(context.Background(), utils.GetSystemOverview, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]any)
	if m["workers"] == nil {
		t.Error("expected workers section")
	}
	if m["tasks"] == nil {
		t.Error("expected tasks section")
	}
}

func TestCallTool_ListBeeExecutions(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	result, err := s.CallTool(context.Background(), utils.ListBeeExecutions, nil)
	if err != nil {
		t.Fatal(err)
	}
	execs := result.([]map[string]any)
	if len(execs) != 0 {
		t.Errorf("expected empty list, got %d", len(execs))
	}
}

func TestCallTool_SaveConstraint(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	result, err := s.CallTool(context.Background(), utils.SaveConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "test_pref",
		"value": "user likes concise replies",
	}))
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]string)
	if m["status"] != "saved" {
		t.Errorf("expected status saved, got %q", m["status"])
	}
}

func TestCallTool_GetConstraint(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	// Save first
	s.CallTool(context.Background(), utils.SaveConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "pref1",
		"value": "value1",
	}))

	// Get by key
	result, err := s.CallTool(context.Background(), utils.GetConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "pref1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected memory, got nil")
	}

	// List by scope (no key)
	result2, err := s.CallTool(context.Background(), utils.GetConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
	}))
	if err != nil {
		t.Fatal(err)
	}
	_ = result2
}

func TestCallTool_DeleteConstraint(t *testing.T) {
	s := setupMCPServerWithMessaging(t)

	s.CallTool(context.Background(), utils.SaveConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "to_delete",
		"value": "temp",
	}))

	result, err := s.CallTool(context.Background(), utils.DeleteConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "to_delete",
	}))
	if err != nil {
		t.Fatal(err)
	}
	m := result.(map[string]string)
	if m["status"] != "deleted" {
		t.Errorf("expected status deleted, got %q", m["status"])
	}
}

// --- list_session_contexts ---

func TestCallTool_ListSessionContexts_Empty(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "list_session_contexts", mustMarshal(t, map[string]any{
		"session_key": "no-such-session",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	agents, ok := result.([]store.SessionAgent)
	if !ok {
		t.Fatalf("expected []store.SessionAgent, got %T", result)
	}
	if len(agents) != 0 {
		t.Errorf("expected empty slice, got %d", len(agents))
	}
}

func TestCallTool_ListSessionContexts_MissingSessionKey(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool(context.Background(), "list_session_contexts", mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected error for missing session_key")
	}
}

// --- clear_worker_session ---

func TestCallTool_ClearWorkerSession_MissingSessionKey(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool(context.Background(), "clear_worker_session", mustMarshal(t, map[string]any{
		"worker_id": "some-worker",
	}))
	if err == nil {
		t.Error("expected error for missing session_key")
	}
}

func TestCallTool_ClearWorkerSession_MissingWorkerID(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool(context.Background(), "clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "sk",
	}))
	if err == nil {
		t.Error("expected error for missing worker_id")
	}
}

func TestCallTool_ClearWorkerSession_RefusesBee(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool(context.Background(), "clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "sk",
		"worker_id":   "bee",
	}))
	if err == nil {
		t.Error("expected error when worker_id is bee")
	}
}

func TestCallTool_ClearWorkerSession_Idempotent(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	// No session row exists; should succeed without error.
	result, err := s.CallTool(context.Background(), "clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "sk",
		"worker_id":   "nonexistent-worker-id",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["cleared"] != true {
		t.Errorf("expected cleared=true, got %v", m["cleared"])
	}
}

func TestCallTool_ClearWorkerSession_ClearsOnlyTargetWorker(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()

	// Create two workers
	workerResult1, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "W1"}))
	workerResult2, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "W2"}))
	w1 := workerResult1.(model.Worker)
	w2 := workerResult2.(model.Worker)

	// Seed session contexts for both workers
	ss := store.NewSessionStore(db)
	ss.UpsertSessionContext(ctx, "sk", w1.ID, "sid-w1-claude", "claude") //nolint
	ss.UpsertSessionContext(ctx, "sk", w1.ID, "sid-w1-codex", "codex")   //nolint
	ss.UpsertSessionContext(ctx, "sk", w2.ID, "sid-w2", "claude")        //nolint

	// Clear only w1
	result, err := s.CallTool(context.Background(), "clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "sk",
		"worker_id":   w1.ID,
	}))
	if err != nil {
		t.Fatalf("clear_worker_session: %v", err)
	}
	m := result.(map[string]any)
	if m["cleared"] != true {
		t.Errorf("expected cleared=true, got %v", m["cleared"])
	}
	if m["worker_name"] != "W1" {
		t.Errorf("expected worker_name=W1, got %v", m["worker_name"])
	}

	// w1 context should be gone across engines; w2 should remain
	w1Claude, _ := ss.GetSessionContextForEngine(ctx, "sk", w1.ID, "claude")
	w1Codex, _ := ss.GetSessionContextForEngine(ctx, "sk", w1.ID, "codex")
	w2sid, _ := ss.GetSessionContextForEngine(ctx, "sk", w2.ID, "claude")
	if w1Claude != "" || w1Codex != "" {
		t.Errorf("expected w1 context cleared across engines, got claude=%q codex=%q", w1Claude, w1Codex)
	}
	if w2sid != "sid-w2" {
		t.Errorf("expected w2 context intact, got %q", w2sid)
	}
}

// --- clear_session confirmation ---

func TestCallTool_ClearSession_RequiresConfirmation_TwoWorkers(t *testing.T) {
	s, db, _, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-conf1", "session-C", "feishu", "hi", `{}`, "", 0) //nolint

	// Create two workers and seed session contexts for both.
	workerResult1, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "W1"}))
	workerResult2, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "W2"}))
	w1 := workerResult1.(model.Worker)
	w2 := workerResult2.(model.Worker)

	ss := store.NewSessionStore(db)
	ss.UpsertSessionContext(ctx, "session-C", w1.ID, "sid-w1", "") //nolint
	ss.UpsertSessionContext(ctx, "session-C", w2.ID, "sid-w2", "") //nolint

	// Call without force — should get confirmation request, NOT clear.
	result, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-C",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["requires_confirmation"] != true {
		t.Errorf("expected requires_confirmation=true, got %v", m["requires_confirmation"])
	}
	workerCount, _ := m["worker_count"].(int)
	if workerCount != 2 {
		t.Errorf("expected worker_count=2, got %v", m["worker_count"])
	}
	linkedWorkers, _ := m["linked_workers"].([]mcp.LinkedWorkerSummary)
	if len(linkedWorkers) != 2 {
		t.Errorf("expected 2 linked_workers, got %v", m["linked_workers"])
	}

	// ClearSession must NOT have been called.
	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 0 {
		t.Errorf("ClearSession must not be called on confirmation prompt, got %v", clearer.cleared)
	}
}

func TestCallTool_ClearSession_DedupesWorkersAcrossEngines(t *testing.T) {
	s, db, _, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-dedupe", "session-D", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "W1"}))
	w1 := workerResult.(model.Worker)

	ss := store.NewSessionStore(db)
	ss.UpsertSessionContext(ctx, "session-D", w1.ID, "sid-claude", "claude") //nolint
	ss.UpsertSessionContext(ctx, "session-D", w1.ID, "sid-codex", "codex")   //nolint

	result, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-D",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["requires_confirmation"] == true {
		t.Fatalf("expected no confirmation for one worker across multiple engines, got %v", m)
	}
	if m["cleared"] != true {
		t.Fatalf("expected clear to proceed, got %v", m)
	}

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 1 || clearer.cleared[0] != "session-D" {
		t.Fatalf("expected clear invoked once for session-D, got %v", clearer.cleared)
	}
}

func TestCallTool_ClearSession_ForceTrue_SkipsConfirmation(t *testing.T) {
	s, db, _, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-force1", "session-F", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult1, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "W1"}))
	workerResult2, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "W2"}))
	w1 := workerResult1.(model.Worker)
	w2 := workerResult2.(model.Worker)

	ss := store.NewSessionStore(db)
	ss.UpsertSessionContext(ctx, "session-F", w1.ID, "sid-w1", "") //nolint
	ss.UpsertSessionContext(ctx, "session-F", w2.ID, "sid-w2", "") //nolint

	result, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-F",
		"force":       true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["cleared"] != true {
		t.Errorf("expected cleared=true, got %v", m["cleared"])
	}

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 1 || clearer.cleared[0] != "session-F" {
		t.Errorf("expected ClearSession(session-F), got %v", clearer.cleared)
	}
}

func TestCallTool_ClearSession_OneWorker_NoConfirmation(t *testing.T) {
	s, db, _, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-one1", "session-O", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	ss := store.NewSessionStore(db)
	ss.UpsertSessionContext(ctx, "session-O", w.ID, "sid-w", "") //nolint

	// Only 1 worker — should clear without confirmation.
	result, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-O",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["cleared"] != true {
		t.Errorf("expected cleared=true, got %v", m["cleared"])
	}

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 1 {
		t.Errorf("expected ClearSession called once, got %v", clearer.cleared)
	}
}

// --- clear_session task detection ---

func TestCallTool_ClearSession_RunningTaskRequiresConfirmation(t *testing.T) {
	s, db, _, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-rt1", "session-RT", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool(ctx, "create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	ts := store.NewTaskStore(db)
	ts.Create(ctx, model.Task{ //nolint
		MessageID: "msg-rt1", WorkerID: w.ID, Instruction: "long running task",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})

	result, err := s.CallTool(ctx, "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-RT",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["requires_confirmation"] != true {
		t.Errorf("expected requires_confirmation=true, got %v", m)
	}
	if m["reason"] != mcp.ClearReasonActiveTasks {
		t.Errorf("expected reason=running_tasks, got %v", m["reason"])
	}
	tasks, ok := m["running_tasks"].([]mcp.ActiveTaskSummary)
	if !ok || len(tasks) != 1 {
		t.Errorf("expected running_tasks with 1 entry, got %v", m["running_tasks"])
	} else {
		if tasks[0].Instruction != "long running task" {
			t.Errorf("expected instruction='long running task', got %v", tasks[0].Instruction)
		}
		if tasks[0].Status != model.TaskStatusRunning {
			t.Errorf("expected status=running, got %v", tasks[0].Status)
		}
	}

	// ClearSession must NOT have been called.
	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 0 {
		t.Errorf("ClearSession must not be called on confirmation prompt, got %v", clearer.cleared)
	}
}

func TestCallTool_ClearSession_PendingTaskRequiresConfirmation(t *testing.T) {
	s, db, _, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-pt1", "session-PT", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool(ctx, "create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	ts := store.NewTaskStore(db)
	ts.Create(ctx, model.Task{ //nolint
		MessageID: "msg-pt1", WorkerID: w.ID, Instruction: "queued task",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: 1, UpdatedAt: 1,
	})

	result, err := s.CallTool(ctx, "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-PT",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["requires_confirmation"] != true {
		t.Errorf("expected requires_confirmation=true for pending task, got %v", m)
	}
	if m["reason"] != mcp.ClearReasonActiveTasks {
		t.Errorf("expected reason=running_tasks, got %v", m["reason"])
	}

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 0 {
		t.Errorf("ClearSession must not be called on confirmation prompt")
	}
}

func TestCallTool_ClearSession_ForceSkipsTaskDetection(t *testing.T) {
	s, db, stopper, clearer := setupMCPServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-fsd1", "session-FSD", "feishu", "hi", `{}`, "", 0) //nolint

	workerResult, _ := s.CallTool(ctx, "create_worker", mustMarshal(t, map[string]any{"name": "W"}))
	w := workerResult.(model.Worker)

	ts := store.NewTaskStore(db)
	taskID, _ := ts.Create(ctx, model.Task{
		MessageID: "msg-fsd1", WorkerID: w.ID, Instruction: "long task",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})
	ts.SetExecution(ctx, taskID, "exec-fsd-1", model.TaskStatusRunning) //nolint

	result, err := s.CallTool(ctx, "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-FSD",
		"force":       true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result.(map[string]any)
	if m["cleared"] != true {
		t.Errorf("expected cleared=true with force=true, got %v", m)
	}

	stopper.mu.Lock()
	defer stopper.mu.Unlock()
	if len(stopper.stopped) != 1 || stopper.stopped[0] != "exec-fsd-1" {
		t.Errorf("expected StopExecution(exec-fsd-1), got %v", stopper.stopped)
	}

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	if len(clearer.cleared) != 1 || clearer.cleared[0] != "session-FSD" {
		t.Errorf("expected ClearSession(session-FSD), got %v", clearer.cleared)
	}
}

func TestCallTool_ClearSession_NonImmediateTaskDoesNotBlock(t *testing.T) {
	cases := []struct {
		taskType   string
		msgID      string
		sessionKey string
	}{
		{model.TaskTypeScheduled, "msg-sched1", "session-SCHED"},
		{model.TaskTypeCountdown, "msg-cd1", "session-CD"},
	}
	for _, tc := range cases {
		t.Run(tc.taskType, func(t *testing.T) {
			s, db, _, clearer := setupMCPServerWithClear(t)
			ctx := context.Background()

			ms := store.NewMessageStore(db)
			ms.Create(ctx, tc.msgID, tc.sessionKey, "feishu", "hi", `{}`, "", 0) //nolint

			workerResult, _ := s.CallTool(ctx, "create_worker", mustMarshal(t, map[string]any{"name": "W"}))
			w := workerResult.(model.Worker)

			ts := store.NewTaskStore(db)
			ts.Create(ctx, model.Task{ //nolint
				MessageID: tc.msgID, WorkerID: w.ID, Instruction: "task",
				Type: tc.taskType, Status: model.TaskStatusPending,
				CreatedAt: 1, UpdatedAt: 1,
			})

			result, err := s.CallTool(ctx, "clear_session", mustMarshal(t, map[string]any{
				"session_key": tc.sessionKey,
			}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			m := result.(map[string]any)
			if m["cleared"] != true {
				t.Errorf("%s pending task should not block clear, got %v", tc.taskType, m)
			}

			clearer.mu.Lock()
			defer clearer.mu.Unlock()
			if len(clearer.cleared) != 1 || clearer.cleared[0] != tc.sessionKey {
				t.Errorf("expected ClearSession(%s), got %v", tc.sessionKey, clearer.cleared)
			}
		})
	}
}

func TestResolveDepartmentID_ByID(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)

	dept, err := ds.Create(model.Department{Name: "Engineering"})
	if err != nil {
		t.Fatalf("create dept: %v", err)
	}

	result, err := s.CallTool(context.Background(), "get_department",
		mustMarshal(t, map[string]any{"id": dept.ID}))
	if err != nil {
		t.Fatalf("get_department by ID: %v", err)
	}
	got, ok := result.(model.Department)
	if !ok {
		t.Fatalf("expected model.Department, got %T", result)
	}
	if got.ID != dept.ID {
		t.Errorf("expected ID %s, got %s", dept.ID, got.ID)
	}
}

func TestResolveDepartmentID_ByName(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)

	_, err := ds.Create(model.Department{Name: "Marketing"})
	if err != nil {
		t.Fatalf("create dept: %v", err)
	}

	result, err := s.CallTool(context.Background(), "get_department",
		mustMarshal(t, map[string]any{"id": "Marketing"}))
	if err != nil {
		t.Fatalf("get_department by name: %v", err)
	}
	got, ok := result.(model.Department)
	if !ok {
		t.Fatalf("expected model.Department, got %T", result)
	}
	if got.Name != "Marketing" {
		t.Errorf("expected name Marketing, got %s", got.Name)
	}
}

func TestResolveDepartmentID_NotFound(t *testing.T) {
	s, _ := setupMCPServerWithSender(t, "feishu", &mockSender{})
	_, err := s.CallTool(context.Background(), "get_department",
		mustMarshal(t, map[string]any{"id": "nonexistent"}))
	if err == nil {
		t.Fatal("expected error for nonexistent department, got nil")
	}
}

func decodeDeptTree(t *testing.T, result any) []map[string]any {
	t.Helper()
	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal list_departments result: %v", err)
	}
	var tree []map[string]any
	if err := json.Unmarshal(b, &tree); err != nil {
		t.Fatalf("unmarshal list_departments result: %v", err)
	}
	return tree
}

func TestCallTool_ListDepartments_Empty(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "list_departments", mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatalf("list_departments: %v", err)
	}
	tree := decodeDeptTree(t, result)
	if len(tree) != 0 {
		t.Errorf("expected empty tree, got %d roots", len(tree))
	}
}

func TestCallTool_ListDepartments_Tree(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)

	parent, _ := ds.Create(model.Department{Name: "R&D"})
	_, _ = ds.Create(model.Department{Name: "Frontend", ParentID: &parent.ID})
	_, _ = ds.Create(model.Department{Name: "Backend", ParentID: &parent.ID})

	result, err := s.CallTool(context.Background(), "list_departments", mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatalf("list_departments: %v", err)
	}
	tree := decodeDeptTree(t, result)
	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}
	if tree[0]["name"].(string) != "R&D" {
		t.Errorf("expected root name R&D, got %s", tree[0]["name"])
	}
	children := tree[0]["children"].([]any)
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
	// Verify slim fields (no parent_id, created_at, updated_at)
	if _, hasParentID := tree[0]["parent_id"]; hasParentID {
		t.Error("expected no parent_id in department list response")
	}
	if _, hasCreatedAt := tree[0]["created_at"]; hasCreatedAt {
		t.Error("expected no created_at in department list response")
	}
}

func TestCallTool_CreateDepartment(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "create_department",
		mustMarshal(t, map[string]any{"name": "Engineering"}))
	if err != nil {
		t.Fatalf("create_department: %v", err)
	}
	dept, ok := result.(model.Department)
	if !ok {
		t.Fatalf("expected model.Department, got %T", result)
	}
	if dept.ID == "" {
		t.Error("expected non-empty ID")
	}
	if dept.Name != "Engineering" {
		t.Errorf("expected name Engineering, got %s", dept.Name)
	}
}

func TestCallTool_CreateDepartment_WithParentByName(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	parent, _ := ds.Create(model.Department{Name: "R&D"})

	result, err := s.CallTool(context.Background(), "create_department",
		mustMarshal(t, map[string]any{"name": "Frontend", "parent_id": "R&D"}))
	if err != nil {
		t.Fatalf("create_department with parent: %v", err)
	}
	child, ok := result.(model.Department)
	if !ok {
		t.Fatalf("expected model.Department, got %T", result)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Errorf("expected ParentID %s, got %v", parent.ID, child.ParentID)
	}
}

func TestCallTool_UpdateDepartment_Name(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	dept, _ := ds.Create(model.Department{Name: "OldName"})

	result, err := s.CallTool(context.Background(), "update_department",
		mustMarshal(t, map[string]any{"id": dept.ID, "name": "NewName"}))
	if err != nil {
		t.Fatalf("update_department: %v", err)
	}
	updated, ok := result.(model.Department)
	if !ok {
		t.Fatalf("expected model.Department, got %T", result)
	}
	if updated.Name != "NewName" {
		t.Errorf("expected name NewName, got %s", updated.Name)
	}
}

func TestCallTool_DeleteDepartment(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	dept, _ := ds.Create(model.Department{Name: "ToDelete"})

	_, err := s.CallTool(context.Background(), "delete_department",
		mustMarshal(t, map[string]any{"id": dept.ID}))
	if err != nil {
		t.Fatalf("delete_department: %v", err)
	}

	_, err = ds.GetByID(dept.ID)
	if err == nil {
		t.Error("expected error fetching deleted department, got nil")
	}
}

func TestCallTool_DeleteDepartment_FailsWithChildren(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	parent, _ := ds.Create(model.Department{Name: "Parent"})
	_, _ = ds.Create(model.Department{Name: "Child", ParentID: &parent.ID})

	_, err := s.CallTool(context.Background(), "delete_department",
		mustMarshal(t, map[string]any{"id": parent.ID}))
	if err == nil {
		t.Error("expected error deleting department with children, got nil")
	}
}

func TestCallTool_ListWorkers_FilterByDepartment(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	ws := store.NewWorkerStore(db)

	dept, _ := ds.Create(model.Department{Name: "Engineering"})
	other, _ := ds.Create(model.Department{Name: "Marketing"})

	w1, _ := ws.Create(model.Worker{Name: "Alice"})
	w2, _ := ws.Create(model.Worker{Name: "Bob"})
	_ = ds.SetWorkerDepartments(w1.ID, []string{dept.ID})
	_ = ds.SetWorkerDepartments(w2.ID, []string{other.ID})

	result, err := s.CallTool(context.Background(), "list_workers",
		mustMarshal(t, map[string]any{"department_id": dept.ID}))
	if err != nil {
		t.Fatalf("list_workers with dept filter: %v", err)
	}
	items, total := decodeListWorkersResult(t, result)
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0].(map[string]any)
	if item["name"].(string) != "Alice" {
		t.Errorf("expected Alice, got %s", item["name"])
	}
}

func TestCallTool_ListWorkers_FilterByDepartment_Recursive(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	ws := store.NewWorkerStore(db)

	parent, _ := ds.Create(model.Department{Name: "R&D"})
	child, _ := ds.Create(model.Department{Name: "Frontend", ParentID: &parent.ID})

	w1, _ := ws.Create(model.Worker{Name: "Alice"})
	w2, _ := ws.Create(model.Worker{Name: "Bob"})
	_ = ds.SetWorkerDepartments(w1.ID, []string{parent.ID})
	_ = ds.SetWorkerDepartments(w2.ID, []string{child.ID})

	// recursive (default): should return both
	result, err := s.CallTool(context.Background(), "list_workers",
		mustMarshal(t, map[string]any{"department_id": parent.ID}))
	if err != nil {
		t.Fatalf("list_workers recursive: %v", err)
	}
	items, _ := decodeListWorkersResult(t, result)
	if len(items) != 2 {
		t.Errorf("expected 2 workers (recursive), got %d", len(items))
	}

	// non-recursive: should return only Alice
	result2, err := s.CallTool(context.Background(), "list_workers",
		mustMarshal(t, map[string]any{"department_id": parent.ID, "recursive": false}))
	if err != nil {
		t.Fatalf("list_workers non-recursive: %v", err)
	}
	items2, _ := decodeListWorkersResult(t, result2)
	if len(items2) != 1 {
		t.Errorf("expected 1 worker (non-recursive), got %d", len(items2))
	}
	item2 := items2[0].(map[string]any)
	if item2["name"].(string) != "Alice" {
		t.Errorf("expected Alice, got %s", item2["name"])
	}
}

func TestCallTool_CreateWorker_WithDepartment(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	dept, _ := ds.Create(model.Department{Name: "Engineering"})

	result, err := s.CallTool(context.Background(), "create_worker",
		mustMarshal(t, map[string]any{"name": "Alice", "department_ids": dept.ID}))
	if err != nil {
		t.Fatalf("create_worker with dept: %v", err)
	}
	w, ok := result.(model.Worker)
	if !ok {
		t.Fatalf("expected model.Worker, got %T", result)
	}

	depts, err := ds.GetWorkerDepartments(w.ID)
	if err != nil {
		t.Fatalf("GetWorkerDepartments: %v", err)
	}
	if len(depts) != 1 || depts[0].ID != dept.ID {
		t.Errorf("expected worker in Engineering dept, got %v", depts)
	}
}

func TestCallTool_UpdateWorker_SetDepartments(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	ws := store.NewWorkerStore(db)

	dept1, _ := ds.Create(model.Department{Name: "Engineering"})
	dept2, _ := ds.Create(model.Department{Name: "Design"})
	w, _ := ws.Create(model.Worker{Name: "Alice"})
	_ = ds.SetWorkerDepartments(w.ID, []string{dept1.ID})

	_, err := s.CallTool(context.Background(), "update_worker",
		mustMarshal(t, map[string]any{"worker_id": w.ID, "department_ids": dept2.Name}))
	if err != nil {
		t.Fatalf("update_worker set dept: %v", err)
	}

	depts, _ := ds.GetWorkerDepartments(w.ID)
	if len(depts) != 1 || depts[0].ID != dept2.ID {
		t.Errorf("expected worker in Design dept only, got %v", depts)
	}
}

func TestCallTool_UpdateWorker_ClearDepartments(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	ws := store.NewWorkerStore(db)

	dept, _ := ds.Create(model.Department{Name: "Engineering"})
	w, _ := ws.Create(model.Worker{Name: "Alice"})
	_ = ds.SetWorkerDepartments(w.ID, []string{dept.ID})

	_, err := s.CallTool(context.Background(), "update_worker",
		mustMarshal(t, map[string]any{"worker_id": w.ID, "department_ids": ""}))
	if err != nil {
		t.Fatalf("update_worker clear depts: %v", err)
	}

	depts, _ := ds.GetWorkerDepartments(w.ID)
	if len(depts) != 0 {
		t.Errorf("expected 0 departments after clear, got %d", len(depts))
	}
}

func workerCtx(workerID string, scopes []string) context.Context {
	ctx := context.WithValue(context.Background(), mcp.CtxWorkerIDKey, workerID)
	return context.WithValue(ctx, mcp.CtxScopesKey, scopes)
}

func TestCheckWorkerScope_WorkerWithScope_CanCallScopedTool(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	ctx := workerCtx("wid-1", []string{"read:workers"})
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCheckWorkerScope_WorkerWithoutScope_CannotCallScopedTool(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	ctx := workerCtx("wid-1", nil) // no scopes
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected permission denied error, got nil")
	}
}

func TestCheckWorkerScope_WorkerWithWrongScope_CannotCallScopedTool(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	ctx := workerCtx("wid-1", []string{"read:tasks"}) // has tasks scope, not workers
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	if err == nil {
		t.Error("expected permission denied error, got nil")
	}
}

func TestCheckWorkerScope_BeeToken_AlwaysAllowed(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	// Bee token: no workerID in context
	ctx := context.Background()
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Errorf("bee token should always be allowed, got: %v", err)
	}
}

func TestCheckWorkerScope_WorkerToken_NonScopedTool_Unchanged(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	ctx := workerCtx("wid-1", nil) // no scopes
	// send_message has no scope requirement — existing behavior, worker can call it
	_, err := s.CallTool(ctx, utils.SendMessage, mustMarshal(t, map[string]any{
		"message_id": "nonexistent",
		"content":    "test",
	}))
	// Should NOT be a permission denied error
	if err != nil && err.Error() == "permission denied: scope read:workers required" {
		t.Error("non-scoped tool should not return permission denied")
	}
}

func TestCallTool_ListOutboundMessages(t *testing.T) {
	s, db := setupMCPServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()

	oms := store.NewOutboundMessageStore(db)
	if err := oms.Create(ctx, store.OutboundMessage{
		ID: "out-1", SessionKey: "sk1", Platform: "feishu",
		Content: "reply", Status: store.OutboundStatusSent,
		SourceType: store.SourceTypeWorker, SourceID: "worker-X",
		SentAt: 1000,
	}); err != nil {
		t.Fatalf("seed outbound: %v", err)
	}
	if err := oms.Create(ctx, store.OutboundMessage{
		ID: "out-2", SessionKey: "sk2", Platform: "local",
		Content: "hi", Status: store.OutboundStatusFailed,
		SourceType: store.SourceTypeBee,
		SentAt: 2000,
	}); err != nil {
		t.Fatalf("seed outbound: %v", err)
	}

	// No filter — returns all
	result, err := s.CallTool(ctx, "list_outbound_messages", mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	m := decodeResult(t, result)
	if m["total"].(float64) != 2 {
		t.Errorf("total: want 2, got %v", m["total"])
	}

	// Filter by source_type=worker
	result2, err := s.CallTool(ctx, "list_outbound_messages", mustMarshal(t, map[string]any{
		"source_type": store.SourceTypeWorker,
	}))
	if err != nil {
		t.Fatalf("CallTool filter: %v", err)
	}
	m2 := decodeResult(t, result2)
	if m2["total"].(float64) != 1 {
		t.Errorf("filtered total: want 1, got %v", m2["total"])
	}
}

func TestCallTool_CreateWorker_WithEngine(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{
		"name":   "EngineBot",
		"engine": "claude",
	}))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	w, ok := result.(model.Worker)
	if !ok {
		t.Fatalf("expected model.Worker, got %T", result)
	}
	if w.Engine != "claude" {
		t.Errorf("expected engine claude, got %q", w.Engine)
	}
}

func TestCallTool_CreateWorker_InvalidEngine(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	_, err := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{
		"name":   "EngineBot",
		"engine": "not-a-real-engine",
	}))
	if err == nil {
		t.Error("expected error for unknown engine, got nil")
	}
}

func TestCallTool_UpdateWorker_WithEngine(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	created, err := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "Bot"}))
	if err != nil {
		t.Fatalf("create_worker: %v", err)
	}
	w := created.(model.Worker)

	result, err := s.CallTool(context.Background(), "update_worker", mustMarshal(t, map[string]any{
		"worker_id": w.ID,
		"engine":    "claude",
	}))
	if err != nil {
		t.Fatalf("update_worker: %v", err)
	}
	updated, ok := result.(model.Worker)
	if !ok {
		t.Fatalf("expected model.Worker, got %T", result)
	}
	if updated.Engine != "claude" {
		t.Errorf("expected engine claude, got %q", updated.Engine)
	}
}

func TestCallTool_UpdateWorker_InvalidEngine(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	created, err := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{"name": "Bot"}))
	if err != nil {
		t.Fatalf("create_worker: %v", err)
	}
	w := created.(model.Worker)

	_, err = s.CallTool(context.Background(), "update_worker", mustMarshal(t, map[string]any{
		"worker_id": w.ID,
		"engine":    "not-a-real-engine",
	}))
	if err == nil {
		t.Error("expected error for unknown engine, got nil")
	}
}

func TestCallTool_UpdateWorker_ClearEngine(t *testing.T) {
	s := setupMCPServerWithMessaging(t)
	created, err := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{
		"name":   "Bot",
		"engine": "claude",
	}))
	if err != nil {
		t.Fatalf("create_worker: %v", err)
	}
	w := created.(model.Worker)

	result, err := s.CallTool(context.Background(), "update_worker", mustMarshal(t, map[string]any{
		"worker_id": w.ID,
		"engine":    "",
	}))
	if err != nil {
		t.Fatalf("update_worker: %v", err)
	}
	updated := result.(model.Worker)
	if updated.Engine != "" {
		t.Errorf("expected engine cleared, got %q", updated.Engine)
	}
}
