package localchat_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/domain/localchat"
	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type cleanupLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *cleanupLog) add(call string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, call)
}

func (l *cleanupLog) list() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

type fakeChatStore struct {
	log       *cleanupLog
	keys      []string
	keysErr   error
	deleteErr error
}

func (f *fakeChatStore) WorkerChatSessionKeys(_ context.Context, workerID string) ([]string, error) {
	f.log.add("keys:" + workerID)
	return f.keys, f.keysErr
}

func (f *fakeChatStore) DeleteWorkerChats(_ context.Context, workerID string) error {
	f.log.add("delete:" + workerID)
	return f.deleteErr
}

type fakeStopper struct {
	log     *cleanupLog
	failKey string
	err     error
}

func (f *fakeStopper) StopConversation(_ context.Context, sessionKey string, w model.Worker) (session.StopWorkerResult, error) {
	f.log.add("stop:" + sessionKey + ":" + w.ID)
	if sessionKey == f.failKey {
		return session.StopWorkerResult{}, f.err
	}
	return session.StopWorkerResult{}, nil
}

func TestChatCleanupStopsThenDeletes(t *testing.T) {
	log := &cleanupLog{}
	chats := &fakeChatStore{log: log, keys: []string{"local:u1:worker:w1", "local:u2:worker:w1"}}
	cleanup := localchat.NewChatCleanup(chats, &fakeStopper{log: log})

	require.NoError(t, cleanup.DeleteWorkerChats(context.Background(), model.Worker{ID: "w1"}))

	assert.Equal(t, []string{
		"keys:w1",
		"stop:local:u1:worker:w1:w1",
		"stop:local:u2:worker:w1:w1",
		"delete:w1",
	}, log.list())
}

func TestChatCleanupDeletesWhenWorkerHasNoChats(t *testing.T) {
	log := &cleanupLog{}
	cleanup := localchat.NewChatCleanup(&fakeChatStore{log: log}, &fakeStopper{log: log})

	require.NoError(t, cleanup.DeleteWorkerChats(context.Background(), model.Worker{ID: "w1"}))

	assert.Equal(t, []string{"keys:w1", "delete:w1"}, log.list())
}

func TestChatCleanupStopFailureSkipsDelete(t *testing.T) {
	log := &cleanupLog{}
	stopErr := errors.New("stop failed")
	chats := &fakeChatStore{log: log, keys: []string{"local:u1:worker:w1", "local:u2:worker:w1"}}
	cleanup := localchat.NewChatCleanup(chats, &fakeStopper{log: log, failKey: "local:u1:worker:w1", err: stopErr})

	err := cleanup.DeleteWorkerChats(context.Background(), model.Worker{ID: "w1"})

	require.ErrorIs(t, err, stopErr)
	assert.Equal(t, []string{"keys:w1", "stop:local:u1:worker:w1:w1"}, log.list())
}

func TestChatCleanupKeyLookupFailureSkipsDelete(t *testing.T) {
	log := &cleanupLog{}
	keysErr := errors.New("keys failed")
	cleanup := localchat.NewChatCleanup(&fakeChatStore{log: log, keysErr: keysErr}, &fakeStopper{log: log})

	err := cleanup.DeleteWorkerChats(context.Background(), model.Worker{ID: "w1"})

	require.ErrorIs(t, err, keysErr)
	assert.Equal(t, []string{"keys:w1"}, log.list())
}

func TestChatCleanupReturnsDeleteError(t *testing.T) {
	log := &cleanupLog{}
	deleteErr := errors.New("delete failed")
	cleanup := localchat.NewChatCleanup(&fakeChatStore{log: log, deleteErr: deleteErr}, &fakeStopper{log: log})

	err := cleanup.DeleteWorkerChats(context.Background(), model.Worker{ID: "w1"})

	require.ErrorIs(t, err, deleteErr)
}
