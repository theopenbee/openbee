package local

import (
	"context"
	"log/slog"

	"github.com/theopenbee/openbee/internal/platform"
)

// LocalReceiver implements platform.PlatformReceiverAdapter.
// Messages are injected via Enqueue and dispatched to the registered handler in Start.
type LocalReceiver struct {
	ch chan platform.InboundMessage
}

// NewLocalReceiver constructs a LocalReceiver with the given channel buffer size.
func NewLocalReceiver(bufSize int) *LocalReceiver {
	return &LocalReceiver{ch: make(chan platform.InboundMessage, bufSize)}
}

// Start blocks, dispatching each enqueued message, until ctx is cancelled.
// Implements platform.PlatformReceiverAdapter.
func (r *LocalReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	for {
		select {
		case msg := <-r.ch:
			dispatch(msg)
		case <-ctx.Done():
			return nil
		}
	}
}

// Enqueue adds a message to the dispatch queue.
// If the channel is full the message is dropped and a warning is logged.
func (r *LocalReceiver) Enqueue(msg platform.InboundMessage) {
	select {
	case r.ch <- msg:
	default:
		slog.Warn("local receiver: channel full, dropping message",
			"component", "local", "sessionKey", msg.SessionKey)
	}
}
