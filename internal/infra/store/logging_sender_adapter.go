package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/platform"
)

var senderLog = logger.With(zap.String("component", "logging_sender"))

// LoggingPlatformSenderAdapter wraps a PlatformSenderAdapter and records every
// outbound message to OutboundMessageStore regardless of send success or failure.
type LoggingPlatformSenderAdapter struct {
	inner        platform.PlatformSenderAdapter
	outboundStore *OutboundMessageStore
	platformID   string
}

// NewLoggingPlatformSenderAdapter constructs a LoggingPlatformSenderAdapter.
func NewLoggingPlatformSenderAdapter(
	inner platform.PlatformSenderAdapter,
	outboundStore *OutboundMessageStore,
	platformID string,
) *LoggingPlatformSenderAdapter {
	return &LoggingPlatformSenderAdapter{
		inner:        inner,
		outboundStore: outboundStore,
		platformID:   platformID,
	}
}

// Send delegates to the inner sender and records the result to the outbound store.
func (a *LoggingPlatformSenderAdapter) Send(ctx context.Context, msg platform.OutboundMessage) error {
	sentAt := time.Now().UnixMilli()
	sendErr := a.inner.Send(ctx, msg)

	status := OutboundStatusSent
	errMsg := ""
	if sendErr != nil {
		status = OutboundStatusFailed
		errMsg = sendErr.Error()
	}

	// Most callers populate ReplyTo (which carries the originating inbound message's session),
	// but direct sends (e.g. proactive notifications) may only set the top-level SessionKey.
	sessionKey := msg.ReplyTo.SessionKey
	if sessionKey == "" {
		sessionKey = msg.SessionKey
	}

	record := OutboundMessage{
		ID:           uuid.New().String(),
		SessionKey:   sessionKey,
		Platform:     a.platformID,
		Content:      msg.Content,
		MediaPath:    msg.MediaPath,
		Status:       status,
		SourceType:   msg.SourceType,
		SourceID:     msg.SourceID,
		InboundMsgID: msg.InboundMsgID,
		Error:        errMsg,
		SentAt:       sentAt,
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if storeErr := a.outboundStore.Create(ctx, record); storeErr != nil {
			senderLog.Error("failed to store outbound message", zap.Error(storeErr),
				zap.String("platform", a.platformID),
				zap.String("sessionKey", sessionKey),
			)
		}
	}()

	return sendErr
}
