package rpc_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/domain/worker"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/infra/utils"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/rpc"
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
func (s *stubEngineAdapter) CollectTokenUsage(_ context.Context, _ string) ([]ai.TokenUsage, error) {
	return nil, ai.ErrSessionDataNotFound
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

type mockClearDispatcher struct {
	mu             sync.Mutex
	cleared        []string
	clearedWorkers []string // sessionKey + "::" + workerID
}

func (m *mockClearDispatcher) ClearSession(sessionKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleared = append(m.cleared, sessionKey)
}

func (m *mockClearDispatcher) ClearWorker(sessionKey, workerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clearedWorkers = append(m.clearedWorkers, sessionKey+"::"+workerID)
}

// newTestServer builds a Bee RPC server on a fresh SQLite DB with the given platform senders.
func newTestServer(t *testing.T, senders map[string]platform.PlatformSenderAdapter) (*rpc.Server, *sql.DB, *mockExecStopper, *mockClearDispatcher) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	if senders == nil {
		senders = map[string]platform.PlatformSenderAdapter{}
	}

	ws := store.NewWorkerStore(db)
	es := store.NewExecutionStore(db, t.TempDir())
	ts := store.NewTaskStore(db)
	ss := store.NewSessionStore(db)
	engineCfg := enginecfg.NewStore("claude")
	mgr := worker.NewManager(
		t.TempDir(),
		config.BeeConfig{Engines: config.EnginesConfig{Claude: config.EngineItemConfig{Path: "claude"}}},
		ws, es,
		map[string]ai.EngineAdapter{"claude": &stubEngineAdapter{}}, engineCfg, nil, nil,
	)
	stopper := &mockExecStopper{}
	disp := &mockClearDispatcher{}
	clearSvc := session.NewClearService(session.ClearServiceDeps{
		Sessions:      ss,
		Tasks:         ts,
		ExecStopper:   stopper,
		ExecFinalizer: es,
		Dispatcher:    disp,
		RunningExecs:  es,
		EngineCfg:     engineCfg,
	})
	srv := rpc.NewBeeServer(ws, mgr, ts, store.NewMessageStore(db), store.NewOutboundMessageStore(db), senders, clearSvc, es, store.NewConstraintStore(db), ss, store.NewDepartmentStore(db))
	return srv, db, stopper, disp
}

func setupServerWithMessaging(t *testing.T) *rpc.Server {
	s, _, _, _ := newTestServer(t, nil)
	return s
}

func setupServerWithSender(t *testing.T, senderID string, sender platform.PlatformSenderAdapter) (*rpc.Server, *sql.DB) {
	s, db, _, _ := newTestServer(t, map[string]platform.PlatformSenderAdapter{senderID: sender})
	return s, db
}

func setupServerWithClear(t *testing.T) (*rpc.Server, *sql.DB, *mockExecStopper, *mockClearDispatcher) {
	return newTestServer(t, nil)
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func decodeResult(t *testing.T, result any) map[string]any {
	t.Helper()
	b, err := json.Marshal(result)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

func decodeListWorkersResult(t *testing.T, result any) (items []any, total int) {
	t.Helper()
	b, err := json.Marshal(result)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	items = m["items"].([]any)
	total = int(m["total"].(float64))
	return
}

// mustCreateWorker creates a worker through the create_worker tool and returns it.
func mustCreateWorker(t *testing.T, s *rpc.Server, args map[string]any) model.Worker {
	t.Helper()
	res, err := s.CallTool(context.Background(), "create_worker", mustMarshal(t, args))
	require.NoError(t, err)
	w, ok := res.(model.Worker)
	require.True(t, ok, "create_worker returned %T", res)
	return w
}

func TestCallTool_ListWorkers_Empty(t *testing.T) {
	s := setupServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "list_workers", mustMarshal(t, map[string]any{}))
	require.NoError(t, err)
	items, total := decodeListWorkersResult(t, result)
	assert.NotNil(t, items, "items must be an empty slice, not null")
	assert.Empty(t, items)
	assert.Equal(t, 0, total)
}

func TestCallTool_CreateWorker(t *testing.T) {
	s := setupServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{
		"name":        "TestBot",
		"description": "A test bot",
		"prompt":      "You are a test bot.",
	}))
	require.NoError(t, err)
	w, ok := result.(model.Worker)
	require.True(t, ok, "create_worker returned %T", result)
	assert.NotEmpty(t, w.ID)
	assert.Equal(t, "TestBot", w.Name)
}

func TestCallTool_GetWorker(t *testing.T) {
	s := setupServerWithMessaging(t)
	w := mustCreateWorker(t, s, map[string]any{"name": "Bot"})

	result, err := s.CallTool(context.Background(), "get_worker", mustMarshal(t, map[string]any{"worker_id": w.ID}))
	require.NoError(t, err)
	fetched := decodeResult(t, result)
	assert.Equal(t, w.ID, fetched["id"].(string))
	_, ok := fetched["departments"]
	assert.True(t, ok, "expected departments field in get_worker response")
}

func TestCallTool_UpdateWorker(t *testing.T) {
	s := setupServerWithMessaging(t)
	w := mustCreateWorker(t, s, map[string]any{"name": "OldName"})

	result, err := s.CallTool(context.Background(), "update_worker", mustMarshal(t, map[string]any{
		"worker_id":   w.ID,
		"name":        "NewName",
		"constraints": "New constraints",
	}))
	require.NoError(t, err)
	updated := result.(model.Worker)
	assert.Equal(t, "NewName", updated.Name)
	assert.Equal(t, "New constraints", updated.Constraints)
	assert.Equal(t, w.Description, updated.Description)
}

func TestCallTool_DeleteWorker(t *testing.T) {
	s := setupServerWithMessaging(t)
	w := mustCreateWorker(t, s, map[string]any{"name": "Bot"})

	_, err := s.CallTool(context.Background(), "delete_worker", mustMarshal(t, map[string]any{"worker_id": w.ID}))
	require.NoError(t, err)

	_, err = s.CallTool(context.Background(), "get_worker", mustMarshal(t, map[string]any{"worker_id": w.ID}))
	assert.Error(t, err)
}

