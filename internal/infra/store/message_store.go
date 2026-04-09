package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// BatchMsg is a single row for a bulk insert via CreateBatch.
type BatchMsg struct {
	ID            string
	SessionKey    string
	Platform      string
	Content       string
	Raw           string
	PlatformMsgID string
	MessageTime   int64
	Status        string // "received" or "merged"
	MergedInto    string // non-empty only when Status == "merged"
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

// SetStatus updates the status of a single message.
func (s *MessageStore) SetStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE bee_platform_messages SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UnixMilli(), id,
	)
	return err
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

// MarkMerged sets primaryID status to "merged" and records merged_into on all mergedIDs.
func (s *MessageStore) MarkMerged(ctx context.Context, primaryID string, mergedIDs []string) error {
	now := time.Now().UnixMilli()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE bee_platform_messages SET status = 'merged', updated_at = ? WHERE id = ?`, now, primaryID,
	); err != nil {
		return err
	}
	for _, id := range mergedIDs {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE bee_platform_messages SET status = 'merged', merged_into = ?, updated_at = ? WHERE id = ?`,
			primaryID, now, id,
		); err != nil {
			return err
		}
	}
	return nil
}

// FetchMergedContent returns the content of all messages merged into the given primary ID,
// ordered by received_at ASC (chronological arrival order).
func (s *MessageStore) FetchMergedContent(ctx context.Context, primaryID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT content FROM bee_platform_messages
         WHERE merged_into = ? AND status = 'merged'
         ORDER BY received_at ASC`, primaryID)
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

// CreateBatch inserts multiple message rows in a single transaction using
// ClaimedMessage is a bee_platform_messages row claimed by the Feeder.
type ClaimedMessage struct {
	ID         string
	SessionKey string
	Platform   string
	Content    string
	RetryCount int
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

	rows, err := tx.QueryContext(ctx,
		`SELECT id, session_key, platform, content, retry_count
		 FROM bee_platform_messages m
		 WHERE status = 'received'
		   AND session_key NOT IN (
		       SELECT session_key FROM bee_platform_messages WHERE status = 'feeding'
		   )
		   AND received_at = (
		       SELECT MIN(received_at)
		       FROM bee_platform_messages m2
		       WHERE m2.session_key = m.session_key
		         AND m2.status = 'received'
		   )
		 ORDER BY received_at ASC
		 LIMIT ?`, batchSize)
	if err != nil {
		return nil, fmt.Errorf("select batch: %w", err)
	}
	var msgs []ClaimedMessage
	for rows.Next() {
		var m ClaimedMessage
		if err := rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.RetryCount); err != nil {
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
		return nil, nil
	}

	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	args := make([]any, 0, len(ids)+2)
	args = append(args, "feeding", time.Now().UnixMilli())
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
	return s.UpdateStatusBatch(ctx, ids, "bee_processed")
}

// ResetFeedingBatch restores 'feeding' messages back to 'received'.
func (s *MessageStore) ResetFeedingBatch(ctx context.Context, ids []string) error {
	return s.UpdateStatusBatch(ctx, ids, "received")
}

// RollbackWithRetry increments retry_count for each message and resets status to
// 'received' for messages below maxRetries, or 'failed' for those that have reached
// the limit. Callers determine which IDs are permanently failed from their in-memory
// ClaimedMessage.RetryCount values (see Feeder.rollback).
func (s *MessageStore) RollbackWithRetry(ctx context.Context, ids []string, maxRetries int) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	args := make([]any, 0, 2+len(ids))
	args = append(args, maxRetries, now)
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE bee_platform_messages
		SET retry_count = retry_count + 1,
		    status = CASE WHEN retry_count + 1 >= ? THEN 'failed' ELSE 'received' END,
		    updated_at = ?
		WHERE id IN (`+inPlaceholders(len(ids))+`)`, args...)
	if err != nil {
		return fmt.Errorf("rollback with retry: %w", err)
	}
	return nil
}

// ResetFeedingToReceived resets all messages stuck in 'feeding' back to 'received'.
// Returns the IDs of affected rows so the caller can delete orphaned pending tasks.
func (s *MessageStore) ResetFeedingToReceived(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM bee_platform_messages WHERE status = 'feeding'`)
	if err != nil {
		return nil, fmt.Errorf("select feeding: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if err := s.UpdateStatusBatch(ctx, ids, "received"); err != nil {
		return nil, fmt.Errorf("reset feeding: %w", err)
	}
	return ids, nil
}

// CountReceived returns the number of messages with status 'received'.
func (s *MessageStore) CountReceived(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bee_platform_messages WHERE status = 'received'`).Scan(&count)
	return count, err
}

// INSERT OR IGNORE. Returns the number of rows actually inserted.
// MessageTime is used as received_at; falls back to time.Now().UnixMilli() if zero.
func (s *MessageStore) CreateBatch(ctx context.Context, msgs []BatchMsg) (int64, error) {
	if len(msgs) == 0 {
		return 0, nil
	}

	now := time.Now().UnixMilli()
	placeholders := strings.Repeat("(?,?,?,?,?,?,?,?,?,?,?),", len(msgs))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, 0, len(msgs)*11)
	for _, m := range msgs {
		mt := m.MessageTime
		if mt == 0 {
			mt = now
		}
		args = append(args, m.ID, m.SessionKey, m.Platform, m.Content, m.Raw,
			m.PlatformMsgID, mt, m.Status, m.MergedInto, now, now)
	}

	result, err := s.db.ExecContext(ctx,
		fmt.Sprintf(`INSERT OR IGNORE INTO bee_platform_messages
			(id, session_key, platform, content, raw, platform_msg_id, received_at, status, merged_into, created_at, updated_at)
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

// ListBySessionKey returns non-merged messages for a session.
// If before > 0, only messages with received_at < before are returned.
// Results are ordered by received_at ASC. limit must be > 0.
func (s *MessageStore) ListBySessionKey(ctx context.Context, sessionKey string, before int64, limit int) ([]InboundMessage, error) {
	var (
		query string
		args  []any
	)
	if before > 0 {
		query = `SELECT id, content, received_at FROM bee_platform_messages
                 WHERE session_key = ? AND status != 'merged' AND received_at < ?
                 ORDER BY received_at DESC LIMIT ?`
		args = []any{sessionKey, before, limit}
	} else {
		query = `SELECT id, content, received_at FROM bee_platform_messages
                 WHERE session_key = ? AND status != 'merged'
                 ORDER BY received_at DESC LIMIT ?`
		args = []any{sessionKey, limit}
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
	// Reverse DESC result to ASC for callers.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// DeleteBySessionKey removes all bee_platform_messages for the given session key.
func (s *MessageStore) DeleteBySessionKey(ctx context.Context, sessionKey string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM bee_platform_messages WHERE session_key = ?`, sessionKey,
	)
	return err
}
