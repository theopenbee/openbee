# List Outbound Messages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `openbee ctl message list-outbound` CLI command and `list_outbound_messages` MCP Tool to query `bee_outbound_messages` with flexible filtering and pagination.

**Architecture:** Add a `ListFiltered` method to `OutboundMessageStore` following the same pattern as `MessageStore.ListFiltered`. Wire a new `outboundMessageStore` field into `MCPServer`, dispatch the new tool in `beeCallTool`, and expose it via a new CLI subcommand.

**Tech Stack:** Go, SQLite (`database/sql`), Cobra CLI, internal MCP server dispatch pattern.

---

## File Map

| Action | File | What Changes |
|--------|------|--------------|
| Modify | `internal/infra/store/outbound_message_store.go` | Add `OutboundMessageFilter`, `ListedOutboundMessage`, `ListFiltered` |
| Modify | `internal/infra/utils/toolnames.go` | Add `ListOutboundMessages` constant |
| Modify | `internal/infra/auth/scopes.go` | Add `ListOutboundMessages` to `ToolScopeMap` |
| Modify | `internal/mcp/server.go` | Add `outboundMessageStore` field; update `NewBeeServer` signature |
| Modify | `internal/mcp/tools.go` | Add `toolListOutboundMessages`; add case to `beeCallTool` switch |
| Modify | `internal/app/app.go` | Pass `s.outboundMsgStore` to `NewBeeServer` |
| Modify | `cmd/openbee/ctl_message.go` | Add `list-outbound` subcommand |
| Create | `internal/infra/store/outbound_message_store_test.go` | Unit tests for `ListFiltered` |
| Modify | `internal/mcp/tools_test.go` | Update helper stubs; add `TestCallTool_ListOutboundMessages` |

---

### Task 1: Store — Add `ListFiltered` to `OutboundMessageStore`

**Files:**
- Modify: `internal/infra/store/outbound_message_store.go`
- Create: `internal/infra/store/outbound_message_store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/infra/store/outbound_message_store_test.go`:

```go
package store

import (
	"context"
	"testing"
)

func setupOutboundStore(t *testing.T) *OutboundMessageStore {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewOutboundMessageStore(db)
}

func seedOutbound(t *testing.T, s *OutboundMessageStore, msgs []OutboundMessage) {
	t.Helper()
	ctx := context.Background()
	for _, m := range msgs {
		if err := s.Create(ctx, m); err != nil {
			t.Fatalf("seed outbound: %v", err)
		}
	}
}

func TestOutboundMessageStore_ListFiltered_NoFilter(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Content: "hello", Status: OutboundStatusSent, SourceType: SourceTypeBee, SentAt: 1000},
		{ID: "o2", SessionKey: "sk2", Platform: "local",  Content: "world", Status: OutboundStatusFailed, SourceType: SourceTypeWorker, SourceID: "w1", SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 2 {
		t.Errorf("total: want 2, got %d", total)
	}
	if len(msgs) != 2 {
		t.Errorf("len(msgs): want 2, got %d", len(msgs))
	}
}

func TestOutboundMessageStore_ListFiltered_BySessionKey(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 1000},
		{ID: "o2", SessionKey: "sk2", Platform: "feishu", Status: OutboundStatusSent, SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SessionKey: "sk1"}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 1 {
		t.Errorf("total: want 1, got %d", total)
	}
	if len(msgs) != 1 || msgs[0].ID != "o1" {
		t.Errorf("expected o1, got %+v", msgs)
	}
}

func TestOutboundMessageStore_ListFiltered_BySourceType(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeBee,    SentAt: 1000},
		{ID: "o2", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeWorker, SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SourceType: SourceTypeWorker}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 1 {
		t.Errorf("total: want 1, got %d", total)
	}
	if len(msgs) != 1 || msgs[0].ID != "o2" {
		t.Errorf("expected o2, got %+v", msgs)
	}
}

func TestOutboundMessageStore_ListFiltered_BySourceID(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeWorker, SourceID: "worker-A", SentAt: 1000},
		{ID: "o2", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeWorker, SourceID: "worker-B", SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SourceID: "worker-A"}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 1 || msgs[0].ID != "o1" {
		t.Errorf("expected o1, got total=%d msgs=%+v", total, msgs)
	}
}

func TestOutboundMessageStore_ListFiltered_BySentAtRange(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 1000},
		{ID: "o2", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 2000},
		{ID: "o3", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 3000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SentAtFrom: 1500, SentAtTo: 2500}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 1 || msgs[0].ID != "o2" {
		t.Errorf("expected o2, got total=%d msgs=%+v", total, msgs)
	}
}

func TestOutboundMessageStore_ListFiltered_Pagination(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 1000},
		{ID: "o2", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 2000},
		{ID: "o3", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 3000},
	})

	// page 1 (limit=2, offset=0) → most recent 2
	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{}, 2, 0)
	if err != nil {
		t.Fatalf("ListFiltered page1: %v", err)
	}
	if total != 3 {
		t.Errorf("total: want 3, got %d", total)
	}
	if len(msgs) != 2 {
		t.Errorf("page1 len: want 2, got %d", len(msgs))
	}
	// Results ordered by sent_at DESC — first item is o3
	if msgs[0].ID != "o3" {
		t.Errorf("page1[0]: want o3, got %s", msgs[0].ID)
	}

	// page 2 (limit=2, offset=2) → remaining 1
	msgs2, _, err := s.ListFiltered(context.Background(), OutboundMessageFilter{}, 2, 2)
	if err != nil {
		t.Fatalf("ListFiltered page2: %v", err)
	}
	if len(msgs2) != 1 || msgs2[0].ID != "o1" {
		t.Errorf("page2: want [o1], got %+v", msgs2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/infra/store/ -run TestOutboundMessageStore_ListFiltered -v
```