// TestCallTool_InvalidArgs covers tool calls that must fail validation; each
// case only asserts that an error is returned.
func TestCallTool_InvalidArgs(t *testing.T) {
	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"GetWorker_NotFound", "get_worker", map[string]any{"worker_id": "nonexistent"}},
		{"UnknownTool", "nonexistent_tool", map[string]any{}},
		{"SendMessage_MissingMessageID", "send_message", map[string]any{"content": "hello"}},
		{"SendMessage_MissingContent", "send_message", map[string]any{"message_id": "msg-x"}},
		{"SendMessage_MessageNotFound", "send_message", map[string]any{"message_id": "nonexistent-msg", "content": "hello"}},
		{"ListTasks_BothParams_Error", "list_tasks", map[string]any{"message_id": "msg-1", "session_key": "session-X"}},
		{"ListTasks_NoParams_Error", "list_tasks", map[string]any{}},
		{"ListSessionContexts_MissingSessionKey", "list_session_contexts", map[string]any{}},
		{"ClearWorkerSession_MissingSessionKey", "clear_worker_session", map[string]any{"worker_id": "some-worker"}},
		{"ClearWorkerSession_MissingWorkerID", "clear_worker_session", map[string]any{"session_key": "sk"}},
		{"ClearWorkerSession_RefusesBee", "clear_worker_session", map[string]any{"session_key": "sk", "worker_id": "bee"}},
		{"CreateWorker_InvalidEngine", "create_worker", map[string]any{"name": "EngineBot", "engine": "not-a-real-engine"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := setupServerWithMessaging(t)
			_, err := s.CallTool(context.Background(), tc.tool, mustMarshal(t, tc.args))
			assert.Error(t, err)
		})
	}
}

// --- send_message ---

func TestCallTool_SendMessage(t *testing.T) {
	cases := []struct {
		name     string
		senderID string
		setup    func(t *testing.T, db *sql.DB) (context.Context, map[string]any)
		check    func(t *testing.T, sent []platform.OutboundMessage)
	}{
		{
			name:     "CallsSender",
			senderID: "feishu",
			setup: func(t *testing.T, db *sql.DB) (context.Context, map[string]any) {
				ctx := context.Background()
				ms := store.NewMessageStore(db)
				ms.Create(ctx, "msg-send-1", "feishu:chat1:userA", "feishu", "hello", //nolint
					`{"event":{"message":{"chat_id":"c1","chat_type":"p2p","message_id":"m1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`, "", 0)
				return ctx, map[string]any{"message_id": "msg-send-1", "content": "Task done!"}
			},
			check: func(t *testing.T, sent []platform.OutboundMessage) {
				require.Len(t, sent, 1)
				assert.Equal(t, "Task done!", sent[0].Content)
			},
		},
		{
			name:     "WorkerPrefixesContent",
			senderID: "feishu",
			setup: func(t *testing.T, db *sql.DB) (context.Context, map[string]any) {
				ctx := context.Background()
				// Create a worker and a message.
				ws := store.NewWorkerStore(db)
				w, err := ws.Create(model.Worker{Name: "MaoMao", Description: "test worker"})
				require.NoError(t, err)
				ms := store.NewMessageStore(db)
				ms.Create(ctx, "msg-worker-prefix", "feishu:chat1:userA", "feishu", "hello", //nolint
					`{"event":{"message":{"chat_id":"c1","chat_type":"p2p","message_id":"m1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`, "", 0)
				// Call with a context that carries the worker's ID.
				workerCtx := context.WithValue(ctx, rpc.CtxWorkerIDKey, w.ID)
				return workerCtx, map[string]any{"message_id": "msg-worker-prefix", "content": "task done"}
			},
			check: func(t *testing.T, sent []platform.OutboundMessage) {
				require.Len(t, sent, 1)
				assert.Equal(t, "MaoMao\ntask done", sent[0].Content)
			},
		},
		{
			name:     "WorkerDeletedFallsBackToWorkerID",
			senderID: "feishu",
			setup: func(t *testing.T, db *sql.DB) (context.Context, map[string]any) {
				ctx := context.Background()
				ms := store.NewMessageStore(db)
				ms.Create(ctx, "msg-deleted-worker", "feishu:chat1:userA", "feishu", "hello", //nolint
					`{"event":{"message":{"chat_id":"c1","chat_type":"p2p","message_id":"m1","message_type":"text","content":"{\"text\":\"hi\"}"}}}`, "", 0)
				// Use a worker ID that does not exist in the store.
				workerCtx := context.WithValue(ctx, rpc.CtxWorkerIDKey, "worker-deleted-xyz")
				return workerCtx, map[string]any{"message_id": "msg-deleted-worker", "content": "task done"}
			},
			check: func(t *testing.T, sent []platform.OutboundMessage) {
				require.Len(t, sent, 1)
				assert.Equal(t, "worker-deleted-xyz\ntask done", sent[0].Content)
			},
		},
		{
			name:     "LinearContentAndMediaSentTogether",
			senderID: "linear",
			setup: func(t *testing.T, db *sql.DB) (context.Context, map[string]any) {
				ctx := context.Background()
				ms := store.NewMessageStore(db)
				ms.Create(ctx, "msg-linear-media", "linear:ENG:ENG-42", "linear", "hello", `{"issue_id":"I1"}`, "", 0) //nolint
				return ctx, map[string]any{"message_id": "msg-linear-media", "content": "see attached", "media_path": "/tmp/snap.png"}
			},
			check: func(t *testing.T, sent []platform.OutboundMessage) {
				require.Len(t, sent, 1, "expected one combined send for Linear")
				assert.Equal(t, "see attached", sent[0].Content)
				assert.Equal(t, "/tmp/snap.png", sent[0].MediaPath)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockSender{}
			s, db := setupServerWithSender(t, tc.senderID, mock)
			ctx, args := tc.setup(t, db)

			result, err := s.CallTool(ctx, "send_message", mustMarshal(t, args))
			require.NoError(t, err)
			m := result.(map[string]string)
			assert.Equal(t, "sent", m["status"])

			mock.mu.Lock()
			sent := append([]platform.OutboundMessage{}, mock.sent...)
			mock.mu.Unlock()
			tc.check(t, sent)
		})
	}
}

func TestCallTool_SendMessage_UnknownPlatform(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-unk", "dingtalk:c1:u1", "dingtalk", "hi", `{}`, "", 0) //nolint

	_, err := s.CallTool(context.Background(), "send_message", mustMarshal(t, map[string]any{
		"message_id": "msg-unk",
		"content":    "hello",
	}))
	assert.Error(t, err)
}

