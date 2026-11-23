package localchat

import (
	"context"
	"fmt"
	"slices"

	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type ChatStore interface {
	WorkerChatSessionKeys(ctx context.Context, workerID string) ([]string, error)
	DeleteWorkerChats(ctx context.Context, workerID string) error
}

type ConversationStopper interface {
	StopConversation(ctx context.Context, sessionKey string, w model.Worker) (session.StopWorkerResult, error)
}

type WorkerTasks interface {
	List(ctx context.Context, f store.TaskFilter) ([]model.Task, error)
	Cancel(ctx context.Context, f store.CancelFilter) (int64, error)
}

type ChatCleanup struct {
	chats   ChatStore
	stopper ConversationStopper
	tasks   WorkerTasks
}

func NewChatCleanup(chats ChatStore, stopper ConversationStopper, tasks WorkerTasks) *ChatCleanup {
	return &ChatCleanup{chats: chats, stopper: stopper, tasks: tasks}
}

func (c *ChatCleanup) DeleteWorkerChats(ctx context.Context, w model.Worker) error {
	keys, err := c.chats.WorkerChatSessionKeys(ctx, w.ID)
	if err != nil {
		return err
	}
	active, err := c.tasks.List(ctx, store.TaskFilter{WorkerID: w.ID, Status: model.TaskStatusActive})
	if err != nil {
		return fmt.Errorf("list active tasks: %w", err)
	}
	for _, t := range active {
		if !slices.Contains(keys, t.SessionKey) {
			keys = append(keys, t.SessionKey)
		}
	}
	for _, key := range keys {
		if _, err := c.stopper.StopConversation(ctx, key, w); err != nil {
			return fmt.Errorf("stop conversation %s: %w", key, err)
		}
		if _, err := c.tasks.Cancel(ctx, store.CancelFilter{SessionKey: key, WorkerID: w.ID}); err != nil {
			return fmt.Errorf("cancel tasks in conversation %s: %w", key, err)
		}
	}
	return c.chats.DeleteWorkerChats(ctx, w.ID)
}
