package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// TestMessageStore_AccountName_PersistAndFilter covers persistence + filter on
// account_name for the MessageStore.
func TestMessageStore_AccountName_PersistAndFilter(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	if _, err := s.Create(ctx, "m-mk", "feishu:marketing:c1", "feishu", "marketing", "hello", "", "", 0); err != nil {
		t.Fatalf("Create marketing: %v", err)
	}
	if _, err := s.Create(ctx, "m-sup", "feishu:support:c1", "feishu", "support", "hi", "", "", 0); err != nil {
		t.Fatalf("Create support: %v", err)
	}

	// Read back via GetByID
	got, err := s.GetByID(ctx, "m-mk")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.AccountName != "marketing" {
		t.Errorf("GetByID account: want marketing, got %q", got.AccountName)
	}

	// Filter for marketing
	msgs, total, err := s.ListFiltered(ctx, MessageFilter{AccountName: "marketing"}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered marketing: %v", err)
	}
	if total != 1 || len(msgs) != 1 || msgs[0].ID != "m-mk" {
		t.Fatalf("filter marketing: want [m-mk], got total=%d msgs=%+v", total, msgs)
	}
	if msgs[0].AccountName != "marketing" {
		t.Errorf("listed account: want marketing, got %q", msgs[0].AccountName)
	}

	// Filter for support
	msgs, total, err = s.ListFiltered(ctx, MessageFilter{AccountName: "support"}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered support: %v", err)
	}
	if total != 1 || msgs[0].ID != "m-sup" {
		t.Fatalf("filter support: want [m-sup], got total=%d msgs=%+v", total, msgs)
	}

	// Filter for a non-existent account returns nothing
	_, total, err = s.ListFiltered(ctx, MessageFilter{AccountName: "nope"}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered nope: %v", err)
	}
	if total != 0 {
		t.Errorf("filter nope: want 0 rows, got %d", total)
	}
}