// --- list_tasks session_key tests ---

func TestCallTool_ListTasks_BySessionKey(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()
	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-sk1", "session-X", "feishu", "hi", `{}`, "", 0) //nolint

	w := mustCreateWorker(t, s, map[string]any{"name": "W"})

	s.CallTool(context.Background(), "create_task", mustMarshal(t, map[string]any{
		"message_id": "msg-sk1", "worker_id": w.ID,
		"instruction": "task1", "type": "immediate",
	}))

	result, err := s.CallTool(context.Background(), "list_tasks", mustMarshal(t, map[string]any{
		"session_key": "session-X",
	}))
	require.NoError(t, err)
	tasks, _ := decodePagedTaskItems(t, result)
	assert.Len(t, tasks, 1)
}

func TestCallTool_ClearSession_EmptySession_ReturnsNoContext(t *testing.T) {
	s, _, _, clearer := setupServerWithClear(t)
	result, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-empty",
	}))
	require.NoError(t, err)
	m := result.(map[string]any)
	assert.Equal(t, false, m["cleared"])
	assert.Equal(t, rpc.ClearReasonNoContext, m["reason"])
	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	assert.Empty(t, clearer.cleared, "ClearSession must not be called when session is empty")
}

func TestCallTool_ClearSession_CancelsAndStopsTasks(t *testing.T) {
	s, db, stopper, clearer := setupServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-c1", "session-Y", "feishu", "hi", `{}`, "", 0) //nolint

	w := mustCreateWorker(t, s, map[string]any{"name": "W"})

	// Create a running task with a running execution in bee_executions.
	ts := store.NewTaskStore(db)
	id, _ := ts.Create(ctx, model.Task{
		MessageID: "msg-c1", WorkerID: w.ID, Instruction: "long task",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})
	// Insert a corresponding execution row so GetRunningByTaskID can find it.
	db.ExecContext(ctx, `INSERT INTO bee_executions (id, task_id, worker_id, session_id, engine, trigger_input, status, result, ai_process_pid, started_at) VALUES (?, ?, ?, '', '', '', ?, '', 0, 1)`, "exec-running-1", id, w.ID, model.ExecStatusRunning) //nolint

	// Create a pending task.
	ts.Create(ctx, model.Task{
		MessageID: "msg-c1", WorkerID: w.ID, Instruction: "queued task",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		CreatedAt: 1, UpdatedAt: 1,
	})

	result, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-Y",
		"force":       true,
	}))
	require.NoError(t, err)
	m := result.(map[string]any)
	cancelled, ok := m["cancelled_tasks"].(int64)
	assert.True(t, ok)
	assert.GreaterOrEqual(t, cancelled, int64(1))

	// StopExecution should have been called for the running task.
	// guard: length check protects the stopper.stopped[0] index below.
	stopper.mu.Lock()
	defer stopper.mu.Unlock()
	require.Len(t, stopper.stopped, 1)
	assert.Equal(t, "exec-running-1", stopper.stopped[0])

	// ClearSession should have been called.
	// guard: length check protects the clearer.cleared[0] index below.
	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	require.Len(t, clearer.cleared, 1)
	assert.Equal(t, "session-Y", clearer.cleared[0])
}

func TestCallTool_ClearSession_MissingSessionKey(t *testing.T) {
	s, _, _, _ := setupServerWithClear(t)
	_, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{}))
	assert.Error(t, err)
}

func TestCallTool_GetWorkerStatus(t *testing.T) {
	s := setupServerWithMessaging(t)
	w := mustCreateWorker(t, s, map[string]any{"name": "status-test"})

	result, err := s.CallTool(context.Background(), utils.GetWorkerStatus, mustMarshal(t, map[string]any{
		"worker_id": w.ID,
	}))
	require.NoError(t, err)
	m := result.(map[string]any)
	assert.Equal(t, w.ID, m["worker_id"])
	assert.Equal(t, "idle", m["status"])
}

func TestCallTool_GetSystemOverview(t *testing.T) {
	s := setupServerWithMessaging(t)

	result, err := s.CallTool(context.Background(), utils.GetSystemOverview, nil)
	require.NoError(t, err)
	m := result.(map[string]any)
	assert.NotNil(t, m["workers"])
	assert.NotNil(t, m["tasks"])
}

func TestCallTool_SaveConstraint(t *testing.T) {
	s := setupServerWithMessaging(t)

	result, err := s.CallTool(context.Background(), utils.SaveConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "test_pref",
		"value": "user likes concise replies",
	}))
	require.NoError(t, err)
	m := result.(map[string]string)
	assert.Equal(t, "saved", m["status"])
}

func TestCallTool_GetConstraint(t *testing.T) {
	s := setupServerWithMessaging(t)

	// Save first.
	s.CallTool(context.Background(), utils.SaveConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "pref1",
		"value": "value1",
	}))

	// Get by key.
	result, err := s.CallTool(context.Background(), utils.GetConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "pref1",
	}))
	require.NoError(t, err)
	require.NotNil(t, result)

	// List by scope (no key).
	_, err = s.CallTool(context.Background(), utils.GetConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
	}))
	require.NoError(t, err)
}

func TestCallTool_DeleteConstraint(t *testing.T) {
	s := setupServerWithMessaging(t)

	s.CallTool(context.Background(), utils.SaveConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "to_delete",
		"value": "temp",
	}))

	result, err := s.CallTool(context.Background(), utils.DeleteConstraint, mustMarshal(t, map[string]any{
		"scope": "global",
		"key":   "to_delete",
	}))
	require.NoError(t, err)
	m := result.(map[string]string)
	assert.Equal(t, "deleted", m["status"])
}

// --- list_session_contexts ---

func TestCallTool_ListSessionContexts_Empty(t *testing.T) {
	s := setupServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "list_session_contexts", mustMarshal(t, map[string]any{
		"session_key": "no-such-session",
	}))
	require.NoError(t, err)
	agents, ok := result.([]store.SessionAgent)
	require.True(t, ok, "expected []store.SessionAgent, got %T", result)
	assert.Empty(t, agents)
}

// --- clear_worker_session ---

func TestCallTool_ClearWorkerSession_Idempotent(t *testing.T) {
	s := setupServerWithMessaging(t)
	// No session row exists; should succeed without error.
	result, err := s.CallTool(context.Background(), "clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "sk",
		"worker_id":   "nonexistent-worker-id",
	}))
	require.NoError(t, err)
	m := result.(map[string]any)
	assert.Equal(t, true, m["cleared"])
}

