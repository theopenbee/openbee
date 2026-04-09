package store

import (
	"context"
	"database/sql"
	"fmt"
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
// Queries limit+1 rows so callers can detect has_more by checking len(result) > limit.
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
		args = []any{sessionKey, before, limit + 1}
	} else {
		query = `SELECT ` + outboundMessageColumns + `
			 FROM bee_outbound_messages
			 WHERE session_key = ?
			 ORDER BY sent_at DESC LIMIT ?`
		args = []any{sessionKey, limit + 1}
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
	// Reverse DESC result to ASC for callers.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// allowedFilterColumns is a whitelist of columns that listByColumn may filter on.
var allowedFilterColumns = map[string]bool{
	"platform":  true,
	"source_id": true,
}

// listByColumn returns outbound messages filtered by a single column, ordered by sent_at descending.
func (s *OutboundMessageStore) listByColumn(ctx context.Context, column, value string, limit int) ([]OutboundMessage, error) {
	if !allowedFilterColumns[column] {
		return nil, fmt.Errorf("invalid filter column: %q", column)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+outboundMessageColumns+`
		 FROM bee_outbound_messages
		 WHERE `+column+` = ?
		 ORDER BY sent_at DESC
		 LIMIT ?`,
		value, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOutboundMessages(rows)
}

// ListByPlatform returns outbound messages for a platform ordered by sent_at descending.
func (s *OutboundMessageStore) ListByPlatform(ctx context.Context, platform string, limit int) ([]OutboundMessage, error) {
	return s.listByColumn(ctx, "platform", platform, limit)
}

// ListBySourceID returns outbound messages sent by a specific source (worker/bee) ordered by sent_at descending.
func (s *OutboundMessageStore) ListBySourceID(ctx context.Context, sourceID string, limit int) ([]OutboundMessage, error) {
	return s.listByColumn(ctx, "source_id", sourceID, limit)
}

// DeleteBySessionKey removes all outbound messages for the given session key.
func (s *OutboundMessageStore) DeleteBySessionKey(ctx context.Context, sessionKey string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM bee_outbound_messages WHERE session_key = ?`, sessionKey,
	)
	return err
}
