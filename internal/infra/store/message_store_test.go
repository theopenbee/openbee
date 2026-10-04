package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	require.EqualValues(t, 2, inserted)

	// Verify merged row
	var status, mergedInto string
	err = s.db.QueryRowContext(ctx,
		`SELECT status, merged_into FROM bee_platform_messages WHERE id = ?`, mergedID,
	).Scan(&status, &mergedInto)
	require.NoError(t, err)
	assert.Equal(t, "merged", status)
	assert.Equal(t, primaryID, mergedInto)

	// Verify primary row
	err = s.db.QueryRowContext(ctx,
		`SELECT status, merged_into FROM bee_platform_messages WHERE id = ?`, primaryID,
	).Scan(&status, &mergedInto)
	require.NoError(t, err)
	assert.Equal(t, "received", status)
	assert.Empty(t, mergedInto)
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
	require.NoError(t, err)
	require.EqualValues(t, 1, inserted)

	// Second insert with same platform_msg_id: INSERT OR IGNORE should skip it
	msg.ID = "id-2" // different row ID but same platform_msg_id
	inserted, err = s.CreateBatch(ctx, []BatchMsg{msg})
	require.NoError(t, err)
	require.EqualValues(t, 0, inserted, "duplicate ignored")
}

func TestMessageStore_CreateBatch_Empty(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	inserted, err := s.CreateBatch(ctx, nil)
	require.NoError(t, err)
	require.EqualValues(t, 0, inserted)
}

func setupMessageStore(t *testing.T) *MessageStore {
	t.Helper()
	return NewMessageStore(newTestDB(t))
}

func TestMessageStore_Create(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	_, err := s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "hello world", `{"text":"hello world"}`, "", 0)
	require.NoError(t, err)

	var raw string
	err = s.db.QueryRowContext(ctx, `SELECT raw FROM bee_platform_messages WHERE id = ?`, "msg-1").Scan(&raw)
	require.NoError(t, err)
	assert.Equal(t, `{"text":"hello world"}`, raw)
}

func TestMessageStore_UpdateStatusBatch(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "a", "", "", 0) //nolint
	s.Create(ctx, "msg-2", "feishu:chat1:userA", "feishu", "b", "", "", 0) //nolint

	err := s.UpdateStatusBatch(ctx, []string{"msg-1", "msg-2"}, "debouncing")
	require.NoError(t, err)

	rows, err := s.db.QueryContext(ctx,
		`SELECT status FROM bee_platform_messages WHERE id IN (?, ?) ORDER BY id`,
		"msg-1", "msg-2",
	)
	require.NoError(t, err)
	defer rows.Close()

	var statuses []string
	for rows.Next() {
		var status string
		require.NoError(t, rows.Scan(&status))
		statuses = append(statuses, status)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []string{"debouncing", "debouncing"}, statuses)
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

	_, err := s.CreateBatch(ctx, msgs)
	require.NoError(t, err)

	contents, err := s.FetchMergedContent(ctx, primaryID)
	require.NoError(t, err)
	require.Len(t, contents, 2)
	assert.Equal(t, "image content", contents[0])
	assert.Equal(t, "second merged", contents[1])

	// No merged content for a message without merges
	contents, err = s.FetchMergedContent(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, contents)
}

func TestMessageStore_Create_Dedup_DuplicatePlatformMsgID(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	inserted, err := s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "hello", "", "feishu-msg-abc", 0)
	require.NoError(t, err)
	assert.True(t, inserted, "first insert")

	inserted, err = s.Create(ctx, "msg-2", "feishu:chat1:userA", "feishu", "hello", "", "feishu-msg-abc", 0)
	require.NoError(t, err)
	assert.False(t, inserted, "duplicate insert")
}

func TestMessageStore_Create_Dedup_EmptyPlatformMsgIDNotDeduped(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	inserted1, err := s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "hello", "", "", 0)
	require.NoError(t, err)
	require.True(t, inserted1)

	inserted2, err := s.Create(ctx, "msg-2", "feishu:chat1:userA", "feishu", "hello", "", "", 0)
	require.NoError(t, err)
	require.True(t, inserted2)
}

func TestMessageStore_Create_ReceivedAtMillisecondPrecision(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "msg-ms", "feishu:chat1:userA", "feishu", "hello", "", "", 0) //nolint

	var receivedAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT received_at FROM bee_platform_messages WHERE id = ?`, "msg-ms",
	).Scan(&receivedAt)
	require.NoError(t, err)
	assert.Positive(t, receivedAt, "want positive Unix millisecond timestamp")
}

func TestMessageStore_Create_ReceivedAt_FromMessageTime(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	const wantTime int64 = 1609073151345 // fixed past timestamp
	inserted, err := s.Create(ctx, "msg-ts", "feishu:chat1:userA", "feishu", "hello", "", "", wantTime)
	require.NoError(t, err)
	require.True(t, inserted)

	var receivedAt int64
	err = s.db.QueryRowContext(ctx,
		`SELECT received_at FROM bee_platform_messages WHERE id = ?`, "msg-ts",
	).Scan(&receivedAt)
	require.NoError(t, err)
	assert.Equal(t, wantTime, receivedAt)
}

func TestMessageStore_Create_ReceivedAt_FallbackToServerTime(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	before := time.Now().UnixMilli()
	s.Create(ctx, "msg-zero", "feishu:chat1:userA", "feishu", "hello", "", "", 0) //nolint
	after := time.Now().UnixMilli()

	var receivedAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT received_at FROM bee_platform_messages WHERE id = ?`, "msg-zero",
	).Scan(&receivedAt)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, receivedAt, before)
	assert.LessOrEqual(t, receivedAt, after)
}

