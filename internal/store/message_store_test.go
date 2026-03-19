package store

import (
	"context"
	"testing"
	"time"
)

func TestMessageStore_CreateBatch(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	primaryID := "primary-1"
	mergedID := "merged-1"

	msgs := []BatchMsg{
		{
			ID: mergedID, SessionKey: "s1", Platform: "test",
			Content: "first", Raw: "", PlatformMsgID: "pmsg-1",
			MessageTime: now, Status: "merged", MergedInto: primaryID,
		},
		{
			ID: primaryID, SessionKey: "s1", Platform: "test",
			Content: "first\n\n---\n\nsecond", Raw: "", PlatformMsgID: "pmsg-2",
			MessageTime: now, Status: "received", MergedInto: "",
		},
	}

	inserted, err := s.CreateBatch(ctx, msgs)
	if err != nil {
		t.Fatalf("CreateBatch error: %v", err)
	}
	if inserted != 2 {
		t.Fatalf("expected 2 rows inserted, got %d", inserted)
	}

	// Verify merged row
	var status, mergedInto string
	if err := s.db.QueryRowContext(ctx,
		`SELECT status, merged_into FROM bee_platform_messages WHERE id = ?`, mergedID,
	).Scan(&status, &mergedInto); err != nil {
		t.Fatalf("scan merged row: %v", err)
	}
	if status != "merged" {
		t.Errorf("merged row: want status=merged, got %q", status)
	}
	if mergedInto != primaryID {
		t.Errorf("merged row: want merged_into=%q, got %q", primaryID, mergedInto)
	}

	// Verify primary row
	if err := s.db.QueryRowContext(ctx,
		`SELECT status, merged_into FROM bee_platform_messages WHERE id = ?`, primaryID,
	).Scan(&status, &mergedInto); err != nil {
		t.Fatalf("scan primary row: %v", err)
	}
	if status != "received" {
		t.Errorf("primary row: want status=received, got %q", status)
	}
	if mergedInto != "" {
		t.Errorf("primary row: want merged_into empty, got %q", mergedInto)
	}
}

func TestMessageStore_CreateBatch_DuplicateIgnored(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	msg := BatchMsg{
		ID: "id-1", SessionKey: "s1", Platform: "test",
		Content: "hello", Raw: "", PlatformMsgID: "pmsg-dup",
		MessageTime: time.Now().UnixMilli(), Status: "received", MergedInto: "",
	}

	// First insert: should succeed
	inserted, err := s.CreateBatch(ctx, []BatchMsg{msg})
	if err != nil {
		t.Fatalf("first CreateBatch error: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("expected 1 row inserted, got %d", inserted)
	}

	// Second insert with same platform_msg_id: INSERT OR IGNORE should skip it
	msg.ID = "id-2" // different row ID but same platform_msg_id
	inserted, err = s.CreateBatch(ctx, []BatchMsg{msg})
	if err != nil {
		t.Fatalf("second CreateBatch error: %v", err)
	}
	if inserted != 0 {
		t.Fatalf("expected 0 rows inserted (duplicate ignored), got %d", inserted)
	}
}

func TestMessageStore_CreateBatch_Empty(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	inserted, err := s.CreateBatch(ctx, nil)
	if err != nil {
		t.Fatalf("CreateBatch(nil) error: %v", err)
	}
	if inserted != 0 {
		t.Fatalf("expected 0 rows inserted for empty batch, got %d", inserted)
	}
}

func setupMessageStore(t *testing.T) *MessageStore {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewMessageStore(db)
}

func TestMessageStore_Create(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	if _, err := s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "hello world", `{"text":"hello world"}`, "", 0); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT raw FROM bee_platform_messages WHERE id = ?`, "msg-1").Scan(&raw); err != nil {
		t.Fatalf("query raw: %v", err)
	}
	if raw != `{"text":"hello world"}` {
		t.Errorf("raw: got %q, want %q", raw, `{"text":"hello world"}`)
	}
}

func TestMessageStore_SetStatus(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "hello", "", "", 0) //nolint
	if err := s.SetStatus(ctx, "msg-1", "debouncing"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
}

func TestMessageStore_UpdateStatusBatch(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "a", "", "", 0) //nolint
	s.Create(ctx, "msg-2", "feishu:chat1:userA", "feishu", "b", "", "", 0) //nolint

	if err := s.UpdateStatusBatch(ctx, []string{"msg-1", "msg-2"}, "debouncing"); err != nil {
		t.Fatalf("UpdateStatusBatch: %v", err)
	}
}

func TestMessageStore_MarkMerged(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "msg1", "", "", 0) //nolint
	s.Create(ctx, "msg-2", "feishu:chat1:userA", "feishu", "msg2", "", "", 0) //nolint
	s.Create(ctx, "msg-3", "feishu:chat1:userA", "feishu", "msg3", "", "", 0) //nolint

	if err := s.MarkMerged(ctx, "msg-1", []string{"msg-2", "msg-3"}); err != nil {
		t.Fatalf("MarkMerged: %v", err)
	}
}

func TestMessageStore_Create_Dedup_FirstInsertReturnsTrue(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	inserted, err := s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "hello", "", "feishu-msg-abc", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !inserted {
		t.Error("first insert: want inserted=true, got false")
	}
}

func TestMessageStore_Create_Dedup_DuplicatePlatformMsgID(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "hello", "", "feishu-msg-abc", 0) //nolint
	inserted, err := s.Create(ctx, "msg-2", "feishu:chat1:userA", "feishu", "hello", "", "feishu-msg-abc", 0)
	if err != nil {
		t.Fatalf("duplicate Create: %v", err)
	}
	if inserted {
		t.Error("duplicate insert: want inserted=false, got true")
	}
}

func TestMessageStore_Create_Dedup_EmptyPlatformMsgIDNotDeduped(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	inserted1, err := s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "hello", "", "", 0)
	if err != nil || !inserted1 {
		t.Fatalf("first empty-id insert: err=%v inserted=%v", err, inserted1)
	}
	inserted2, err := s.Create(ctx, "msg-2", "feishu:chat1:userA", "feishu", "hello", "", "", 0)
	if err != nil || !inserted2 {
		t.Fatalf("second empty-id insert: err=%v inserted=%v", err, inserted2)
	}
}

func TestMessageStore_Create_ReceivedAtMillisecondPrecision(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "msg-ms", "feishu:chat1:userA", "feishu", "hello", "", "", 0) //nolint

	var receivedAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT received_at FROM bee_platform_messages WHERE id = ?`, "msg-ms",
	).Scan(&receivedAt)
	if err != nil {
		t.Fatalf("scan received_at: %v", err)
	}
	if receivedAt <= 0 {
		t.Errorf("received_at %d: want positive Unix millisecond timestamp", receivedAt)
	}
}

