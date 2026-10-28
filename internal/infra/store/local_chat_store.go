package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"unicode/utf8"
)

type LastMessage struct {
	SenderType string
	SenderID   string
	Kind       string
	Content    string
	Payload    string
	TS         int64
}

type LocalChatStore struct {
	db *sql.DB
}

func NewLocalChatStore(db *sql.DB) *LocalChatStore {
	return &LocalChatStore{db: db}
}

func (s *LocalChatStore) MarkRead(ctx context.Context, userID, conversationID string, ts int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bee_local_reads (user_id, conversation_id, last_read_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(user_id, conversation_id) DO UPDATE
		 SET last_read_at = MAX(last_read_at, excluded.last_read_at)`,
		userID, conversationID, ts,
	)
	return err
}

func (s *LocalChatStore) ReadMarks(ctx context.Context, userID string) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT conversation_id, last_read_at FROM bee_local_reads WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	marks := make(map[string]int64)
	for rows.Next() {
		var conversationID string
		var lastReadAt int64
		if err := rows.Scan(&conversationID, &lastReadAt); err != nil {
			return nil, err
		}
		marks[conversationID] = lastReadAt
	}
	return marks, rows.Err()
}

func (s *LocalChatStore) LastMessage(ctx context.Context, sessionKey string) (LastMessage, bool, error) {
	var m LastMessage
	err := s.db.QueryRowContext(ctx,
		`SELECT sender_type, sender_id, kind, content, payload, ts FROM (
			SELECT sender_type AS sender_type, '' AS sender_id, ? AS kind, content AS content,
			       '' AS payload, received_at AS ts, 0 AS ord
			  FROM bee_platform_messages
			 WHERE session_key = ? AND status != ?
			UNION ALL
			SELECT CASE source_type WHEN '' THEN ? ELSE source_type END, source_id, kind, content,
			       payload, sent_at, 1
			  FROM bee_outbound_messages
			 WHERE session_key = ? AND status = ? AND kind != ?
		 ) ORDER BY ts DESC, ord DESC LIMIT 1`,
		OutboundKindMessage, sessionKey, MsgStatusMerged,
		SenderTypeBee, sessionKey, OutboundStatusSent, OutboundKindReset,
	).Scan(&m.SenderType, &m.SenderID, &m.Kind, &m.Content, &m.Payload, &m.TS)
	if errors.Is(err, sql.ErrNoRows) {
		return LastMessage{}, false, nil
	}
	if err != nil {
		return LastMessage{}, false, fmt.Errorf("last message for session %s: %w", sessionKey, err)
	}
	return m, true, nil
}

func (s *LocalChatStore) LastKind(ctx context.Context, sessionKey string) (string, bool, error) {
	var kind string
	err := s.db.QueryRowContext(ctx,
		`SELECT kind FROM (
			SELECT ? AS kind, received_at AS ts, 0 AS ord
			  FROM bee_platform_messages
			 WHERE session_key = ? AND status != ?
			UNION ALL
			SELECT kind, sent_at, 1
			  FROM bee_outbound_messages
			 WHERE session_key = ? AND status = ?
		 ) ORDER BY ts DESC, ord DESC LIMIT 1`,
		OutboundKindMessage, sessionKey, MsgStatusMerged,
		sessionKey, OutboundStatusSent,
	).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("last kind for session %s: %w", sessionKey, err)
	}
	return kind, true, nil
}

func (s *LocalChatStore) UnreadCount(ctx context.Context, sessionKey string, after int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT
			(SELECT COUNT(*) FROM bee_outbound_messages
			  WHERE session_key = ? AND status = ? AND kind != ? AND sent_at > ?)
		  + (SELECT COUNT(*) FROM bee_platform_messages
			  WHERE session_key = ? AND sender_type = ? AND received_at > ?)`,
		sessionKey, OutboundStatusSent, OutboundKindReset, after,
		sessionKey, SenderTypeBee, after,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("unread count for session %s: %w", sessionKey, err)
	}
	return count, nil
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func workerChatSessionKeys(ctx context.Context, q queryer, workerID string) ([]string, error) {
	suffix := ":worker:" + workerID
	rows, err := q.QueryContext(ctx,
		`SELECT session_key FROM bee_platform_messages
		  WHERE substr(session_key, 1, 6) = 'local:' AND substr(session_key, -?) = ?
		 UNION
		 SELECT session_key FROM bee_outbound_messages
		  WHERE substr(session_key, 1, 6) = 'local:' AND substr(session_key, -?) = ?
		 UNION
		 SELECT session_key FROM bee_tasks
		  WHERE substr(session_key, 1, 6) = 'local:' AND substr(session_key, -?) = ?
		 UNION
		 SELECT session_key FROM bee_session_contexts
		  WHERE substr(session_key, 1, 6) = 'local:' AND substr(session_key, -?) = ?`,
		utf8.RuneCountInString(suffix), suffix,
		utf8.RuneCountInString(suffix), suffix,
		utf8.RuneCountInString(suffix), suffix,
		utf8.RuneCountInString(suffix), suffix,
	)
	if err != nil {
		return nil, err
	}
	return scanIDs(rows)
}

func (s *LocalChatStore) WorkerChatSessionKeys(ctx context.Context, workerID string) ([]string, error) {
	keys, err := workerChatSessionKeys(ctx, s.db, workerID)
	if err != nil {
		return nil, fmt.Errorf("worker chat session keys for %s: %w", workerID, err)
	}
	return keys, nil
}

func (s *LocalChatStore) DeleteWorkerChats(ctx context.Context, workerID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	keys, err := workerChatSessionKeys(ctx, tx, workerID)
	if err != nil {
		return fmt.Errorf("worker chat session keys for %s: %w", workerID, err)
	}
	for _, key := range keys {
		for _, table := range []string{"bee_platform_messages", "bee_outbound_messages", "bee_session_contexts"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE session_key = ?`, key); err != nil {
				return fmt.Errorf("delete %s for session %s: %w", table, key, err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM bee_local_reads WHERE conversation_id = 'worker:' || ?`, workerID,
	); err != nil {
		return fmt.Errorf("delete local reads for worker %s: %w", workerID, err)
	}
	return tx.Commit()
}
