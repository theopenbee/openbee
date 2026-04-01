# Worker Name Prefix Auto-Injection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-prepend `"<worker_name>\n"` to outbound messages sent via `send_message`, so workers no longer need to manually prefix their messages with a name.

**Architecture:** Thread a `context.Context` carrying the caller's `worker_id` through the MCP call stack (`HandleCall` / `HandleMessages` → `dispatch` → `handleToolCall` → `callToolFn` → `toolSendMessage`). Inside `toolSendMessage`, extract the worker_id from context, look up the worker name, and prepend it to the content. Bee tokens have an empty `worker_id` and are unaffected.

**Tech Stack:** Go, Gin, JWT (existing auth), SQLite (existing stores), testify

---

## File Map

| File | Change |
|---|---|
| `internal/mcp/server.go` | `callToolFn` type; `HandleCall`, `HandleMessages`, `dispatch`, `handleToolCall` all accept/pass `context.Context`; `NewWorkerServer` gains `*store.WorkerStore` parameter |
| `internal/mcp/tools.go` | `CallTool`, `beeCallTool`, `workerCallTool`, `toolSendMessage` accept `context.Context`; prefix logic in `toolSendMessage` |
| `internal/mcp/tools_test.go` | Update all `s.CallTool(...)` callers to pass `context.Background()`; add two new prefix tests |
| `internal/mcp/server_test.go` | Update `s.CallTool(...)` callers if any (currently uses `HandleCall` HTTP path only — no change needed) |
| `internal/app/app.go` | Pass `s.workerStore` to `mcp.NewWorkerServer(...)` |
| `/Users/tengyongzhi/.claude/skills/openbee-worker/skill.md` | Remove name-prefix requirement; update examples to plain content |

---

### Task 1: Thread `context.Context` through the call stack

No behavior change in this task — pure refactoring to wire context end-to-end.

**Files:**
- Modify: `internal/mcp/server.go`
- Modify: `internal/mcp/tools.go`

- [ ] **Step 1: Update `callToolFn` type and all four methods in `tools.go`**

In `internal/mcp/tools.go`, change the signatures of `CallTool`, `beeCallTool`, `workerCallTool`, and `toolSendMessage` to accept `ctx context.Context` as their first parameter. Pass `ctx` through each call.

```go
// CallTool is exported for testing. Production code uses callToolFn via handleToolCall.
func (s *MCPServer) CallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	return s.callToolFn(ctx, name, args)
}

// beeCallTool dispatches to the named tool handler and returns the result.
func (s *MCPServer) beeCallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	switch name {
	case toolnames.ListWorkers:
		return s.toolListWorkers(args)
	case toolnames.GetWorker:
		return s.toolGetWorker(args)
	case toolnames.CreateWorker:
		return s.toolCreateWorker(args)
	case toolnames.UpdateWorker:
		return s.toolUpdateWorker(args)
	case toolnames.DeleteWorker:
		return s.toolDeleteWorker(args)
	case toolnames.CreateTask:
		return s.toolCreateTask(args)
	case toolnames.ListTasks:
		return s.toolListTasks(args)
	case toolnames.CancelTask:
		return s.toolCancelTask(args)
	case toolnames.SendMessage:
		return s.toolSendMessage(ctx, args)
	case toolnames.ClearSession:
		return s.toolClearSession(args)
	case toolnames.GetWorkerStatus:
		return s.toolGetWorkerStatus(args)
	case toolnames.GetSystemOverview:
		return s.toolGetSystemOverview(args)
	case toolnames.ListBeeExecutions:
		return s.toolListBeeExecutions(args)
	case toolnames.SaveMemory:
		return s.toolSaveMemory(args)
	case toolnames.GetMemory:
		return s.toolGetMemory(args)
	case toolnames.DeleteMemory:
		return s.toolDeleteMemory(args)
	case toolnames.ListSessionContexts:
		return s.toolListSessionContexts(args)
	case toolnames.ClearWorkerSession:
		return s.toolClearWorkerSession(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// workerCallTool delegates to beeCallTool after checking the worker allowlist.
func (s *MCPServer) workerCallTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	if !workerToolNames[name] {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return s.beeCallTool(ctx, name, args)
}
```

Also add `"context"` to the imports of `tools.go` if not already present.

Update `toolSendMessage` signature (no logic change yet):

```go
func (s *MCPServer) toolSendMessage(ctx context.Context, args json.RawMessage) (any, error) {
    // ... existing body unchanged ...
}
```

- [ ] **Step 2: Update `callToolFn` field type and all callers in `server.go`**

In `internal/mcp/server.go`:

1. Change the field type:

```go
callToolFn func(ctx context.Context, name string, args json.RawMessage) (any, error)
```

2. Update `HandleCall` to inject caller identity into context and pass it:

```go
func (s *MCPServer) HandleCall(c *gin.Context) {
	var req struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	ctx := context.WithValue(context.Background(), ctxKeyWorkerID, c.GetString(CtxKeyWorkerID))
	result, err := s.callToolFn(ctx, req.Name, req.Arguments)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}
```

3. Add `dispatch` context parameter and propagate through `handleToolCall`:

```go
func (s *MCPServer) dispatch(ctx context.Context, req rpcRequest) rpcResponse {
	switch req.Method {
	// ... all cases unchanged except tools/call ...
	case "tools/call":
		return s.handleToolCall(ctx, req)
	default:
		return errResponse(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *MCPServer) handleToolCall(ctx context.Context, req rpcRequest) rpcResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.callToolFn(ctx, params.Name, params.Arguments)
	if err != nil {
		return okResponse(req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		})
	}

	data, _ := json.Marshal(result)
	return okResponse(req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(data)}},
	})
}
```

4. Update `HandleMessages` to extract worker_id and pass to `dispatch`:

```go
func (s *MCPServer) HandleMessages(c *gin.Context) {
	sessionID := c.Query("session_id")

	s.mu.Lock()
	ch, ok := s.sessions[sessionID]
	s.mu.Unlock()

	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown session_id"})
		return
	}

	var req rpcRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ch <- errResponse(nil, -32700, "parse error: "+err.Error())
		c.Status(http.StatusAccepted)
		return
	}

	ctx := context.WithValue(context.Background(), ctxKeyWorkerID, c.GetString(CtxKeyWorkerID))
	resp := s.dispatch(ctx, req)

	if req.ID != nil {
		ch <- resp
	}

	c.Status(http.StatusAccepted)
}
```

5. Add the typed context key near the top of `server.go` (after the `const` block or as a new `const`/`type` block):

```go
type ctxKey string

const ctxKeyWorkerID ctxKey = CtxKeyWorkerID
```

6. Add `"context"` to the imports of `server.go` if not already present.

- [ ] **Step 3: Add `workerStore` to `NewWorkerServer` and update `app.go`**

The Worker MCP server needs to look up worker names, so it needs a `WorkerStore`. Update `NewWorkerServer` in `internal/mcp/server.go`:

```go
func NewWorkerServer(
	ts *store.TaskStore,
	ms *store.MessageStore,
	senders map[string]platform.PlatformSenderAdapter,
	memStore *store.MemoryStore,
	ws *store.WorkerStore,
) *MCPServer {
	s := &MCPServer{
		basePath:     config.MCPWorkerBasePath,
		taskStore:    ts,
		messageStore: ms,
		senders:      senders,
		memoryStore:  memStore,
		workerStore:  ws,
		sessions:     make(map[string]chan rpcResponse),
	}
	s.schemasFn = workerToolSchemas
	s.callToolFn = s.workerCallTool
	return s
}
```

Update the call site in `internal/app/app.go` (line ~116):

```go
workerMCPSrv := mcp.NewWorkerServer(s.taskStore, s.msgStore, sendersByPlatform, s.memoryStore, s.workerStore)
```

- [ ] **Step 4: Fix all test callers of `CallTool` in `tools_test.go`**

Every call to `s.CallTool(name, args)` in `internal/mcp/tools_test.go` must become `s.CallTool(context.Background(), name, args)`. There are roughly 30 call sites — use a global find-and-replace within the file.

Pattern to replace:
```
s.CallTool(
```
Replace with:
```
s.CallTool(context.Background(),
```

Also add `"context"` to the imports in `tools_test.go` if not already present.

- [ ] **Step 5: Verify the build compiles and existing tests pass**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-ctl-cli
go build ./...
go test ./internal/mcp/... -v 2>&1 | tail -30
```

Expected: all existing tests pass. Fix any compilation errors before proceeding.

- [ ] **Step 6: Commit the refactor**

```bash
git add internal/mcp/server.go internal/mcp/tools.go internal/mcp/tools_test.go internal/app/app.go
git commit -m "refactor(mcp): thread context.Context through call stack"
```

---

### Task 2: Auto-prepend worker name in `toolSendMessage`

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 1: Write two failing tests**

Add these two tests to `internal/mcp/tools_test.go` after the existing `send_message` tests (after `TestCallTool_SendMessage_MessageNotFound`):

```go
func TestCallTool_SendMessage_WorkerPrefixesContent(t *testing.T) {
	mock := &mockSender{}
	s, db := setupMCPServerWithSender(t, "feishu", mock)
	ctx := context.Background()

	// Create a worker and a message
	ws := store.NewWorkerStore(db)
	w, err := ws.Create("毛毛", "test worker", "", "")
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
		"content":    "任务完成",
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
	want := "毛毛\n任务完成"
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
		"content":    "任务完成",
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
	want := "worker-deleted-xyz\n任务完成"
	if mock.sent[0].Content != want {
		t.Errorf("expected content %q, got %q", want, mock.sent[0].Content)
	}
}
```

Note: both tests use `mcp.CtxWorkerIDKey` — a new exported symbol to be added in the next step.

- [ ] **Step 2: Run the new tests to confirm they fail to compile**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-ctl-cli
go test ./internal/mcp/... -run "TestCallTool_SendMessage_WorkerPrefix|TestCallTool_SendMessage_WorkerDeleted" -v 2>&1 | head -20
```

Expected: compile error — `mcp.CtxWorkerIDKey` undefined.