func TestMessageStore_Create_ReceivedAt_FromMessageTime(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	const wantTime int64 = 1609073151345 // fixed past timestamp
	inserted, err := s.Create(ctx, "msg-ts", "feishu:chat1:userA", "feishu", "hello", "", "", wantTime)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !inserted {
		t.Fatal("expected inserted=true")
	}

	var receivedAt int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT received_at FROM bee_platform_messages WHERE id = ?`, "msg-ts",
	).Scan(&receivedAt); err != nil {
		t.Fatalf("scan received_at: %v", err)
	}
	if receivedAt != wantTime {
		t.Errorf("received_at: got %d, want %d", receivedAt, wantTime)
	}
}

func TestMessageStore_Create_ReceivedAt_FallbackToServerTime(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	before := time.Now().UnixMilli()
	s.Create(ctx, "msg-zero", "feishu:chat1:userA", "feishu", "hello", "", "", 0) //nolint
	after := time.Now().UnixMilli()

	var receivedAt int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT received_at FROM bee_platform_messages WHERE id = ?`, "msg-zero",
	).Scan(&receivedAt); err != nil {
		t.Fatalf("scan received_at: %v", err)
	}
	if receivedAt < before || receivedAt > after {
		t.Errorf("received_at %d: want value between %d and %d (server time range)", receivedAt, before, after)
	}
}

func TestMessageStore_GetByID_ReturnsStoredFields(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "hello", `{"raw":"data"}`, "", 0) //nolint

	got, err := s.GetByID(ctx, "msg-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Platform != "feishu" {
		t.Errorf("Platform: want feishu, got %q", got.Platform)
	}
	if got.SessionKey != "feishu:chat1:userA" {
		t.Errorf("SessionKey: want feishu:chat1:userA, got %q", got.SessionKey)
	}
	if got.Raw != `{"raw":"data"}` {
		t.Errorf("Raw: want %q, got %q", `{"raw":"data"}`, got.Raw)
	}
}

func TestMessageStore_GetByID_NotFound(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	_, err := s.GetByID(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for missing message, got nil")
	}
}