// TestMessageStore_CreateBatch_AccountName ensures CreateBatch persists account_name.
func TestMessageStore_CreateBatch_AccountName(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	_, err := s.CreateBatch(ctx, []BatchMsg{
		{ID: "b1", SessionKey: "sk-a", Platform: "feishu", AccountName: "marketing",
			Content: "x", Status: "received", MessageTime: 1000},
		{ID: "b2", SessionKey: "sk-b", Platform: "feishu", AccountName: "support",
			Content: "y", Status: "received", MessageTime: 1001},
	})
	if err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	msgs, total, err := s.ListFiltered(ctx, MessageFilter{AccountName: "marketing"}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 1 || msgs[0].ID != "b1" {
		t.Errorf("expected b1, got total=%d msgs=%+v", total, msgs)
	}
}

// TestOutboundMessageStore_AccountName_PersistAndFilter covers persistence +
// filter on account_name for OutboundMessageStore.
func TestOutboundMessageStore_AccountName_PersistAndFilter(t *testing.T) {
	s := setupOutboundStore(t)
	ctx := context.Background()
	seedOutbound(t, s, []OutboundMessage{
		{ID: "om-mk", SessionKey: "sk1", Platform: "feishu", AccountName: "marketing", Status: OutboundStatusSent, SentAt: 1000},
		{ID: "om-sup", SessionKey: "sk2", Platform: "feishu", AccountName: "support", Status: OutboundStatusSent, SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(ctx, OutboundMessageFilter{AccountName: "marketing"}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 1 || msgs[0].ID != "om-mk" {
		t.Fatalf("want om-mk, got total=%d msgs=%+v", total, msgs)
	}
	if msgs[0].AccountName != "marketing" {
		t.Errorf("listed account: want marketing, got %q", msgs[0].AccountName)
	}

	// Read back full row via ListBySessionKey to confirm AccountName is scanned.
	full, err := s.ListBySessionKey(ctx, "sk1", 0, 10)
	if err != nil {
		t.Fatalf("ListBySessionKey: %v", err)
	}
	if len(full) != 1 || full[0].AccountName != "marketing" {
		t.Errorf("ListBySessionKey: got %+v", full)
	}
}

// TestTaskStore_AccountName_PersistAndFilter exercises model.Task.AccountName +
// TaskFilter.AccountName + ClaimedTask.MessageAccountName.
func TestTaskStore_AccountName_PersistAndFilter(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Seed two messages with distinct account_name; tasks inherit theirs.
	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','W','/','idle',1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages
		(id,session_key,platform,account_name,content,raw,platform_msg_id,received_at,created_at,updated_at)
		VALUES ('m-mk','feishu:marketing:c','feishu','marketing','x','','',1,1,1)`)
	db.Exec(`INSERT INTO bee_platform_messages
		(id,session_key,platform,account_name,content,raw,platform_msg_id,received_at,created_at,updated_at)
		VALUES ('m-sup','feishu:support:c','feishu','support','y','','',1,1,1)`)

	ts := NewTaskStore(db)
	ctx := context.Background()

	idMk, err := ts.Create(ctx, model.Task{
		MessageID: "m-mk", WorkerID: "w1", Instruction: "mk",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		AccountName: "marketing",
		CreatedAt:   1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatalf("Create mk task: %v", err)
	}
	idSup, err := ts.Create(ctx, model.Task{
		MessageID: "m-sup", WorkerID: "w1", Instruction: "sup",
		Type: model.TaskTypeImmediate, Status: model.TaskStatusPending,
		AccountName: "support",
		CreatedAt:   1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatalf("Create sup task: %v", err)
	}

	// Read-back via GetByID
	got, err := ts.GetByID(ctx, idMk)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.AccountName != "marketing" {
		t.Errorf("task account: want marketing, got %q", got.AccountName)
	}

	// Filter by account
	tasks, err := ts.List(ctx, TaskFilter{AccountName: "marketing"})
	if err != nil {
		t.Fatalf("List marketing: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != idMk {
		t.Errorf("List marketing: want [%s], got %+v", idMk, tasks)
	}

	tasks, err = ts.List(ctx, TaskFilter{AccountName: "support"})
	if err != nil {
		t.Fatalf("List support: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != idSup {
		t.Errorf("List support: want [%s], got %+v", idSup, tasks)
	}

	// ClaimDueTasks: surfaces MessageAccountName.
	claimed, err := ts.ClaimDueTasks(ctx, 999999999, nil)
	if err != nil {
		t.Fatalf("ClaimDueTasks: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("want 2 claimed, got %d", len(claimed))
	}
	byID := map[string]model.ClaimedTask{}
	for _, c := range claimed {
		byID[c.ID] = c
	}
	if byID[idMk].MessageAccountName != "marketing" {
		t.Errorf("claimed mk: MessageAccountName want marketing, got %q", byID[idMk].MessageAccountName)
	}
	if byID[idSup].MessageAccountName != "support" {
		t.Errorf("claimed sup: MessageAccountName want support, got %q", byID[idSup].MessageAccountName)
	}
	if byID[idMk].AccountName != "marketing" {
		t.Errorf("claimed mk task.AccountName want marketing, got %q", byID[idMk].AccountName)
	}
}

// TestExecutionStore_AccountName_PersistAndFilter covers Create + ExecutionFilter.
func TestExecutionStore_AccountName_PersistAndFilter(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Exec(`INSERT INTO bee_workers (id,name,work_dir,status,created_at,updated_at) VALUES ('w1','W','/','idle',0,0)`)
	es := NewExecutionStore(db, t.TempDir())

	execMk, err := es.Create("w1", "mk", uuid.New().String(), "marketing", "claude")
	if err != nil {
		t.Fatalf("Create marketing: %v", err)
	}
	execSup, err := es.Create("w1", "sup", uuid.New().String(), "support", "claude")
	if err != nil {
		t.Fatalf("Create support: %v", err)
	}

	got, err := es.GetByID(execMk.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.AccountName != "marketing" {
		t.Errorf("AccountName: want marketing, got %q", got.AccountName)
	}

	ctx := context.Background()
	rows, total, err := es.ListFiltered(ctx, ExecutionFilter{AccountName: "marketing"}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 1 || rows[0].ID != execMk.ID {
		t.Fatalf("want only marketing exec, got total=%d rows=%+v", total, rows)
	}

	rows, total, err = es.ListFiltered(ctx, ExecutionFilter{AccountName: "support"}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered support: %v", err)
	}
	if total != 1 || rows[0].ID != execSup.ID {
		t.Fatalf("want only support exec, got total=%d rows=%+v", total, rows)
	}
}

// TestSessionStore_AccountName_PersistAndList exercises UpsertSessionContext +
// ListSessionContexts to confirm account_name round-trip.
func TestSessionStore_AccountName_PersistAndList(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ss := NewSessionStore(db)
	ctx := context.Background()

	if err := ss.UpsertSessionContext(ctx, "feishu:marketing:c", BeeAgentID, "sess-mk", "marketing", "claude"); err != nil {
		t.Fatalf("Upsert marketing: %v", err)
	}
	if err := ss.UpsertSessionContext(ctx, "feishu:support:c", BeeAgentID, "sess-sup", "support", "claude"); err != nil {
		t.Fatalf("Upsert support: %v", err)
	}

	got, err := ss.ListSessionContexts(ctx, "feishu:marketing:c")
	if err != nil {
		t.Fatalf("ListSessionContexts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 row, got %d", len(got))
	}
	if got[0].AccountName != "marketing" {
		t.Errorf("AccountName: want marketing, got %q", got[0].AccountName)
	}

	// Verify support session is isolated and also carries its account name.
	got, err = ss.ListSessionContexts(ctx, "feishu:support:c")
	if err != nil {
		t.Fatalf("ListSessionContexts support: %v", err)
	}
	if len(got) != 1 || got[0].AccountName != "support" {
		t.Errorf("support session: want AccountName=support, got %+v", got)
	}
}