func TestMessageStore_GetByID_ReturnsStoredFields(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "msg-1", "feishu:chat1:userA", "feishu", "hello", `{"raw":"data"}`, "", 0) //nolint

	got, err := s.GetByID(ctx, "msg-1")
	require.NoError(t, err)
	assert.Equal(t, "feishu", got.Platform)
	assert.Equal(t, "feishu:chat1:userA", got.SessionKey)
	assert.Equal(t, `{"raw":"data"}`, got.Raw)
}

func TestMessageStore_GetByID_NotFound(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	_, err := s.GetByID(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestMessageStore_ClaimBatch_SkipsFeedingSession(t *testing.T) {
	db := newTestDB(t)
	s := NewMessageStore(db)
	ctx := context.Background()

	// Insert two messages for the same session; mark the first as feeding.
	now := time.Now().UnixMilli()
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m1', 'sk1', 'feishu', 'msg1', 'feeding', ?, ?, ?)`, now, now, now)
	db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m2', 'sk1', 'feishu', 'msg2', 'received', ?, ?, ?)`, now+1, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, msgs, "expected 0 messages (session already feeding)")
}

func TestMessageStore_ClaimBatch_OnePerSession(t *testing.T) {
	db := newTestDB(t)
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
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	ids := map[string]bool{}
	for _, m := range msgs {
		ids[m.ID] = true
	}
	assert.True(t, ids["m1"], "expected m1 (earliest for sk1) to be claimed, not m2")
	assert.True(t, ids["m3"], "expected m3 (sk2) to be claimed")
}

func TestMessageStore_ClaimBatch_RespectsLimit(t *testing.T) {
	db := newTestDB(t)
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
	require.NoError(t, err)
	assert.Len(t, msgs, 3)
}

func TestMessageStore_MarkFailed(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	s.Create(ctx, "m1", "feishu:c:u", "feishu", "hello", "", "", 0) //nolint
	s.UpdateStatusBatch(ctx, []string{"m1"}, "feeding")             //nolint

	require.NoError(t, s.MarkFailed(ctx, []string{"m1"}))

	var status string
	s.db.QueryRowContext(ctx, `SELECT status FROM bee_platform_messages WHERE id = 'm1'`).Scan(&status) //nolint
	assert.Equal(t, "failed", status)
}

func TestMessageStore_ClaimBatch_StalesLateArrivingMessage(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	s.db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgB', 'sk1', 'feishu', 'newer', 'bee_processed', ?, ?, ?)`, now+1000, now, now)
	s.db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgA', 'sk1', 'feishu', 'older', 'received', ?, ?, ?)`, now, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, msgs, "expected 0 claimed messages (msgA is stale)")

	var status string
	err = s.db.QueryRowContext(ctx,
		`SELECT status FROM bee_platform_messages WHERE id = 'msgA'`,
	).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, MsgStatusStale, status)
}

func TestMessageStore_ClaimBatch_DoesNotStaleMessageWithNoNewerProcessed(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	s.db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgA', 'sk1', 'feishu', 'hello', 'received', ?, ?, ?)`, now, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "msgA", msgs[0].ID)
}

func TestMessageStore_ClaimBatch_DoesNotStaleMessageNewerThanProcessed(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	// B is newer than A — it should be claimed, not staled.
	s.db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgA', 'sk1', 'feishu', 'older', 'bee_processed', ?, ?, ?)`, now, now, now)
	s.db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
	          VALUES ('msgB', 'sk1', 'feishu', 'newer', 'received', ?, ?, ?)`, now+1000, now, now)

	msgs, err := s.ClaimBatch(ctx, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "msgB", msgs[0].ID)
}

func TestMessageStore_FailReceived(t *testing.T) {
	s := setupMessageStore(t)
	ctx := context.Background()

	insert := func(id, sessionKey, status string) {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO bee_platform_messages (id, session_key, platform, content, raw, received_at, status, created_at, updated_at)
             VALUES (?, ?, 'test', 'x', '', 0, ?, 0, 0)`,
			id, sessionKey, status)
		require.NoError(t, err)
	}
	insert("msg-a1", "sessionA", MsgStatusReceived)
	insert("msg-a2", "sessionA", MsgStatusReceived)
	insert("msg-b1", "sessionB", MsgStatusReceived)
	insert("msg-a3", "sessionA", MsgStatusFeeding)

	ids, err := s.FailReceived(ctx, "sessionA")
	require.NoError(t, err)
	require.Len(t, ids, 2)
	got := map[string]bool{ids[0]: true, ids[1]: true}
	assert.True(t, got["msg-a1"], "expected msg-a1 in %v", ids)
	assert.True(t, got["msg-a2"], "expected msg-a2 in %v", ids)

	for _, id := range []string{"msg-a1", "msg-a2"} {
		var status string
		s.db.QueryRowContext(ctx, `SELECT status FROM bee_platform_messages WHERE id = ?`, id).Scan(&status) //nolint
		assert.Equal(t, MsgStatusFailed, status, "id=%s", id)
	}

	for id, want := range map[string]string{"msg-b1": MsgStatusReceived, "msg-a3": MsgStatusFeeding} {
		var status string
		s.db.QueryRowContext(ctx, `SELECT status FROM bee_platform_messages WHERE id = ?`, id).Scan(&status) //nolint
		assert.Equal(t, want, status, "id=%s", id)
	}
}