- [ ] **Step 3: Add `CtxWorkerIDKey` export and prefix logic in `toolSendMessage`**

In `internal/mcp/server.go`, export the typed context key so tests can use it:

```go
// CtxWorkerIDKey is the context key used to carry the caller's worker ID through tool dispatch.
// It is exported so tests can construct contexts that simulate worker calls.
const CtxWorkerIDKey = ctxKeyWorkerID
```

In `internal/mcp/tools.go`, update `toolSendMessage` to prepend the worker name before sending text content:

```go
func (s *MCPServer) toolSendMessage(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		MessageID string `json:"message_id"`
		Content   string `json:"content"`
		MediaPath string `json:"media_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if params.MessageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}
	if params.Content == "" && params.MediaPath == "" {
		return nil, fmt.Errorf("at least one of 'content' or 'media_path' must be provided")
	}

	// Auto-prepend worker name when caller is a worker.
	if workerID, _ := ctx.Value(ctxKeyWorkerID).(string); workerID != "" && params.Content != "" {
		name := workerID // fallback: use worker_id if worker record not found
		if s.workerStore != nil {
			if w, err := s.workerStore.GetByID(workerID); err == nil {
				name = w.Name
			}
		}
		params.Content = name + "\n" + params.Content
	}

	stored, err := s.messageStore.GetByID(context.Background(), params.MessageID)
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}

	sender, ok := s.senders[stored.Platform]
	if !ok {
		return nil, fmt.Errorf("no sender registered for platform %q", stored.Platform)
	}

	replyTo := platform.InboundMessage{
		Platform:   stored.Platform,
		SessionKey: stored.SessionKey,
		Raw:        stored.Raw,
	}

	// Send text first if both content and media_path are provided
	if params.Content != "" {
		outbound := platform.OutboundMessage{ReplyTo: replyTo, Content: params.Content}
		if err := sender.Send(context.Background(), outbound); err != nil {
			return nil, fmt.Errorf("send text message: %w", err)
		}
	}

	// Send media if media_path is provided
	if params.MediaPath != "" {
		outbound := platform.OutboundMessage{ReplyTo: replyTo, MediaPath: params.MediaPath}
		if err := sender.Send(context.Background(), outbound); err != nil {
			return nil, fmt.Errorf("send media message: %w", err)
		}
	}

	return map[string]string{"status": "sent"}, nil
}
```

- [ ] **Step 4: Run all new and existing send_message tests**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-ctl-cli
go test ./internal/mcp/... -run "TestCallTool_SendMessage" -v
```

Expected output (all pass):
```
--- PASS: TestCallTool_SendMessage_CallsSender
--- PASS: TestCallTool_SendMessage_MissingMessageID
--- PASS: TestCallTool_SendMessage_MissingContent
--- PASS: TestCallTool_SendMessage_UnknownPlatform
--- PASS: TestCallTool_SendMessage_MessageNotFound
--- PASS: TestCallTool_SendMessage_WorkerPrefixesContent
--- PASS: TestCallTool_SendMessage_WorkerDeletedFallsBackToWorkerID
PASS
```

Note: `TestCallTool_SendMessage_CallsSender` tests the bee path (no worker_id in context) — it must still pass with `Content == "Task done!"` unchanged.

- [ ] **Step 5: Run full test suite**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-ctl-cli
go test ./... 2>&1 | tail -20
```

Expected: all packages pass.

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/server.go internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat(mcp): auto-prepend worker name to send_message content"
```

---

### Task 3: Remove name-prefix requirement from `openbee-worker/skill.md`

**Files:**
- Modify: `/Users/tengyongzhi/.claude/skills/openbee-worker/skill.md`

- [ ] **Step 1: Remove the name-prefix requirement**

In `/Users/tengyongzhi/.claude/skills/openbee-worker/skill.md`, find the notification spec section and make these changes:

1. Remove the sentence: `发送通知的消息内容以姓名作为前缀，格式为 "姓名: 消息内容"。这是强制要求，不可省略。`

2. Remove the format template line: `openbee ctl message send --message-id <message_id> --content "姓名: 消息内容"`

3. Update all four examples to plain content (no name prefix):

```bash
openbee ctl message send --message-id <id> --content "已收到任务，正在处理。"

openbee ctl message send --message-id <id> --content "第一阶段完成，已修改 foo.go。下一步开始更新测试。"

openbee ctl message send --message-id <id> --content "任务完成。已修改 3 个文件，所有测试通过。"

openbee ctl message send --message-id <id> --content "遇到问题需要确认：数据库迁移会删除旧字段，是否继续？"
```

- [ ] **Step 2: Verify the file reads correctly**

```bash
grep -n "姓名\|前缀\|name prefix" /Users/tengyongzhi/.claude/skills/openbee-worker/skill.md
```

Expected: no matches (the requirement has been removed).

- [ ] **Step 3: Commit**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee/.worktrees/feat-ctl-cli
git add /Users/tengyongzhi/.claude/skills/openbee-worker/skill.md
git commit -m "feat(worker): remove manual name-prefix requirement from skill"
```

Note: `skill.md` lives outside the repo. If git does not track it, skip the git step and just confirm the file is saved.
