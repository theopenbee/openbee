package localchat

import (
	"context"
	"fmt"

	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type ChatStore interface {
	WorkerChatSessionKeys(ctx context.Context, workerID string) ([]string, error)
	DeleteWorkerChats(ctx context.Context, workerID string) error
}

type ConversationStopper interface {
	StopConversation(ctx context.Context, sessionKey string, w model.Worker) (session.StopWorkerResult, error)
}

type ChatCleanup struct {
	chats   ChatStore
	stopper ConversationStopper
}

func NewChatCleanup(chats ChatStore, stopper ConversationStopper) *ChatCleanup {
	return &ChatCleanup{chats: chats, stopper: stopper}
}

func (c *ChatCleanup) DeleteWorkerChats(ctx context.Context, w model.Worker) error {
	keys, err := c.chats.WorkerChatSessionKeys(ctx, w.ID)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if _, err := c.stopper.StopConversation(ctx, key, w); err != nil {
			return fmt.Errorf("stop conversation %s: %w", key, err)
		}
	}
	return c.chats.DeleteWorkerChats(ctx, w.ID)
}