Expected: compile error — `OutboundMessageFilter` and `ListFiltered` undefined.

- [ ] **Step 3: Implement `ListFiltered` in `outbound_message_store.go`**

Append to `internal/infra/store/outbound_message_store.go`:

```go
// OutboundMessageFilter holds optional filter criteria for ListFiltered.
// Zero values are ignored.
type OutboundMessageFilter struct {
	SessionKey string
	Platform   string
	Status     string // "sent" | "failed"
	SourceType string // "bee" | "worker" | "system"
	SourceID   string
	SentAtFrom int64 // inclusive lower bound (Unix ms); 0 = no lower bound
	SentAtTo   int64 // inclusive upper bound (Unix ms); 0 = no upper bound
}

// ListedOutboundMessage is a bee_outbound_messages row for admin/API listing purposes.
type ListedOutboundMessage struct {
	ID           string `json:"id"`
	SessionKey   string `json:"session_key"`
	Platform     string `json:"platform"`
	Content      string `json:"content"`
	Status       string `json:"status"`
	SourceType   string `json:"source_type"`
	SourceID     string `json:"source_id"`
	InboundMsgID string `json:"inbound_msg_id"`
	Error        string `json:"error"`
	SentAt       int64  `json:"sent_at"`
}

// ListFiltered returns paginated outbound messages matching the given filters,
// ordered by sent_at DESC.
func (s *OutboundMessageStore) ListFiltered(ctx context.Context, f OutboundMessageFilter, limit, offset int) ([]ListedOutboundMessage, int, error) {
	where, args := outboundFilterWhere(f)

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bee_outbound_messages"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	queryArgs := append(args[:len(args):len(args)], limit, offset)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_key, platform, content, status, source_type, source_id, inbound_msg_id, error, sent_at
		 FROM bee_outbound_messages`+where+` ORDER BY sent_at DESC LIMIT ? OFFSET ?`,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var msgs []ListedOutboundMessage
	for rows.Next() {
		var m ListedOutboundMessage
		if err := rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.Status,
			&m.SourceType, &m.SourceID, &m.InboundMsgID, &m.Error, &m.SentAt); err != nil {
			return nil, 0, err
		}
		msgs = append(msgs, m)
	}
	return msgs, total, rows.Err()
}

