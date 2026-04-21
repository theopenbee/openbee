package store

import (
	"context"
	"fmt"
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

func TestMessageStore_UpdateStatusBatch(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "a", "", "", 0) //nolint
	s.Create(ctx, "msg-2", "feishu:chat1:userA", "feishu", "b", "", "", 0) //nolint

	if err := s.UpdateStatusBatch(ctx, []string{"msg-1", "msg-2"}, "debouncing"); err != nil {
		t.Fatalf("UpdateStatusBatch: %v", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT status FROM bee_platform_messages WHERE id IN (?, ?) ORDER BY id`,
		"msg-1", "msg-2",
	)
	if err != nil {
		t.Fatalf("query statuses: %v", err)
	}
	defer rows.Close()

	var statuses []string
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			t.Fatalf("scan status: %v", err)
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if len(statuses) != 2 || statuses[0] != "debouncing" || statuses[1] != "debouncing" {
		t.Fatalf("unexpected statuses: %v", statuses)
	}
}

func TestMessageStore_FetchMergedContent(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	primaryID := "primary-1"

	msgs := []BatchMsg{
		{
			ID: "merged-a", SessionKey: "s1", Platform: "test",
			Content: "image content", Raw: "", PlatformMsgID: "pmsg-a",
			MessageTime: now, Status: "merged", MergedInto: primaryID,
		},
		{
			ID: "merged-b", SessionKey: "s1", Platform: "test",
			Content: "second merged", Raw: "", PlatformMsgID: "pmsg-b",
			MessageTime: now + 1, Status: "merged", MergedInto: primaryID,
		},
		{
			ID: primaryID, SessionKey: "s1", Platform: "test",
			Content: "primary text", Raw: "", PlatformMsgID: "pmsg-c",
			MessageTime: now + 2, Status: "received", MergedInto: "",
		},
	}

	if _, err := s.CreateBatch(ctx, msgs); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	contents, err := s.FetchMergedContent(ctx, primaryID)
	if err != nil {
		t.Fatalf("FetchMergedContent: %v", err)
	}
	if len(contents) != 2 {
		t.Fatalf("expected 2 merged contents, got %d", len(contents))
	}
	if contents[0] != "image content" {
		t.Errorf("contents[0]: want %q, got %q", "image content", contents[0])
	}
	if contents[1] != "second merged" {
		t.Errorf("contents[1]: want %q, got %q", "second merged", contents[1])
	}

	// No merged content for a message without merges
	contents, err = s.FetchMergedContent(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("FetchMergedContent(nonexistent): %v", err)
	}
	if len(contents) != 0 {
		t.Errorf("expected 0 merged contents for nonexistent, got %d", len(contents))
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

func TestMessageStore_ClaimBatch_SkipsFeedingSession(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewMessageStore(db)
	ctx := context.Background()

	// Insert two messages for the same session; mark the first as feeding.
	now := time.Now().UnixMilli()
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m1', 'sk1', 'feishu', 'msg1', 'feeding', ?, ?, ?)`, now, now, now)
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m2', 'sk1', 'feishu', 'msg2', 'received', ?, ?, ?)`, now+1, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages (session already feeding), got %d", len(msgs))
	}
}

func TestMessageStore_ClaimBatch_OnePerSession(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewMessageStore(db)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	// Two messages for sk1 (different times), one for sk2.
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m1', 'sk1', 'feishu', 'first', 'received', ?, ?, ?)`, now, now, now)
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m2', 'sk1', 'feishu', 'second', 'received', ?, ?, ?)`, now+1, now, now)
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m3', 'sk2', 'feishu', 'other', 'received', ?, ?, ?)`, now, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (one per session), got %d", len(msgs))
	}
	ids := map[string]bool{}
	for _, m := range msgs {
		ids[m.ID] = true
	}
	if !ids["m1"] {
		t.Error("expected m1 (earliest for sk1) to be claimed, not m2")
	}
	if !ids["m3"] {
		t.Error("expected m3 (sk2) to be claimed")
	}
}

func TestMessageStore_ClaimBatch_RespectsLimit(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewMessageStore(db)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		sk := fmt.Sprintf("sk%d", i)
		id := fmt.Sprintf("m%d", i)
		db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
                  VALUES (?, ?, 'feishu', 'msg', 'received', ?, ?, ?)`, id, sk, now+int64(i), now, now)
	}

	msgs, err := s.ClaimBatch(ctx, 3)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages (limit), got %d", len(msgs))
	}
}

func TestMessageStore_MarkFailed(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "m1", "feishu:c:u", "feishu", "hello", "", "", 0) //nolint
	s.UpdateStatusBatch(ctx, []string{"m1"}, "feeding")              //nolint

	if err := s.MarkFailed(ctx, []string{"m1"}); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	var status string
	s.db.QueryRowContext(ctx, `SELECT status FROM bee_platform_messages WHERE id = 'm1'`).Scan(&status) //nolint
	if status != "failed" {
		t.Errorf("expected status=failed, got %q", status)
	}
}

func TestMessageStore_ClaimBatch_StalesLateArrivingMessage(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewMessageStore(db)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	// Message B arrived first and was processed (received_at = now+1000).
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgB', 'sk1', 'feishu', 'newer', 'bee_processed', ?, ?, ?)`, now+1000, now, now)
	// Message A arrives late (received_at = now, older than B).
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgA', 'sk1', 'feishu', 'older', 'received', ?, ?, ?)`, now, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 claimed messages (msgA is stale), got %d", len(msgs))
	}

	var status string
	if err := db.QueryRowContext(ctx,
		`SELECT status FROM bee_platform_messages WHERE id = 'msgA'`,
	).Scan(&status); err != nil {
		t.Fatalf("scan msgA status: %v", err)
	}
	if status != MsgStatusStale {
		t.Errorf("msgA: want status=%q, got %q", MsgStatusStale, status)
	}
}

func TestMessageStore_ClaimBatch_DoesNotStaleMessageWithNoNewerProcessed(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewMessageStore(db)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	// One received message, no bee_processed in the session — should be claimed normally.
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgA', 'sk1', 'feishu', 'hello', 'received', ?, ?, ?)`, now, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 claimed message, got %d", len(msgs))
	}
	if msgs[0].ID != "msgA" {
		t.Errorf("expected msgA to be claimed, got %q", msgs[0].ID)
	}
}

func TestMessageStore_ClaimBatch_DoesNotStaleMessageNewerThanProcessed(t *testing.T) {
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewMessageStore(db)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	// Message A processed (received_at = now), message B received (received_at = now+1000).
	// B is newer than A — it should be claimed, not staled.
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgA', 'sk1', 'feishu', 'older', 'bee_processed', ?, ?, ?)`, now, now, now)
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgB', 'sk1', 'feishu', 'newer', 'received', ?, ?, ?)`, now+1000, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 claimed message, got %d", len(msgs))
	}
	if msgs[0].ID != "msgB" {
		t.Errorf("expected msgB to be claimed, got %q", msgs[0].ID)
	}
}
