package store

import (
	"context"
	"database/sql"
	"slices"
	"time"
)

// Outbound message status constants.
const (
	OutboundStatusSent   = "sent"
	OutboundStatusFailed = "failed"
)

// Outbound message source type constants.
const (
	SourceTypeBee    = "bee"
	SourceTypeWorker = "worker"
	SourceTypeSystem = "system"
)

const outboundMessageColumns = `id, session_key, platform, content, media_path, status, platform_msg_id,
	source_type, source_id, inbound_msg_id, error, retry_count, sent_at, created_at`

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

func scanOutboundMessage(scanner interface{ Scan(...any) error }) (OutboundMessage, error) {
	var m OutboundMessage
	err := scanner.Scan(
		&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.MediaPath,
		&m.Status, &m.PlatformMsgID, &m.SourceType, &m.SourceID,
		&m.InboundMsgID, &m.Error, &m.RetryCount, &m.SentAt, &m.CreatedAt,
	)
	return m, err
}

func scanOutboundMessages(rows *sql.Rows) ([]OutboundMessage, error) {
	var msgs []OutboundMessage
	for rows.Next() {
		m, err := scanOutboundMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
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

// ListBySessionKey returns outbound messages for a session ordered by sent_at ascending.
// If before > 0, only messages with sent_at < before are returned.
// limit must be > 0.
// limit controls max rows returned. Callers typically pass limit+1 to enable has_more detection.
func (s *OutboundMessageStore) ListBySessionKey(ctx context.Context, sessionKey string, before int64, limit int) ([]OutboundMessage, error) {
	var (
		query string
		args  []any
	)
	if before > 0 {
		query = `SELECT ` + outboundMessageColumns + `
			 FROM bee_outbound_messages
			 WHERE session_key = ? AND sent_at < ?
			 ORDER BY sent_at DESC LIMIT ?`
		args = []any{sessionKey, before, limit}
	} else {
		query = `SELECT ` + outboundMessageColumns + `
			 FROM bee_outbound_messages
			 WHERE session_key = ?
			 ORDER BY sent_at DESC LIMIT ?`
		args = []any{sessionKey, limit}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	msgs, err := scanOutboundMessages(rows)
	if err != nil {
		return nil, err
	}
	slices.Reverse(msgs)
	return msgs, nil
}