func TestCallTool_ClearWorkerSession_RunningTask_RequiresConfirmation(t *testing.T) {
	s, db, _, disp := setupServerWithClear(t)
	ctx := context.Background()

	w := mustCreateWorker(t, s, map[string]any{"name": "W"})

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-cw1", "session-CW", "feishu", "hi", `{}`, "", 0) //nolint

	ts := store.NewTaskStore(db)
	ts.Create(ctx, model.Task{ //nolint
		MessageID: "msg-cw1", WorkerID: w.ID, Instruction: "long running",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})

	result, err := s.CallTool(ctx, "clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "session-CW",
		"worker_id":   w.ID,
	}))
	require.NoError(t, err)
	m := result.(map[string]any)
	assert.Equal(t, true, m["requires_confirmation"])
	assert.Equal(t, rpc.ClearReasonActiveTasks, m["reason"])
	disp.mu.Lock()
	defer disp.mu.Unlock()
	assert.Empty(t, disp.clearedWorkers, "ClearWorker must not be called on confirmation prompt")
}

func TestCallTool_ClearWorkerSession_Force_CancelsAndStops(t *testing.T) {
	s, db, stopper, disp := setupServerWithClear(t)
	ctx := context.Background()

	w := mustCreateWorker(t, s, map[string]any{"name": "W"})

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-cw2", "session-CW2", "feishu", "hi", `{}`, "", 0) //nolint

	ts := store.NewTaskStore(db)
	taskID, _ := ts.Create(ctx, model.Task{
		MessageID: "msg-cw2", WorkerID: w.ID, Instruction: "long running",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})
	db.ExecContext(ctx, `INSERT INTO bee_executions (id, task_id, worker_id, session_id, engine, trigger_input, status, result, ai_process_pid, started_at) VALUES (?, ?, ?, '', '', '', ?, '', 0, 1)`, "exec-cw-1", taskID, w.ID, model.ExecStatusRunning) //nolint

	result, err := s.CallTool(ctx, "clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "session-CW2",
		"worker_id":   w.ID,
		"force":       true,
	}))
	require.NoError(t, err)
	m := result.(map[string]any)
	assert.Equal(t, true, m["cleared"])
	cancelled, _ := m["cancelled_tasks"].(int64)
	assert.GreaterOrEqual(t, cancelled, int64(1))

	// guard: length check protects the stopper.stopped[0] index below.
	stopper.mu.Lock()
	defer stopper.mu.Unlock()
	require.Len(t, stopper.stopped, 1)
	assert.Equal(t, "exec-cw-1", stopper.stopped[0])

	// guard: length check protects the disp.clearedWorkers[0] index below.
	disp.mu.Lock()
	defer disp.mu.Unlock()
	want := "session-CW2::" + w.ID
	require.Len(t, disp.clearedWorkers, 1)
	assert.Equal(t, want, disp.clearedWorkers[0])
}

func TestCallTool_ClearWorkerSession_ClearsOnlyTargetWorker(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()

	w1 := mustCreateWorker(t, s, map[string]any{"name": "W1"})
	w2 := mustCreateWorker(t, s, map[string]any{"name": "W2"})

	// Seed session contexts for both workers.
	ss := store.NewSessionStore(db)
	ss.UpsertSessionContext(ctx, "sk", w1.ID, "sid-w1-claude", "claude") //nolint
	ss.UpsertSessionContext(ctx, "sk", w1.ID, "sid-w1-codex", "codex")   //nolint
	ss.UpsertSessionContext(ctx, "sk", w2.ID, "sid-w2", "claude")        //nolint

	// Clear only w1.
	result, err := s.CallTool(context.Background(), "clear_worker_session", mustMarshal(t, map[string]any{
		"session_key": "sk",
		"worker_id":   w1.ID,
	}))
	require.NoError(t, err)
	m := result.(map[string]any)
	assert.Equal(t, true, m["cleared"])
	assert.Equal(t, "W1", m["worker_name"])

	// w1 context should be gone only on the active engine (claude); its codex
	// row stays untouched, matching /clear's active-engine semantics. w2 is
	// untouched entirely.
	w1Claude, _ := ss.GetSessionContextForEngine(ctx, "sk", w1.ID, "claude")
	w1Codex, _ := ss.GetSessionContextForEngine(ctx, "sk", w1.ID, "codex")
	w2sid, _ := ss.GetSessionContextForEngine(ctx, "sk", w2.ID, "claude")
	assert.Equal(t, "", w1Claude)
	assert.Equal(t, "sid-w1-codex", w1Codex, "expected w1 codex context intact (active-engine scope)")
	assert.Equal(t, "sid-w2", w2sid)
}

// --- clear_session confirmation ---

func TestCallTool_ClearSession_Confirmation(t *testing.T) {
	const sessionKey = "sk"
	cases := []struct {
		name           string
		workers        []string
		force          bool
		wantCleared    bool
		wantClearCalls []string
		check          func(t *testing.T, m map[string]any)
	}{
		{
			name:           "NoActiveTasks",
			workers:        []string{"W"},
			wantCleared:    true,
			wantClearCalls: []string{sessionKey},
		},
		{
			name:    "RequiresConfirmation_TwoWorkers",
			workers: []string{"W1", "W2"},
			check: func(t *testing.T, m map[string]any) {
				assert.Equal(t, true, m["requires_confirmation"])
				workerCount, _ := m["worker_count"].(int)
				assert.Equal(t, 2, workerCount)
				linkedWorkers, _ := m["linked_workers"].([]rpc.LinkedWorkerSummary)
				assert.Len(t, linkedWorkers, 2)
			},
		},
		{
			name:           "ForceTrue_SkipsConfirmation",
			workers:        []string{"W1", "W2"},
			force:          true,
			wantCleared:    true,
			wantClearCalls: []string{sessionKey},
		},
		{
			name:           "OneWorker_NoConfirmation",
			workers:        []string{"W"},
			wantCleared:    true,
			wantClearCalls: []string{sessionKey},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, db, _, clearer := setupServerWithClear(t)
			ctx := context.Background()

			ms := store.NewMessageStore(db)
			ms.Create(ctx, "msg-1", sessionKey, "feishu", "hi", `{}`, "", 0) //nolint

			ss := store.NewSessionStore(db)
			for i, name := range tc.workers {
				w := mustCreateWorker(t, s, map[string]any{"name": name})
				ss.UpsertSessionContext(ctx, sessionKey, w.ID, fmt.Sprintf("sid-w%d", i), "") //nolint
			}

			args := map[string]any{"session_key": sessionKey}
			if tc.force {
				args["force"] = true
			}
			result, err := s.CallTool(ctx, "clear_session", mustMarshal(t, args))
			require.NoError(t, err)
			m := result.(map[string]any)
			if tc.wantCleared {
				assert.Equal(t, true, m["cleared"])
			}
			if tc.check != nil {
				tc.check(t, m)
			}

			clearer.mu.Lock()
			defer clearer.mu.Unlock()
			assert.Equal(t, tc.wantClearCalls, clearer.cleared)
		})
	}
}

