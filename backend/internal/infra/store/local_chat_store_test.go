package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedInbound(t *testing.T, db *sql.DB, id, sessionKey, senderType, status string, receivedAt int64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO bee_platform_messages
		(id, session_key, platform, content, raw, platform_msg_id, status, sender_type, received_at, created_at, updated_at)
		VALUES (?, ?, 'local', ?, '', '', ?, ?, ?, 1, 1)`,
		id, sessionKey, "in-"+id, status, senderType, receivedAt)
	require.NoError(t, err)
}

func seedOutboundRow(t *testing.T, db *sql.DB, id, sessionKey, sourceType, sourceID, kind, status string, sentAt int64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO bee_outbound_messages
		(id, session_key, platform, content, status, source_type, source_id, kind, sent_at, created_at)
		VALUES (?, ?, 'local', ?, ?, ?, ?, COALESCE(NULLIF(?, ''), 'message'), ?, 1)`,
		id, sessionKey, "out-"+id, status, sourceType, sourceID, kind, sentAt)
	require.NoError(t, err)
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(query, args...).Scan(&n))
	return n
}

func TestLastMessagePicksNewestAcrossTables(t *testing.T) {
	db := newTestDB(t)
	s := NewLocalChatStore(db)
	ctx := context.Background()
	key := "local:u1:worker:w1"

	_, ok, err := s.LastMessage(ctx, key)
	require.NoError(t, err)
	assert.False(t, ok)

	seedInbound(t, db, "i1", key, SenderTypeUser, MsgStatusReceived, 100)
	last, ok, err := s.LastMessage(ctx, key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, LastMessage{SenderType: SenderTypeUser, Kind: OutboundKindMessage, Content: "in-i1", TS: 100}, last)

	seedOutboundRow(t, db, "o1", key, "", "", "", OutboundStatusSent, 200)
	last, ok, err = s.LastMessage(ctx, key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, LastMessage{SenderType: SenderTypeBee, Kind: OutboundKindMessage, Content: "out-o1", TS: 200}, last)

	seedInbound(t, db, "i2", key, SenderTypeBee, MsgStatusBeeProcessed, 300)
	last, ok, err = s.LastMessage(ctx, key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, LastMessage{SenderType: SenderTypeBee, Kind: OutboundKindMessage, Content: "in-i2", TS: 300}, last)

	seedOutboundRow(t, db, "o2", key, SourceTypeWorker, "w1", OutboundKindHandoff, OutboundStatusSent, 400)
	_, err = db.Exec(`UPDATE bee_outbound_messages SET payload = ? WHERE id = ?`, `{"worker_id":"w9"}`, "o2")
	require.NoError(t, err)
	seedOutboundRow(t, db, "o3", key, SourceTypeWorker, "w1", "", OutboundStatusFailed, 500)
	seedInbound(t, db, "i3", key, SenderTypeUser, MsgStatusMerged, 600)
	seedInbound(t, db, "i4", "local:u1:worker:w2", SenderTypeUser, MsgStatusReceived, 700)
	last, ok, err = s.LastMessage(ctx, key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, LastMessage{SenderType: SourceTypeWorker, SenderID: "w1", Kind: OutboundKindHandoff, Content: "out-o2", Payload: `{"worker_id":"w9"}`, TS: 400}, last)
}

func TestLastMessageSameTimestampPrefersReply(t *testing.T) {
	db := newTestDB(t)
	s := NewLocalChatStore(db)
	key := "local:u1"
	seedInbound(t, db, "i1", key, SenderTypeUser, MsgStatusReceived, 100)
	seedOutboundRow(t, db, "o1", key, SourceTypeBee, "", "", OutboundStatusSent, 100)

	last, ok, err := s.LastMessage(context.Background(), key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "out-o1", last.Content)
}

func TestLastMessageSkipsReset(t *testing.T) {
	db := newTestDB(t)
	s := NewLocalChatStore(db)
	key := "local:u1:worker:w1"
	seedInbound(t, db, "i1", key, SenderTypeUser, MsgStatusReceived, 100)
	seedOutboundRow(t, db, "o1", key, SourceTypeSystem, "", OutboundKindReset, OutboundStatusSent, 200)

	last, ok, err := s.LastMessage(context.Background(), key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "in-i1", last.Content)
}

