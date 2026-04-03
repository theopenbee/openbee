package store

import (
	"context"
	"database/sql"
	"time"
)

// OutboundMessage is a row from the bee_outbound_messages table.
type OutboundMessage struct {
	ID            string `json:"id"`
	SessionKey    string `json:"session_key"`
	Platform      string `json:"platform"`
	Content       string `json:"content"`
	MediaPath     string `json:"media_path"`
	Status        string `json:"status"`
	PlatformMsgID string `json:"platform_msg_id"`
	SourceType    string `json:"source_type"`
	SourceID      string `json:"source_id"`
	InboundMsgID  string `json:"inbound_msg_id"`
	Error         string `json:"error"`
	RetryCount    int    `json:"retry_count"`
	SentAt        int64  `json:"sent_at"`
	CreatedAt     int64  `json:"created_at"`
}

// OutboundMessageStore manages the bee_outbound_messages table.
type OutboundMessageStore struct {
	db *sql.DB
}

// NewOutboundMessageStore constructs an OutboundMessageStore.
func NewOutboundMessageStore(db *sql.DB) *OutboundMessageStore {
	return &OutboundMessageStore{db: db}
}

// Create inserts a new outbound message record.
func (s *OutboundMessageStore) Create(ctx context.Context, msg OutboundMessage) error {
	now := time.Now().UnixMilli()
	if msg.SentAt == 0 {
		msg.SentAt = now
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bee_outbound_messages
		 (id, session_key, platform, content, media_path, status, platform_msg_id,
		  source_type, source_id, inbound_msg_id, error, retry_count, sent_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionKey, msg.Platform, msg.Content, msg.MediaPath,
		msg.Status, msg.PlatformMsgID, msg.SourceType, msg.SourceID,
		msg.InboundMsgID, msg.Error, msg.RetryCount, msg.SentAt, now,
	)
	return err
}

// ListBySessionKey returns all outbound messages for a session ordered by sent_at ascending.
func (s *OutboundMessageStore) ListBySessionKey(ctx context.Context, sessionKey string) ([]OutboundMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_key, platform, content, media_path, status, platform_msg_id,
		        source_type, source_id, inbound_msg_id, error, retry_count, sent_at, created_at
		 FROM bee_outbound_messages
		 WHERE session_key = ?
		 ORDER BY sent_at ASC`,
		sessionKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []OutboundMessage
	for rows.Next() {
		var m OutboundMessage
		if err := rows.Scan(
			&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.MediaPath,
			&m.Status, &m.PlatformMsgID, &m.SourceType, &m.SourceID,
			&m.InboundMsgID, &m.Error, &m.RetryCount, &m.SentAt, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// ListByPlatform returns outbound messages for a platform ordered by sent_at descending.
func (s *OutboundMessageStore) ListByPlatform(ctx context.Context, platform string, limit int) ([]OutboundMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_key, platform, content, media_path, status, platform_msg_id,
		        source_type, source_id, inbound_msg_id, error, retry_count, sent_at, created_at
		 FROM bee_outbound_messages
		 WHERE platform = ?
		 ORDER BY sent_at DESC
		 LIMIT ?`,
		platform, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []OutboundMessage
	for rows.Next() {
		var m OutboundMessage
		if err := rows.Scan(
			&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.MediaPath,
			&m.Status, &m.PlatformMsgID, &m.SourceType, &m.SourceID,
			&m.InboundMsgID, &m.Error, &m.RetryCount, &m.SentAt, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// ListBySourceID returns outbound messages sent by a specific source (worker/bee) ordered by sent_at descending.
func (s *OutboundMessageStore) ListBySourceID(ctx context.Context, sourceID string, limit int) ([]OutboundMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_key, platform, content, media_path, status, platform_msg_id,
		        source_type, source_id, inbound_msg_id, error, retry_count, sent_at, created_at
		 FROM bee_outbound_messages
		 WHERE source_id = ?
		 ORDER BY sent_at DESC
		 LIMIT ?`,
		sourceID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []OutboundMessage
	for rows.Next() {
		var m OutboundMessage
		if err := rows.Scan(
			&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.MediaPath,
			&m.Status, &m.PlatformMsgID, &m.SourceType, &m.SourceID,
			&m.InboundMsgID, &m.Error, &m.RetryCount, &m.SentAt, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// DeleteBySessionKey removes all outbound messages for the given session key.
func (s *OutboundMessageStore) DeleteBySessionKey(ctx context.Context, sessionKey string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM bee_outbound_messages WHERE session_key = ?`, sessionKey,
	)
	return err
}