func TestCallTool_ClearSession_DedupesWorkersAcrossEngines(t *testing.T) {
	s, db, _, clearer := setupServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-dedupe", "session-D", "feishu", "hi", `{}`, "", 0) //nolint

	w1 := mustCreateWorker(t, s, map[string]any{"name": "W1"})

	ss := store.NewSessionStore(db)
	ss.UpsertSessionContext(ctx, "session-D", w1.ID, "sid-claude", "claude") //nolint
	ss.UpsertSessionContext(ctx, "session-D", w1.ID, "sid-codex", "codex")   //nolint

	result, err := s.CallTool(context.Background(), "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-D",
	}))
	require.NoError(t, err)
	m := result.(map[string]any)
	require.NotEqual(t, true, m["requires_confirmation"], "expected no confirmation for one worker across multiple engines")
	require.Equal(t, true, m["cleared"])

	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	require.Len(t, clearer.cleared, 1)
	assert.Equal(t, "session-D", clearer.cleared[0])
}

// --- clear_session task detection ---

func TestCallTool_ClearSession_ActiveTask(t *testing.T) {
	cases := []struct {
		name        string
		taskStatus  string
		instruction string
		check       func(t *testing.T, m map[string]any)
	}{
		{
			name:        "RunningTaskRequiresConfirmation",
			taskStatus:  model.TaskStatusRunning,
			instruction: "long running task",
			check: func(t *testing.T, m map[string]any) {
				tasks, ok := m["running_tasks"].([]rpc.ActiveTaskSummary)
				require.True(t, ok, "running_tasks type %T", m["running_tasks"])
				require.Len(t, tasks, 1)
				assert.Equal(t, "long running task", tasks[0].Instruction)
				assert.Equal(t, model.TaskStatusRunning, tasks[0].Status)
			},
		},
		{
			name:        "PendingTaskRequiresConfirmation",
			taskStatus:  model.TaskStatusPending,
			instruction: "queued task",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, db, _, clearer := setupServerWithClear(t)
			ctx := context.Background()

			ms := store.NewMessageStore(db)
			ms.Create(ctx, "msg-1", "sk", "feishu", "hi", `{}`, "", 0) //nolint

			w := mustCreateWorker(t, s, map[string]any{"name": "W"})

			ts := store.NewTaskStore(db)
			ts.Create(ctx, model.Task{ //nolint
				MessageID: "msg-1", WorkerID: w.ID, Instruction: tc.instruction,
				Type: model.TaskTypeImmediate, Status: tc.taskStatus,
				CreatedAt: 1, UpdatedAt: 1,
			})

			result, err := s.CallTool(ctx, "clear_session", mustMarshal(t, map[string]any{
				"session_key": "sk",
			}))
			require.NoError(t, err)
			m := result.(map[string]any)
			assert.Equal(t, true, m["requires_confirmation"])
			assert.Equal(t, rpc.ClearReasonActiveTasks, m["reason"])
			if tc.check != nil {
				tc.check(t, m)
			}

			// ClearSession must NOT have been called.
			clearer.mu.Lock()
			defer clearer.mu.Unlock()
			assert.Empty(t, clearer.cleared, "ClearSession must not be called on confirmation prompt")
		})
	}
}

func TestCallTool_ClearSession_ForceSkipsTaskDetection(t *testing.T) {
	s, db, stopper, clearer := setupServerWithClear(t)
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-fsd1", "session-FSD", "feishu", "hi", `{}`, "", 0) //nolint

	w := mustCreateWorker(t, s, map[string]any{"name": "W"})

	ts := store.NewTaskStore(db)
	taskID, _ := ts.Create(ctx, model.Task{
		MessageID: "msg-fsd1", WorkerID: w.ID, Instruction: "long task",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning,
		CreatedAt: 1, UpdatedAt: 1,
	})
	// Insert a corresponding execution row so GetRunningByTaskID can find it.
	db.ExecContext(ctx, `INSERT INTO bee_executions (id, task_id, worker_id, session_id, engine, trigger_input, status, result, ai_process_pid, started_at) VALUES (?, ?, ?, '', '', '', ?, '', 0, 1)`, "exec-fsd-1", taskID, w.ID, model.ExecStatusRunning) //nolint

	result, err := s.CallTool(ctx, "clear_session", mustMarshal(t, map[string]any{
		"session_key": "session-FSD",
		"force":       true,
	}))
	require.NoError(t, err)
	m := result.(map[string]any)
	assert.Equal(t, true, m["cleared"])

	// guard: length check protects the stopper.stopped[0] index below.
	stopper.mu.Lock()
	defer stopper.mu.Unlock()
	require.Len(t, stopper.stopped, 1)
	assert.Equal(t, "exec-fsd-1", stopper.stopped[0])

	// guard: length check protects the clearer.cleared[0] index below.
	clearer.mu.Lock()
	defer clearer.mu.Unlock()
	require.Len(t, clearer.cleared, 1)
	assert.Equal(t, "session-FSD", clearer.cleared[0])
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
			s, db, _, clearer := setupServerWithClear(t)
			ctx := context.Background()

			ms := store.NewMessageStore(db)
			ms.Create(ctx, tc.msgID, tc.sessionKey, "feishu", "hi", `{}`, "", 0) //nolint

			w := mustCreateWorker(t, s, map[string]any{"name": "W"})

			// Seed a session context so the clear has something to act on; the
			// test's intent is that non-immediate pending tasks don't gate.
			ss := store.NewSessionStore(db)
			ss.UpsertSessionContext(ctx, tc.sessionKey, w.ID, "sid-w", "") //nolint

			ts := store.NewTaskStore(db)
			ts.Create(ctx, model.Task{ //nolint
				MessageID: tc.msgID, WorkerID: w.ID, Instruction: "task",
				Type: tc.taskType, Status: model.TaskStatusPending,
				CreatedAt: 1, UpdatedAt: 1,
			})

			result, err := s.CallTool(ctx, "clear_session", mustMarshal(t, map[string]any{
				"session_key": tc.sessionKey,
			}))
			require.NoError(t, err)
			m := result.(map[string]any)
			assert.Equal(t, true, m["cleared"], "%s pending task should not block clear", tc.taskType)

			// guard: length check protects the clearer.cleared[0] index below.
			clearer.mu.Lock()
			defer clearer.mu.Unlock()
			require.Len(t, clearer.cleared, 1)
			assert.Equal(t, tc.sessionKey, clearer.cleared[0])
		})
	}
}

