package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	MsgStatusReceived     = "received"
	MsgStatusFeeding      = "feeding"
	MsgStatusMerged       = "merged"
	MsgStatusBeeProcessed = "bee_processed"
	MsgStatusFailed       = "failed"
	MsgStatusStale        = "stale"
)

// BatchMsg is a single row for a bulk insert via CreateBatch.
type BatchMsg struct {
	ID             string
	SessionKey     string
	Platform       string
	Content        string
	Raw            string
	PlatformMsgID  string
	MessageTime    int64
	Status         string // "received" or "merged"
	MergedInto     string // non-empty only when Status == "merged"
	TargetWorkerID string // when non-empty, route directly to this worker instead of bee
}

// MessageStore persists platform messages to the bee_platform_messages table.
type MessageStore struct {
	db *sql.DB
}

// NewMessageStore constructs a MessageStore.
func NewMessageStore(db *sql.DB) *MessageStore {
	return &MessageStore{db: db}
}

// Create inserts a new message record with status "received".
// Returns inserted=false (no error) when platform_msg_id is non-empty and already exists.
// If platform_msg_id is empty, the insert always proceeds (no dedup).
// messageTime is stored as received_at; pass 0 to use server time.
func (s *MessageStore) Create(ctx context.Context, id, sessionKey, platform, content, raw, platformMsgID string, messageTime int64) (bool, error) {
	if messageTime == 0 {
		messageTime = time.Now().UnixMilli()
	}
	now := time.Now().UnixMilli()
	result, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO bee_platform_messages (id, session_key, platform, content, raw, platform_msg_id, received_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, sessionKey, platform, content, raw, platformMsgID, messageTime, now, now,
	)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// UpdateStatusBatch sets the same status on all provided message IDs.
func (s *MessageStore) UpdateStatusBatch(ctx context.Context, ids []string, status string) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, 0, len(ids)+2)
	args = append(args, status, time.Now().UnixMilli())
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE bee_platform_messages SET status = ?, updated_at = ? WHERE id IN (`+inPlaceholders(len(ids))+`)`,
		args...,
	)
	return err
}

// FetchMergedContent returns the content of all messages merged into the given primary ID,
// ordered by received_at ASC (chronological arrival order).
func (s *MessageStore) FetchMergedContent(ctx context.Context, primaryID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT content FROM bee_platform_messages
         WHERE merged_into = ? AND status = ?
         ORDER BY received_at ASC`, primaryID, MsgStatusMerged)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var contents []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		contents = append(contents, c)
	}
	return contents, rows.Err()
}

// ClaimedMessage is a bee_platform_messages row claimed by the Feeder.
type ClaimedMessage struct {
	ID             string
	SessionKey     string
	Platform       string
	Content        string
	TargetWorkerID string
}

// ClaimBatch atomically selects up to batchSize 'received' messages — at most one per
// session_key — skipping any session that already has a message in 'feeding' status.
// Within each session, the message with the earliest received_at is selected (FIFO).
func (s *MessageStore) ClaimBatch(ctx context.Context, batchSize int) ([]ClaimedMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UnixMilli()
	if _, err := tx.ExecContext(ctx,
		`UPDATE bee_platform_messages
		 SET    status = ?, updated_at = ?
		 WHERE  status = ?
		   AND  EXISTS (
		          SELECT 1
		          FROM   bee_platform_messages b2
		          WHERE  b2.session_key  = bee_platform_messages.session_key
		            AND  b2.status       = ?
		            AND  b2.received_at  > bee_platform_messages.received_at
		        )`,
		MsgStatusStale, now, MsgStatusReceived, MsgStatusBeeProcessed,
	); err != nil {
		return nil, fmt.Errorf("mark stale: %w", err)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id, session_key, platform, content, target_worker_id
		 FROM bee_platform_messages m
		 WHERE status = ?
		   AND session_key NOT IN (
		       SELECT session_key FROM bee_platform_messages WHERE status = ?
		   )
		   AND received_at = (
		       SELECT MIN(received_at)
		       FROM bee_platform_messages m2
		       WHERE m2.session_key = m.session_key
		         AND m2.status = ?
		   )
		 ORDER BY received_at ASC
		 LIMIT ?`, MsgStatusReceived, MsgStatusFeeding, MsgStatusReceived, batchSize)
	if err != nil {
		return nil, fmt.Errorf("select batch: %w", err)
	}
	var msgs []ClaimedMessage
	for rows.Next() {
		var m ClaimedMessage
		if err := rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.TargetWorkerID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan: %w", err)
		}
		msgs = append(msgs, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, tx.Commit()
	}

	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	args := make([]any, 0, len(ids)+2)
	args = append(args, MsgStatusFeeding, now)
	for _, id := range ids {
		args = append(args, id)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE bee_platform_messages SET status = ?, updated_at = ? WHERE id IN (`+inPlaceholders(len(ids))+`)`, args...); err != nil {
		return nil, fmt.Errorf("update feeding: %w", err)
	}
	return msgs, tx.Commit()
}

