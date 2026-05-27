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

const outboundMessageColumns = `id, session_key, platform, account_name, content, media_path, status, platform_msg_id,
	source_type, source_id, inbound_msg_id, error, retry_count, sent_at, created_at`

// OutboundMessage is a row from the bee_outbound_messages table.
type OutboundMessage struct {
	ID            string `json:"id"`
	SessionKey    string `json:"session_key"`
	Platform      string `json:"platform"`
	AccountName   string `json:"account_name"`
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
		&m.ID, &m.SessionKey, &m.Platform, &m.AccountName, &m.Content, &m.MediaPath,
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
	if msg.AccountName == "" {
		msg.AccountName = "default"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bee_outbound_messages
		 (id, session_key, platform, account_name, content, media_path, status, platform_msg_id,
		  source_type, source_id, inbound_msg_id, error, retry_count, sent_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionKey, msg.Platform, msg.AccountName, msg.Content, msg.MediaPath,
		msg.Status, msg.PlatformMsgID, msg.SourceType, msg.SourceID,
		msg.InboundMsgID, msg.Error, msg.RetryCount, msg.SentAt, now,
	)
	return err
}

// OutboundMessageFilter holds optional filter criteria for ListFiltered.
// Zero values are ignored.
type OutboundMessageFilter struct {
	SessionKey  string
	Platform    string
	AccountName string
	Status      string // OutboundStatusSent or OutboundStatusFailed
	SourceType  string // SourceTypeBee, SourceTypeWorker, or SourceTypeSystem
	SourceID    string
	SentAtFrom  int64 // inclusive lower bound (Unix ms); 0 = no lower bound
	SentAtTo    int64 // inclusive upper bound (Unix ms); 0 = no upper bound
}

// ListedOutboundMessage is a bee_outbound_messages row for admin/API listing purposes.
type ListedOutboundMessage struct {
	ID           string `json:"id"`
	SessionKey   string `json:"session_key"`
	Platform     string `json:"platform"`
	AccountName  string `json:"account_name"`
	Content      string `json:"content"`
	Status       string `json:"status"`
	SourceType   string `json:"source_type"`
	SourceID     string `json:"source_id"`
	InboundMsgID string `json:"inbound_msg_id"`
	Error        string `json:"error"`
	SentAt       int64  `json:"sent_at"`
}

func (s *OutboundMessageStore) ListFiltered(ctx context.Context, f OutboundMessageFilter, limit, offset int) ([]ListedOutboundMessage, int, error) {
	where, args := outboundFilterWhere(f)

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bee_outbound_messages"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_key, platform, account_name, content, status, source_type, source_id, inbound_msg_id, error, sent_at
		 FROM bee_outbound_messages`+where+` ORDER BY sent_at DESC LIMIT ? OFFSET ?`,
		appendPaginationArgs(args, limit, offset)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	msgs := make([]ListedOutboundMessage, 0, limit)
	for rows.Next() {
		var m ListedOutboundMessage
		if err := rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.AccountName, &m.Content, &m.Status,
			&m.SourceType, &m.SourceID, &m.InboundMsgID, &m.Error, &m.SentAt); err != nil {
			return nil, 0, err
		}
		msgs = append(msgs, m)
	}
	return msgs, total, rows.Err()
}

func outboundFilterWhere(f OutboundMessageFilter) (string, []any) {
	var b whereBuilder
	if f.SessionKey != "" {
		b.add("session_key = ?", f.SessionKey)
	}
	if f.Platform != "" {
		b.add("platform = ?", f.Platform)
	}
	if f.AccountName != "" {
		b.add("account_name = ?", f.AccountName)
	}
	if f.Status != "" {
		b.add("status = ?", f.Status)
	}
	if f.SourceType != "" {
		b.add("source_type = ?", f.SourceType)
	}
	if f.SourceID != "" {
		b.add("source_id = ?", f.SourceID)
	}
	if f.SentAtFrom > 0 {
		b.add("sent_at >= ?", f.SentAtFrom)
	}
	if f.SentAtTo > 0 {
		b.add("sent_at <= ?", f.SentAtTo)
	}
	return b.build()
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