func TestResolveDepartmentID_ByID(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)

	dept, err := ds.Create(model.Department{Name: "Engineering"})
	require.NoError(t, err)

	result, err := s.CallTool(context.Background(), "get_department",
		mustMarshal(t, map[string]any{"id": dept.ID}))
	require.NoError(t, err)
	got, ok := result.(model.Department)
	require.True(t, ok, "expected model.Department, got %T", result)
	assert.Equal(t, dept.ID, got.ID)
}

func TestResolveDepartmentID_ByName(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)

	_, err := ds.Create(model.Department{Name: "Marketing"})
	require.NoError(t, err)

	result, err := s.CallTool(context.Background(), "get_department",
		mustMarshal(t, map[string]any{"id": "Marketing"}))
	require.NoError(t, err)
	got, ok := result.(model.Department)
	require.True(t, ok, "expected model.Department, got %T", result)
	assert.Equal(t, "Marketing", got.Name)
}

func TestResolveDepartmentID_NotFound(t *testing.T) {
	s, _ := setupServerWithSender(t, "feishu", &mockSender{})
	_, err := s.CallTool(context.Background(), "get_department",
		mustMarshal(t, map[string]any{"id": "nonexistent"}))
	require.Error(t, err)
}

func TestToolListTasks_IncludesExecutions(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-exec1", "session-EX", "feishu", "hi", `{}`, "", 0) //nolint

	w := mustCreateWorker(t, s, map[string]any{"name": "W1"})

	taskResult, err := s.CallTool(ctx, "create_task", mustMarshal(t, map[string]any{
		"message_id":  "msg-exec1",
		"worker_id":   w.ID,
		"instruction": "do x",
		"type":        "immediate",
	}))
	require.NoError(t, err)
	taskID := taskResult.(map[string]string)["task_id"]

	es := store.NewExecutionStore(db, t.TempDir())
	exec, err := es.Create(store.ExecutionCreate{WorkerID: w.ID, TaskID: taskID, TriggerInput: "do x", SessionID: "sess-1", Engine: "claude"})
	require.NoError(t, err)

	result, err := s.CallTool(ctx, utils.ListTasks, mustMarshal(t, map[string]any{"worker_id": w.ID}))
	require.NoError(t, err)

	b, _ := json.Marshal(result)
	raw := string(b)
	assert.Contains(t, raw, `"executions"`)
	assert.Contains(t, raw, exec.ID)
	tasks, _ := decodePagedTaskItems(t, result)
	require.Len(t, tasks, 1)
	execsAny, ok := tasks[0]["executions"]
	require.True(t, ok, "task missing 'executions' field")
	execsList, ok := execsAny.([]any)
	require.True(t, ok, "executions is not a list, got %T", execsAny)
	assert.Len(t, execsList, 1)
}

func decodePagedTaskItems(t *testing.T, result any) ([]map[string]any, map[string]any) {
	t.Helper()
	b, err := json.Marshal(result)
	require.NoError(t, err)
	var page map[string]any
	require.NoError(t, json.Unmarshal(b, &page))
	rawItems, ok := page["items"].([]any)
	require.True(t, ok, "items missing or not array: %T", page["items"])
	items := make([]map[string]any, 0, len(rawItems))
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		require.True(t, ok, "item is not object: %T", raw)
		items = append(items, item)
	}
	return items, page
}

func TestToolListTasks_PaginatesAndLimitsExecutions(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()

	ms := store.NewMessageStore(db)
	ms.Create(ctx, "msg-page", "session-PAGE", "feishu", "hi", `{}`, "", 0) //nolint

	w := mustCreateWorker(t, s, map[string]any{"name": "W1"})
	ts := store.NewTaskStore(db)
	es := store.NewExecutionStore(db, t.TempDir())

	var newestExecForFirstTask string
	for i := 0; i < 3; i++ {
		taskID, err := ts.Create(ctx, model.Task{
			MessageID: "msg-page", WorkerID: w.ID, Instruction: fmt.Sprintf("task-%d", i),
			Type: model.TaskTypeImmediate, Status: model.TaskStatusCompleted,
			CreatedAt: int64(i + 1), UpdatedAt: int64(i + 1),
		})
		require.NoError(t, err)
		for j := 0; j < 3; j++ {
			exec, err := es.Create(store.ExecutionCreate{WorkerID: w.ID, TaskID: taskID, TriggerInput: fmt.Sprintf("run-%d-%d", i, j), SessionID: fmt.Sprintf("sess-%d-%d", i, j), Engine: "claude"})
			require.NoError(t, err)
			if i == 2 && j == 2 {
				newestExecForFirstTask = exec.ID
			}
		}
	}

	result, err := s.CallTool(ctx, utils.ListTasks, mustMarshal(t, map[string]any{
		"worker_id":       w.ID,
		"page":            1,
		"page_size":       2,
		"execution_limit": 1,
	}))
	require.NoError(t, err)
	items, page := decodePagedTaskItems(t, result)
	require.Len(t, items, 2)
	require.Equal(t, float64(3), page["total"])
	execs, ok := items[0]["executions"].([]any)
	require.True(t, ok, "expected first item to include executions, got %#v", items[0]["executions"])
	require.Len(t, execs, 1)
	firstExec := execs[0].(map[string]any)
	require.Equal(t, newestExecForFirstTask, firstExec["id"])
}