// MarkBeeProcessed sets status to 'bee_processed' for the given message IDs.
func (s *MessageStore) MarkBeeProcessed(ctx context.Context, ids []string) error {
	return s.UpdateStatusBatch(ctx, ids, MsgStatusBeeProcessed)
}

// MarkFailed sets status to 'failed' for the given message IDs.
func (s *MessageStore) MarkFailed(ctx context.Context, ids []string) error {
	return s.UpdateStatusBatch(ctx, ids, MsgStatusFailed)
}

// FailReceived marks all 'received' messages for sessionKey as 'failed'.
// Returns the IDs of the affected messages.
func (s *MessageStore) FailReceived(ctx context.Context, sessionKey string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM bee_platform_messages WHERE session_key = ? AND status = ?`,
		sessionKey, MsgStatusReceived)
	if err != nil {
		return nil, fmt.Errorf("select received: %w", err)
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if err := s.UpdateStatusBatch(ctx, ids, MsgStatusFailed); err != nil {
		return nil, fmt.Errorf("mark failed: %w", err)
	}
	return ids, nil
}

// ResetFeedingToReceived resets all messages stuck in 'feeding' back to 'received'.
// Returns the IDs of affected rows so the caller can delete orphaned pending tasks.
func (s *MessageStore) ResetFeedingToReceived(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM bee_platform_messages WHERE status = ?`, MsgStatusFeeding)
	if err != nil {
		return nil, fmt.Errorf("select feeding: %w", err)
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if err := s.UpdateStatusBatch(ctx, ids, MsgStatusReceived); err != nil {
		return nil, fmt.Errorf("reset feeding: %w", err)
	}
	return ids, nil
}

// HasActiveMessages reports whether any messages with status received or feeding exist.
func (s *MessageStore) HasActiveMessages(ctx context.Context) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM bee_platform_messages WHERE status IN (?, ?))`,
		MsgStatusReceived, MsgStatusFeeding,
	).Scan(&exists)
	return exists == 1, err
}

// CountReceived returns the number of messages with status 'received'.
func (s *MessageStore) CountReceived(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bee_platform_messages WHERE status = ?`, MsgStatusReceived).Scan(&count)
	return count, err
}