func outboundFilterWhere(f OutboundMessageFilter) (string, []any) {
	var b whereBuilder
	if f.SessionKey != "" { b.add("session_key = ?", f.SessionKey) }
	if f.Platform != ""   { b.add("platform = ?", f.Platform) }
	if f.Status != ""     { b.add("status = ?", f.Status) }
	if f.SourceType != "" { b.add("source_type = ?", f.SourceType) }
	if f.SourceID != ""   { b.add("source_id = ?", f.SourceID) }
	if f.SentAtFrom > 0   { b.add("sent_at >= ?", f.SentAtFrom) }
	if f.SentAtTo > 0     { b.add("sent_at <= ?", f.SentAtTo) }
	return b.build()
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/infra/store/ -run TestOutboundMessageStore_ListFiltered -v
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/outbound_message_store.go internal/infra/store/outbound_message_store_test.go
git commit -m "feat: add OutboundMessageStore.ListFiltered with filter and pagination"
```

---

### Task 2: Constants and Auth — Tool Name and Scope

**Files:**
- Modify: `internal/infra/utils/toolnames.go`
- Modify: `internal/infra/auth/scopes.go`

- [ ] **Step 1: Add the tool name constant**

In `internal/infra/utils/toolnames.go`, add after `ListMessages = "list_messages"`:

```go
	ListOutboundMessages = "list_outbound_messages"
```

Result (lines 28–30):
```go
	ListMessages         = "list_messages"
	ListOutboundMessages = "list_outbound_messages"
	ListExecutions       = "list_executions"
```

- [ ] **Step 2: Add to ToolScopeMap**

In `internal/infra/auth/scopes.go`, add after `utils.ListMessages: ScopeReadMessages,`:

```go
	utils.ListOutboundMessages: ScopeReadMessages,
```

Result (lines 66–68):
```go
	utils.ListMessages:         ScopeReadMessages,
	utils.ListOutboundMessages: ScopeReadMessages,
	utils.ListExecutions:       ScopeReadExecutions,
```

- [ ] **Step 3: Build to verify no compile errors**

```bash
go build ./...
```

Expected: builds with no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/utils/toolnames.go internal/infra/auth/scopes.go
git commit -m "feat: add ListOutboundMessages tool name constant and scope mapping"
```

---

### Task 3: MCP Server — Wire `outboundMessageStore`

**Files:**
- Modify: `internal/mcp/server.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Add the field and update `NewBeeServer`**

In `internal/mcp/server.go`, add the field after `messageStore` (line 44):

```go
	messageStore         *store.MessageStore
	outboundMessageStore *store.OutboundMessageStore
```

Update `NewBeeServer` to accept the new store (add `oms *store.OutboundMessageStore` after `ms *store.MessageStore`):

```go
func NewBeeServer(
	ws *store.WorkerStore,
	mgr *worker.Manager,
	ts *store.TaskStore,
	ms *store.MessageStore,
	oms *store.OutboundMessageStore,
	senders map[string]platform.PlatformSenderAdapter,
	execStopper ExecutionStopper,
	sessionClearer SessionClearer,
	es *store.ExecutionStore,
	memStore *store.MemoryStore,
	sessionStore *store.SessionStore,
	ds *store.DepartmentStore,
) *MCPServer {
	return &MCPServer{
		workerStore:          ws,
		manager:              mgr,
		taskStore:            ts,
		messageStore:         ms,
		outboundMessageStore: oms,
		senders:              senders,
		execStopper:          execStopper,
		sessionClearer:       sessionClearer,
		executionStore:       es,
		memoryStore:          memStore,
		sessionStore:         sessionStore,
		departmentStore:      ds,
	}
}
```

- [ ] **Step 2: Update `app.go` call site**

In `internal/app/app.go` line 124, pass `s.outboundMsgStore` as the second argument after `s.msgStore`:

```go
beeMCPSrv := mcp.NewBeeServer(s.workerStore, mgr, s.taskStore, s.msgStore, s.outboundMsgStore, sendersByPlatform, mgr, disp, s.execStore, s.memoryStore, s.sessionStore, s.departmentStore)
```

- [ ] **Step 3: Build to verify no compile errors**

```bash
go build ./...
```

Expected: compile error on `tools_test.go` only (test helper calls `NewBeeServer` with old signature).

- [ ] **Step 4: Fix test helpers in `tools_test.go`**

There are 3 call sites. Add `store.NewOutboundMessageStore(db)` as the 5th argument in each.

At line 51 (function `setupMCPServer`):
```go
return mcp.NewBeeServer(ws, mgr, ts, ms, store.NewOutboundMessageStore(db), senders, nil, nil, es, store.NewMemoryStore(db), store.NewSessionStore(db), store.NewDepartmentStore(db))
```

At line 237 (function `setupMCPServerWithSender`):
```go
return mcp.NewBeeServer(ws, mgr, ts, ms, store.NewOutboundMessageStore(db), senders, nil, nil, es, store.NewMemoryStore(db), store.NewSessionStore(db), store.NewDepartmentStore(db)), db
```

At line 487 (function `setupMCPServerWithClear`):
```go
return mcp.NewBeeServer(ws, mgr, ts, ms, store.NewOutboundMessageStore(db), senders, stopper, clearer, es, store.NewMemoryStore(db), store.NewSessionStore(db), store.NewDepartmentStore(db)), db, stopper, clearer
```

- [ ] **Step 5: Build and test to verify no compile errors**

```bash
go build ./...
go test ./internal/mcp/ -v 2>&1 | tail -5
```

Expected: all existing MCP tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/server.go internal/app/app.go internal/mcp/tools_test.go
git commit -m "feat: wire outboundMessageStore into MCPServer"
```

---

### Task 4: MCP Tool — `toolListOutboundMessages`

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/tools_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/mcp/tools_test.go`:

```go
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
	m := result.(map[string]any)
	if m["total"].(float64) != 2 {
		t.Errorf("total: want 2, got %v", m["total"])
	}

	// Filter by source_type=worker
	result2, err := s.CallTool(ctx, "list_outbound_messages", mustMarshal(t, map[string]any{
		"source_type": "worker",
	}))
	if err != nil {
		t.Fatalf("CallTool filter: %v", err)
	}
	m2 := result2.(map[string]any)
	if m2["total"].(float64) != 1 {
		t.Errorf("filtered total: want 1, got %v", m2["total"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/mcp/ -run TestCallTool_ListOutboundMessages -v
```

Expected: FAIL — `unknown tool: list_outbound_messages`.

- [ ] **Step 3: Add the tool handler**

In `internal/mcp/tools.go`, add after `toolListMessages` (after line 1170):

```go
func (s *MCPServer) toolListOutboundMessages(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		SessionKey string `json:"session_key"`
		Platform   string `json:"platform"`
		Status     string `json:"status"`
		SourceType string `json:"source_type"`
		SourceID   string `json:"source_id"`
		SentAtFrom int64  `json:"sent_at_from"`
		SentAtTo   int64  `json:"sent_at_to"`
		Page       int    `json:"page"`
		PageSize   int    `json:"page_size"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	var offset int
	params.Page, params.PageSize, offset = normalizePage(params.Page, params.PageSize, 100)
	msgs, total, err := s.outboundMessageStore.ListFiltered(ctx, store.OutboundMessageFilter{
		SessionKey: params.SessionKey,
		Platform:   params.Platform,
		Status:     params.Status,
		SourceType: params.SourceType,
		SourceID:   params.SourceID,
		SentAtFrom: params.SentAtFrom,
		SentAtTo:   params.SentAtTo,
	}, params.PageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("list outbound messages: %w", err)
	}
	return pagedResult(msgs, total, params.Page, params.PageSize), nil
}
```

Add the dispatch case in `beeCallTool` switch, after `case utils.ListMessages:` (line 115–116):

```go
	case utils.ListMessages:
		return s.toolListMessages(ctx, args)
	case utils.ListOutboundMessages:
		return s.toolListOutboundMessages(ctx, args)
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/mcp/ -run TestCallTool_ListOutboundMessages -v
```

Expected: PASS.

- [ ] **Step 5: Run all MCP tests**

```bash
go test ./internal/mcp/ -v 2>&1 | tail -10
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "feat: add list_outbound_messages MCP tool"
```

---

### Task 5: CLI — `list-outbound` Subcommand

**Files:**
- Modify: `cmd/openbee/ctl_message.go`

- [ ] **Step 1: Add the flag variables and command**

In `cmd/openbee/ctl_message.go`, after the existing `var (msgListPage ... )` block (after line 27), add:

```go
var (
	msgListOutSessionKey string
	msgListOutPlatform   string
	msgListOutStatus     string
	msgListOutSourceType string
	msgListOutSourceID   string
	msgListOutSentFrom   int64
	msgListOutSentTo     int64
	msgListOutPage       int
	msgListOutPageSize   int
)

var ctlMessageListOutboundCmd = &cobra.Command{
	Use:   "list-outbound",
	Short: "List outbound (sent) messages with optional filters",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{}
		if msgListOutSessionKey != "" {
			a["session_key"] = msgListOutSessionKey
		}
		if msgListOutPlatform != "" {
			a["platform"] = msgListOutPlatform
		}
		if msgListOutStatus != "" {
			a["status"] = msgListOutStatus
		}
		if msgListOutSourceType != "" {
			a["source_type"] = msgListOutSourceType
		}
		if msgListOutSourceID != "" {
			a["source_id"] = msgListOutSourceID
		}
		if msgListOutSentFrom > 0 {
			a["sent_at_from"] = msgListOutSentFrom
		}
		if msgListOutSentTo > 0 {
			a["sent_at_to"] = msgListOutSentTo
		}
		if msgListOutPage > 0 {
			a["page"] = msgListOutPage
		}
		if msgListOutPageSize > 0 {
			a["page_size"] = msgListOutPageSize
		}
		return ctlRun(utils.ListOutboundMessages, a)
	},
}
```

- [ ] **Step 2: Register flags and subcommand in `init()`**

In the `init()` function, before `ctlMessageCmd.AddCommand(...)`, add:

```go
	ctlMessageListOutboundCmd.Flags().StringVar(&msgListOutSessionKey, "session-key", "", "Filter by session key")
	ctlMessageListOutboundCmd.Flags().StringVar(&msgListOutPlatform, "platform", "", "Filter by platform (e.g. feishu, local)")
	ctlMessageListOutboundCmd.Flags().StringVar(&msgListOutStatus, "status", "", "Filter by status (sent, failed)")
	ctlMessageListOutboundCmd.Flags().StringVar(&msgListOutSourceType, "source-type", "", "Filter by source type (bee, worker, system)")
	ctlMessageListOutboundCmd.Flags().StringVar(&msgListOutSourceID, "source-id", "", "Filter by source ID")
	ctlMessageListOutboundCmd.Flags().Int64Var(&msgListOutSentFrom, "sent-from", 0, "Filter sent_at >= value (Unix ms)")
	ctlMessageListOutboundCmd.Flags().Int64Var(&msgListOutSentTo, "sent-to", 0, "Filter sent_at <= value (Unix ms)")
	ctlMessageListOutboundCmd.Flags().IntVar(&msgListOutPage, "page", 0, "Page number (default: 1)")
	ctlMessageListOutboundCmd.Flags().IntVar(&msgListOutPageSize, "page-size", 0, "Page size (default: 50, max: 100)")
```

Change the last line of `init()`:
```go
	ctlMessageCmd.AddCommand(ctlMessageSendCmd, ctlMessageListCmd, ctlMessageListOutboundCmd)
```

- [ ] **Step 3: Build to verify no compile errors**

```bash
go build ./cmd/openbee/
```

Expected: builds with no errors.

- [ ] **Step 4: Smoke test — verify help text**

```bash
./openbee ctl message list-outbound --help
```

Expected output includes:
```
List outbound (sent) messages with optional filters

Usage:
  openbee ctl message list-outbound [flags]

Flags:
      --page int             Page number (default: 1)
      --page-size int        Page size (default: 50, max: 100)
      --platform string      Filter by platform (e.g. feishu, local)
      --sent-from int        Filter sent_at >= value (Unix ms)
      --sent-to int          Filter sent_at <= value (Unix ms)
      --session-key string   Filter by session key
      --source-id string     Filter by source ID
      --source-type string   Filter by source type (bee, worker, system)
      --status string        Filter by status (sent, failed)
```

Clean up the binary after testing:
```bash
rm -f openbee
```

- [ ] **Step 5: Commit**

```bash
git add cmd/openbee/ctl_message.go
git commit -m "feat: add ctl message list-outbound CLI command"
```

---

### Task 6: Final Verification

- [ ] **Step 1: Run all tests**

```bash
go test ./... 2>&1 | tail -20
```

Expected: all packages PASS, no failures.

- [ ] **Step 2: Commit if any cleanup was needed**

If no changes needed, skip. Otherwise:
```bash
git add -p
git commit -m "fix: address final review feedback"
```