func decodeDeptTree(t *testing.T, result any) []map[string]any {
	t.Helper()
	b, err := json.Marshal(result)
	require.NoError(t, err)
	var tree []map[string]any
	require.NoError(t, json.Unmarshal(b, &tree))
	return tree
}

func TestCallTool_ListDepartments_Empty(t *testing.T) {
	s := setupServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "list_departments", mustMarshal(t, map[string]any{}))
	require.NoError(t, err)
	tree := decodeDeptTree(t, result)
	assert.Empty(t, tree)
}

func TestCallTool_ListDepartments_Tree(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)

	parent, _ := ds.Create(model.Department{Name: "R&D"})
	_, _ = ds.Create(model.Department{Name: "Frontend", ParentID: &parent.ID})
	_, _ = ds.Create(model.Department{Name: "Backend", ParentID: &parent.ID})

	result, err := s.CallTool(context.Background(), "list_departments", mustMarshal(t, map[string]any{}))
	require.NoError(t, err)
	tree := decodeDeptTree(t, result)
	require.Len(t, tree, 1)
	assert.Equal(t, "R&D", tree[0]["name"].(string))
	children := tree[0]["children"].([]any)
	assert.Len(t, children, 2)
	// Verify slim fields (no parent_id, created_at, updated_at).
	_, hasParentID := tree[0]["parent_id"]
	assert.False(t, hasParentID, "expected no parent_id in department list response")
	_, hasCreatedAt := tree[0]["created_at"]
	assert.False(t, hasCreatedAt, "expected no created_at in department list response")
}

func TestCallTool_CreateDepartment(t *testing.T) {
	s := setupServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "create_department",
		mustMarshal(t, map[string]any{"name": "Engineering"}))
	require.NoError(t, err)
	dept, ok := result.(model.Department)
	require.True(t, ok, "expected model.Department, got %T", result)
	assert.NotEmpty(t, dept.ID)
	assert.Equal(t, "Engineering", dept.Name)
}

func TestCallTool_CreateDepartment_WithParentByName(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	parent, _ := ds.Create(model.Department{Name: "R&D"})

	result, err := s.CallTool(context.Background(), "create_department",
		mustMarshal(t, map[string]any{"name": "Frontend", "parent_id": "R&D"}))
	require.NoError(t, err)
	child, ok := result.(model.Department)
	require.True(t, ok, "expected model.Department, got %T", result)
	// guard: nil check protects the *child.ParentID dereference below.
	require.NotNil(t, child.ParentID)
	assert.Equal(t, parent.ID, *child.ParentID)
}

func TestCallTool_UpdateDepartment_Name(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	dept, _ := ds.Create(model.Department{Name: "OldName"})

	result, err := s.CallTool(context.Background(), "update_department",
		mustMarshal(t, map[string]any{"id": dept.ID, "name": "NewName"}))
	require.NoError(t, err)
	updated, ok := result.(model.Department)
	require.True(t, ok, "expected model.Department, got %T", result)
	assert.Equal(t, "NewName", updated.Name)
}

func TestCallTool_DeleteDepartment(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	dept, _ := ds.Create(model.Department{Name: "ToDelete"})

	_, err := s.CallTool(context.Background(), "delete_department",
		mustMarshal(t, map[string]any{"id": dept.ID}))
	require.NoError(t, err)

	_, err = ds.GetByID(dept.ID)
	assert.Error(t, err)
}

func TestCallTool_DeleteDepartment_FailsWithChildren(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	parent, _ := ds.Create(model.Department{Name: "Parent"})
	_, _ = ds.Create(model.Department{Name: "Child", ParentID: &parent.ID})

	_, err := s.CallTool(context.Background(), "delete_department",
		mustMarshal(t, map[string]any{"id": parent.ID}))
	assert.Error(t, err)
}

func TestCallTool_ListWorkers_FilterByDepartment(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
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
	require.NoError(t, err)
	items, total := decodeListWorkersResult(t, result)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, "Alice", item["name"].(string))
}

func TestCallTool_ListWorkers_FilterByDepartment_Recursive(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	ws := store.NewWorkerStore(db)

	parent, _ := ds.Create(model.Department{Name: "R&D"})
	child, _ := ds.Create(model.Department{Name: "Frontend", ParentID: &parent.ID})

	w1, _ := ws.Create(model.Worker{Name: "Alice"})
	w2, _ := ws.Create(model.Worker{Name: "Bob"})
	_ = ds.SetWorkerDepartments(w1.ID, []string{parent.ID})
	_ = ds.SetWorkerDepartments(w2.ID, []string{child.ID})

	// recursive (default): should return both.
	result, err := s.CallTool(context.Background(), "list_workers",
		mustMarshal(t, map[string]any{"department_id": parent.ID}))
	require.NoError(t, err)
	items, _ := decodeListWorkersResult(t, result)
	assert.Len(t, items, 2)

	// non-recursive: should return only Alice.
	result2, err := s.CallTool(context.Background(), "list_workers",
		mustMarshal(t, map[string]any{"department_id": parent.ID, "recursive": false}))
	require.NoError(t, err)
	items2, _ := decodeListWorkersResult(t, result2)
	// guard: length check protects the items2[0] index below.
	require.Len(t, items2, 1)
	item2 := items2[0].(map[string]any)
	assert.Equal(t, "Alice", item2["name"].(string))
}

func TestCallTool_CreateWorker_WithDepartment(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	dept, _ := ds.Create(model.Department{Name: "Engineering"})

	result, err := s.CallTool(context.Background(), "create_worker",
		mustMarshal(t, map[string]any{"name": "Alice", "department_ids": dept.ID}))
	require.NoError(t, err)
	w, ok := result.(model.Worker)
	require.True(t, ok, "expected model.Worker, got %T", result)

	depts, err := ds.GetWorkerDepartments(w.ID)
	require.NoError(t, err)
	// guard: length check protects the depts[0] index below.
	require.Len(t, depts, 1)
	assert.Equal(t, dept.ID, depts[0].ID)
}