// INSERT OR IGNORE. Returns the number of rows actually inserted.
// MessageTime is used as received_at; falls back to time.Now().UnixMilli() if zero.
func (s *MessageStore) CreateBatch(ctx context.Context, msgs []BatchMsg) (int64, error) {
	if len(msgs) == 0 {
		return 0, nil
	}

	now := time.Now().UnixMilli()
	placeholders := strings.Repeat("(?,?,?,?,?,?,?,?,?,?,?,?),", len(msgs))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, 0, len(msgs)*12)
	for _, m := range msgs {
		mt := m.MessageTime
		if mt == 0 {
			mt = now
		}
		args = append(args, m.ID, m.SessionKey, m.Platform, m.Content, m.Raw,
			m.PlatformMsgID, mt, m.Status, m.MergedInto, m.TargetWorkerID, now, now)
	}

	result, err := s.db.ExecContext(ctx,
		fmt.Sprintf(`INSERT OR IGNORE INTO bee_platform_messages
			(id, session_key, platform, content, raw, platform_msg_id, received_at, status, merged_into, target_worker_id, created_at, updated_at)
			VALUES %s`, placeholders),
		args...,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// StoredMessage is the subset of bee_platform_messages fields needed by platform senders.
type StoredMessage struct {
	Platform   string
	SessionKey string
	Raw        string
}

// GetByID fetches the platform, session_key, and raw fields for a single message.
func (s *MessageStore) GetByID(ctx context.Context, id string) (StoredMessage, error) {
	var m StoredMessage
	err := s.db.QueryRowContext(ctx,
		`SELECT platform, session_key, raw FROM bee_platform_messages WHERE id = ?`, id,
	).Scan(&m.Platform, &m.SessionKey, &m.Raw)
	if err != nil {
		return StoredMessage{}, fmt.Errorf("get message %s: %w", id, err)
	}
	return m, nil
}

// InboundMessage is a non-merged bee_platform_messages row for display in chat history.
type InboundMessage struct {
	ID         string
	Content    string
	ReceivedAt int64
}

// ListedMessage is a bee_platform_messages row for admin/API listing purposes.
type ListedMessage struct {
	ID         string `json:"id"`
	SessionKey string `json:"session_key"`
	Platform   string `json:"platform"`
	Content    string `json:"content"`
	Status     string `json:"status"`
	ReceivedAt int64  `json:"received_at"`
}

// MessageFilter holds optional filter criteria for ListFiltered.
// Zero values are ignored (no filtering on that field).
type MessageFilter struct {
	SessionKey      string
	Platform        string
	Status          string
	ReceivedAtFrom  int64 // inclusive lower bound (Unix ms); 0 = no lower bound
	ReceivedAtTo    int64 // inclusive upper bound (Unix ms); 0 = no upper bound
}

// ListFiltered returns paginated messages matching the given filters.
func (s *MessageStore) ListFiltered(ctx context.Context, f MessageFilter, limit, offset int) ([]ListedMessage, int, error) {
	where, args := messageFilterWhere(f)

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bee_platform_messages"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_key, platform, content, status, received_at
		 FROM bee_platform_messages`+where+` ORDER BY received_at DESC LIMIT ? OFFSET ?`,
		appendPaginationArgs(args, limit, offset)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var msgs []ListedMessage
	for rows.Next() {
		var m ListedMessage
		if err := rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.Status, &m.ReceivedAt); err != nil {
			return nil, 0, err
		}
		msgs = append(msgs, m)
	}
	return msgs, total, rows.Err()
}

func messageFilterWhere(f MessageFilter) (string, []any) {
	var b whereBuilder
	if f.SessionKey != "" {
		b.add("session_key = ?", f.SessionKey)
	}
	if f.Platform != "" {
		b.add("platform = ?", f.Platform)
	}
	if f.Status != "" {
		b.add("status = ?", f.Status)
	}
	if f.ReceivedAtFrom > 0 {
		b.add("received_at >= ?", f.ReceivedAtFrom)
	}
	if f.ReceivedAtTo > 0 {
		b.add("received_at <= ?", f.ReceivedAtTo)
	}
	return b.build()
}

// ListBySessionKey returns non-merged messages for a session.
// If before > 0, only messages with received_at < before are returned.
// Results are ordered by received_at ASC. limit must be > 0.
// limit controls max rows returned. Callers typically pass limit+1 to enable has_more detection.
func (s *MessageStore) ListBySessionKey(ctx context.Context, sessionKey string, before int64, limit int) ([]InboundMessage, error) {
	var (
		query string
		args  []any
	)
	if before > 0 {
		query = `SELECT id, content, received_at FROM bee_platform_messages
                 WHERE session_key = ? AND status != ? AND received_at < ?
                 ORDER BY received_at DESC LIMIT ?`
		args = []any{sessionKey, MsgStatusMerged, before, limit}
	} else {
		query = `SELECT id, content, received_at FROM bee_platform_messages
                 WHERE session_key = ? AND status != ?
                 ORDER BY received_at DESC LIMIT ?`
		args = []any{sessionKey, MsgStatusMerged, limit}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []InboundMessage
	for rows.Next() {
		var m InboundMessage
		if err := rows.Scan(&m.ID, &m.Content, &m.ReceivedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.Reverse(msgs)
	return msgs, nil
}

func scanIDs(rows *sql.Rows) ([]string, error) {
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