func TestUnreadCountSkipsReset(t *testing.T) {
	db := newTestDB(t)
	s := NewLocalChatStore(db)
	key := "local:u1:worker:w1"
	seedOutboundRow(t, db, "o1", key, SourceTypeWorker, "w1", "", OutboundStatusSent, 100)
	seedOutboundRow(t, db, "o2", key, SourceTypeSystem, "", OutboundKindReset, OutboundStatusSent, 200)

	n, err := s.UnreadCount(context.Background(), key, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestLastKind(t *testing.T) {
	db := newTestDB(t)
	s := NewLocalChatStore(db)
	ctx := context.Background()
	key := "local:u1:worker:w1"

	_, ok, err := s.LastKind(ctx, key)
	require.NoError(t, err)
	assert.False(t, ok)

	seedInbound(t, db, "i1", key, SenderTypeUser, MsgStatusReceived, 100)
	kind, ok, err := s.LastKind(ctx, key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, OutboundKindMessage, kind)

	seedOutboundRow(t, db, "o1", key, SourceTypeSystem, "", OutboundKindReset, OutboundStatusSent, 200)
	seedInbound(t, db, "i2", "local:u1:worker:w2", SenderTypeUser, MsgStatusReceived, 300)
	kind, ok, err = s.LastKind(ctx, key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, OutboundKindReset, kind)

	seedOutboundRow(t, db, "o2", key, SourceTypeWorker, "w1", "", OutboundStatusFailed, 400)
	kind, ok, err = s.LastKind(ctx, key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, OutboundKindMessage, kind)

	seedOutboundRow(t, db, "o3", key, SourceTypeSystem, "", OutboundKindReset, OutboundStatusSent, 500)
	seedInbound(t, db, "i3", key, SenderTypeUser, MsgStatusCommand, 600)
	kind, ok, err = s.LastKind(ctx, key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, OutboundKindMessage, kind)
}

func TestUnreadCountCountsBeeInboundAndOutbound(t *testing.T) {
	db := newTestDB(t)
	s := NewLocalChatStore(db)
	ctx := context.Background()
	key := "local:u1:worker:w1"

	seedInbound(t, db, "i-user", key, SenderTypeUser, MsgStatusReceived, 250)
	seedInbound(t, db, "i-bee-old", key, SenderTypeBee, MsgStatusBeeProcessed, 150)
	seedInbound(t, db, "i-bee", key, SenderTypeBee, MsgStatusBeeProcessed, 250)
	seedOutboundRow(t, db, "o-old", key, SourceTypeWorker, "w1", "", OutboundStatusSent, 100)
	seedOutboundRow(t, db, "o-new", key, SourceTypeWorker, "w1", "", OutboundStatusSent, 300)
	seedOutboundRow(t, db, "o-failed", key, SourceTypeWorker, "w1", "", OutboundStatusFailed, 350)
	seedOutboundRow(t, db, "o-boundary", key, SourceTypeWorker, "w1", "", OutboundStatusSent, 200)
	seedOutboundRow(t, db, "o-other", "local:u1:worker:w2", SourceTypeWorker, "w2", "", OutboundStatusSent, 300)

	n, err := s.UnreadCount(ctx, key, 200)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	n, err = s.UnreadCount(ctx, key, 0)
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	n, err = s.UnreadCount(ctx, key, 1000)
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestMarkReadKeepsMax(t *testing.T) {
	s := NewLocalChatStore(newTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.MarkRead(ctx, "u1", "bee", 200))
	require.NoError(t, s.MarkRead(ctx, "u1", "bee", 100))
	require.NoError(t, s.MarkRead(ctx, "u1", "worker:w1", 50))
	require.NoError(t, s.MarkRead(ctx, "u2", "bee", 999))

	marks, err := s.ReadMarks(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{"bee": 200, "worker:w1": 50}, marks)

	require.NoError(t, s.MarkRead(ctx, "u1", "bee", 300))
	marks, err = s.ReadMarks(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, int64(300), marks["bee"])

	marks, err = s.ReadMarks(ctx, "nobody")
	require.NoError(t, err)
	assert.Empty(t, marks)
}

func TestWorkerChatSessionKeys(t *testing.T) {
	db := newTestDB(t, seedWorkerSQL("w1"), seedMessageSQL("m1", "local:u1"))
	s := NewLocalChatStore(db)
	ctx := context.Background()

	seedInbound(t, db, "i1", "local:u1:worker:w1", SenderTypeUser, MsgStatusReceived, 1)
	seedInbound(t, db, "i2", "local:u1:worker:w1", SenderTypeUser, MsgStatusReceived, 2)
	seedInbound(t, db, "i3", "local:u1:worker:xw1", SenderTypeUser, MsgStatusReceived, 3)
	seedInbound(t, db, "i4", "local:u1", SenderTypeUser, MsgStatusReceived, 4)
	seedOutboundRow(t, db, "o1", "local:u2:worker:w1", SourceTypeWorker, "w1", "", OutboundStatusSent, 5)
	for _, q := range []string{
		`INSERT INTO bee_tasks (id, message_id, worker_id, instruction, type, status, session_key, platform, created_at, updated_at)
		 VALUES ('t1','m1','w1','go','countdown','pending','local:u3:worker:w1','local',1,1)`,
		`INSERT INTO bee_session_contexts (session_key, agent_id, engine, session_id, updated_at)
		 VALUES ('local:u4:worker:w1','w1','claude','s',1), ('local:u1:worker:w1','w1','claude','s',1), ('telegram:1:worker:w1','w1','claude','s',1)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}

	keys, err := s.WorkerChatSessionKeys(ctx, "w1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"local:u1:worker:w1", "local:u2:worker:w1", "local:u3:worker:w1", "local:u4:worker:w1",
	}, keys)

	keys, err = s.WorkerChatSessionKeys(ctx, "w9")
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func TestDeleteWorkerChats(t *testing.T) {
	db := newTestDB(t, seedWorkerSQL("w1"), seedWorkerSQL("w2"), seedMessageSQL("m1", "local:u9"))
	s := NewLocalChatStore(db)
	ctx := context.Background()
	w1, w2, bee := "local:u1:worker:w1", "local:u1:worker:w2", "local:u1"

	for _, key := range []string{w1, w2, bee} {
		seedInbound(t, db, "i-"+key, key, SenderTypeUser, MsgStatusReceived, 10)
		seedOutboundRow(t, db, "o-"+key, key, SourceTypeWorker, "w1", "", OutboundStatusSent, 20)
	}
	seedInbound(t, db, "i-u2", "local:u2:worker:w1", SenderTypeUser, MsgStatusReceived, 10)
	_, err := db.Exec(`INSERT INTO bee_session_contexts (session_key, agent_id, engine, session_id, updated_at) VALUES
		('local:u1:worker:w1','w1','claude','s',1),
		('local:u1:worker:w2','w2','claude','s',1),
		('local:u1','bee','claude','s',1),
		('local:u1','w1','claude','s',1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO bee_tasks (id, message_id, worker_id, instruction, type, status, session_key, platform, created_at, updated_at)
		VALUES ('t1','m1','w1','go','countdown','pending','local:u1:worker:w1','local',1,1)`)
	require.NoError(t, err)
	require.NoError(t, s.MarkRead(ctx, "u1", "worker:w1", 5))
	require.NoError(t, s.MarkRead(ctx, "u2", "worker:w1", 5))
	require.NoError(t, s.MarkRead(ctx, "u1", "worker:w2", 5))
	require.NoError(t, s.MarkRead(ctx, "u1", "bee", 5))

	require.NoError(t, s.DeleteWorkerChats(ctx, "w1"))

	for _, key := range []string{w1, "local:u2:worker:w1"} {
		assert.Zero(t, countRows(t, db, `SELECT COUNT(*) FROM bee_platform_messages WHERE session_key = ?`, key), key)
		assert.Zero(t, countRows(t, db, `SELECT COUNT(*) FROM bee_outbound_messages WHERE session_key = ?`, key), key)
		assert.Zero(t, countRows(t, db, `SELECT COUNT(*) FROM bee_session_contexts WHERE session_key = ?`, key), key)
	}
	for _, key := range []string{w2, bee} {
		assert.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM bee_platform_messages WHERE session_key = ?`, key), key)
		assert.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM bee_outbound_messages WHERE session_key = ?`, key), key)
	}
	assert.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM bee_session_contexts WHERE session_key = ?`, w2))
	assert.Equal(t, 2, countRows(t, db, `SELECT COUNT(*) FROM bee_session_contexts WHERE session_key = ?`, bee))
	assert.Equal(t, 1, countRows(t, db, `SELECT COUNT(*) FROM bee_tasks WHERE id = 't1'`))

	marks, err := s.ReadMarks(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{"bee": 5, "worker:w2": 5}, marks)
	marks, err = s.ReadMarks(ctx, "u2")
	require.NoError(t, err)
	assert.Empty(t, marks)
}