func TestCallTool_UpdateWorker_SetDepartments(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	ws := store.NewWorkerStore(db)

	dept1, _ := ds.Create(model.Department{Name: "Engineering"})
	dept2, _ := ds.Create(model.Department{Name: "Design"})
	w, _ := ws.Create(model.Worker{Name: "Alice"})
	_ = ds.SetWorkerDepartments(w.ID, []string{dept1.ID})

	_, err := s.CallTool(context.Background(), "update_worker",
		mustMarshal(t, map[string]any{"worker_id": w.ID, "department_ids": dept2.Name}))
	require.NoError(t, err)

	depts, _ := ds.GetWorkerDepartments(w.ID)
	// guard: length check protects the depts[0] index below.
	require.Len(t, depts, 1)
	assert.Equal(t, dept2.ID, depts[0].ID)
}

func TestCallTool_UpdateWorker_ClearDepartments(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ds := store.NewDepartmentStore(db)
	ws := store.NewWorkerStore(db)

	dept, _ := ds.Create(model.Department{Name: "Engineering"})
	w, _ := ws.Create(model.Worker{Name: "Alice"})
	_ = ds.SetWorkerDepartments(w.ID, []string{dept.ID})

	_, err := s.CallTool(context.Background(), "update_worker",
		mustMarshal(t, map[string]any{"worker_id": w.ID, "department_ids": ""}))
	require.NoError(t, err)

	depts, _ := ds.GetWorkerDepartments(w.ID)
	assert.Empty(t, depts)
}

func workerCtx(workerID string, scopes []string) context.Context {
	ctx := context.WithValue(context.Background(), rpc.CtxWorkerIDKey, workerID)
	return context.WithValue(ctx, rpc.CtxScopesKey, scopes)
}

func TestCheckWorkerScope_WorkerWithScope_CanCallScopedTool(t *testing.T) {
	s := setupServerWithMessaging(t)
	ctx := workerCtx("wid-1", []string{"read:workers"})
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	assert.NoError(t, err)
}

func TestCheckWorkerScope_WorkerWithoutScope_CannotCallScopedTool(t *testing.T) {
	s := setupServerWithMessaging(t)
	ctx := workerCtx("wid-1", nil) // no scopes
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	assert.Error(t, err)
}

func TestCheckWorkerScope_WorkerWithWrongScope_CannotCallScopedTool(t *testing.T) {
	s := setupServerWithMessaging(t)
	ctx := workerCtx("wid-1", []string{"read:tasks"}) // has tasks scope, not workers
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	assert.Error(t, err)
}

func TestCheckWorkerScope_BeeToken_AlwaysAllowed(t *testing.T) {
	s := setupServerWithMessaging(t)
	// Bee token: no workerID in context.
	ctx := context.Background()
	_, err := s.CallTool(ctx, utils.ListWorkers, mustMarshal(t, map[string]any{}))
	assert.NoError(t, err)
}

func TestCheckWorkerScope_WorkerToken_NonScopedTool_Unchanged(t *testing.T) {
	s := setupServerWithMessaging(t)
	ctx := workerCtx("wid-1", nil) // no scopes
	// send_message has no scope requirement — existing behavior, worker can call it.
	_, err := s.CallTool(ctx, utils.SendMessage, mustMarshal(t, map[string]any{
		"message_id": "nonexistent",
		"content":    "test",
	}))
	// Should NOT be a permission denied error.
	if err != nil {
		assert.NotEqual(t, "permission denied: scope read:workers required", err.Error())
	}
}

func TestCallTool_ListOutboundMessages(t *testing.T) {
	s, db := setupServerWithSender(t, "feishu", &mockSender{})
	ctx := context.Background()

	oms := store.NewOutboundMessageStore(db)
	require.NoError(t, oms.Create(ctx, store.OutboundMessage{
		ID: "out-1", SessionKey: "sk1", Platform: "feishu",
		Content: "reply", Status: store.OutboundStatusSent,
		SourceType: store.SourceTypeWorker, SourceID: "worker-X",
		SentAt: 1000,
	}))
	require.NoError(t, oms.Create(ctx, store.OutboundMessage{
		ID: "out-2", SessionKey: "sk2", Platform: "local",
		Content: "hi", Status: store.OutboundStatusFailed,
		SourceType: store.SourceTypeBee,
		SentAt:     2000,
	}))

	// No filter — returns all.
	result, err := s.CallTool(ctx, "list_outbound_messages", mustMarshal(t, map[string]any{}))
	require.NoError(t, err)
	m := decodeResult(t, result)
	assert.Equal(t, float64(2), m["total"])

	// Filter by source_type=worker.
	result2, err := s.CallTool(ctx, "list_outbound_messages", mustMarshal(t, map[string]any{
		"source_type": store.SourceTypeWorker,
	}))
	require.NoError(t, err)
	m2 := decodeResult(t, result2)
	assert.Equal(t, float64(1), m2["total"])
}

func TestCallTool_CreateWorker_WithEngine(t *testing.T) {
	s := setupServerWithMessaging(t)
	result, err := s.CallTool(context.Background(), "create_worker", mustMarshal(t, map[string]any{
		"name":   "EngineBot",
		"engine": "claude",
	}))
	require.NoError(t, err)
	w, ok := result.(model.Worker)
	require.True(t, ok, "expected model.Worker, got %T", result)
	assert.Equal(t, "claude", w.Engine)
}

func TestCallTool_UpdateWorker_Engine(t *testing.T) {
	cases := []struct {
		name         string
		createEngine string
		engine       string
		wantEngine   string
	}{
		{"WithEngine", "", "claude", "claude"},
		{"ClearEngine", "claude", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := setupServerWithMessaging(t)
			createArgs := map[string]any{"name": "Bot"}
			if tc.createEngine != "" {
				createArgs["engine"] = tc.createEngine
			}
			w := mustCreateWorker(t, s, createArgs)

			result, err := s.CallTool(context.Background(), "update_worker", mustMarshal(t, map[string]any{
				"worker_id": w.ID,
				"engine":    tc.engine,
			}))
			require.NoError(t, err)
			updated, ok := result.(model.Worker)
			require.True(t, ok, "expected model.Worker, got %T", result)
			assert.Equal(t, tc.wantEngine, updated.Engine)
		})
	}
}

func TestCallTool_UpdateWorker_InvalidEngine(t *testing.T) {
	s := setupServerWithMessaging(t)
	w := mustCreateWorker(t, s, map[string]any{"name": "Bot"})

	_, err := s.CallTool(context.Background(), "update_worker", mustMarshal(t, map[string]any{
		"worker_id": w.ID,
		"engine":    "not-a-real-engine",
	}))
	assert.Error(t, err)
}
