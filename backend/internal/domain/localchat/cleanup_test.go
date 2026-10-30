package localchat_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/domain/localchat"
	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform/local"
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

type fakeTaskCanceller struct {
	log *cleanupLog
	err error
}

func (f *fakeTaskCanceller) Cancel(_ context.Context, filter store.CancelFilter) (int64, error) {
	f.log.add("cancel:" + filter.SessionKey + ":" + filter.WorkerID + filter.Type)
	return 0, f.err
}

func TestChatCleanupStopsThenDeletes(t *testing.T) {
	log := &cleanupLog{}
	chats := &fakeChatStore{log: log, keys: []string{"local:u1:worker:w1", "local:u2:worker:w1"}}
	cleanup := localchat.NewChatCleanup(chats, &fakeStopper{log: log}, &fakeTaskCanceller{log: log})

	require.NoError(t, cleanup.DeleteWorkerChats(context.Background(), model.Worker{ID: "w1"}))

	assert.Equal(t, []string{
		"keys:w1",
		"stop:local:u1:worker:w1:w1",
		"cancel:local:u1:worker:w1:w1",
		"stop:local:u2:worker:w1:w1",
		"cancel:local:u2:worker:w1:w1",
		"delete:w1",
	}, log.list())
}

func TestChatCleanupDeletesWhenWorkerHasNoChats(t *testing.T) {
	log := &cleanupLog{}
	cleanup := localchat.NewChatCleanup(&fakeChatStore{log: log}, &fakeStopper{log: log}, &fakeTaskCanceller{log: log})

	require.NoError(t, cleanup.DeleteWorkerChats(context.Background(), model.Worker{ID: "w1"}))

	assert.Equal(t, []string{"keys:w1", "delete:w1"}, log.list())
}

func TestChatCleanupStopFailureSkipsDelete(t *testing.T) {
	log := &cleanupLog{}
	stopErr := errors.New("stop failed")
	chats := &fakeChatStore{log: log, keys: []string{"local:u1:worker:w1", "local:u2:worker:w1"}}
	cleanup := localchat.NewChatCleanup(chats, &fakeStopper{log: log, failKey: "local:u1:worker:w1", err: stopErr}, &fakeTaskCanceller{log: log})

	err := cleanup.DeleteWorkerChats(context.Background(), model.Worker{ID: "w1"})

	require.ErrorIs(t, err, stopErr)
	assert.Equal(t, []string{"keys:w1", "stop:local:u1:worker:w1:w1"}, log.list())
}

func TestChatCleanupKeyLookupFailureSkipsDelete(t *testing.T) {
	log := &cleanupLog{}
	keysErr := errors.New("keys failed")
	cleanup := localchat.NewChatCleanup(&fakeChatStore{log: log, keysErr: keysErr}, &fakeStopper{log: log}, &fakeTaskCanceller{log: log})

	err := cleanup.DeleteWorkerChats(context.Background(), model.Worker{ID: "w1"})

	require.ErrorIs(t, err, keysErr)
	assert.Equal(t, []string{"keys:w1"}, log.list())
}

func TestChatCleanupReturnsDeleteError(t *testing.T) {
	log := &cleanupLog{}
	deleteErr := errors.New("delete failed")
	cleanup := localchat.NewChatCleanup(&fakeChatStore{log: log, deleteErr: deleteErr}, &fakeStopper{log: log}, &fakeTaskCanceller{log: log})

	err := cleanup.DeleteWorkerChats(context.Background(), model.Worker{ID: "w1"})

	require.ErrorIs(t, err, deleteErr)
}

func TestChatCleanupCancelFailureSkipsDelete(t *testing.T) {
	log := &cleanupLog{}
	cancelErr := errors.New("cancel failed")
	chats := &fakeChatStore{log: log, keys: []string{"local:u1:worker:w1"}}
	cleanup := localchat.NewChatCleanup(chats, &fakeStopper{log: log}, &fakeTaskCanceller{log: log, err: cancelErr})

	err := cleanup.DeleteWorkerChats(context.Background(), model.Worker{ID: "w1"})

	require.ErrorIs(t, err, cancelErr)
	assert.Equal(t, []string{"keys:w1", "stop:local:u1:worker:w1:w1", "cancel:local:u1:worker:w1:w1"}, log.list())
}

func TestChatCleanupCancelsEveryPendingWorkerChatTask(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	tasks := store.NewTaskStore(db)
	ctx := context.Background()
	past := time.Now().Add(-time.Minute).UnixMilli()
	create := func(sessionKey, workerID, taskType string) string {
		t.Helper()
		tk := model.Task{SessionKey: sessionKey, Platform: local.PlatformID, WorkerID: workerID, Instruction: "report", Type: taskType, Status: model.TaskStatusPending}
		switch taskType {
		case model.TaskTypeScheduled:
			tk.CronExpr = "*/5 * * * *"
			tk.NextRunAt = &past
		case model.TaskTypeCountdown:
			tk.ScheduledAt = &past
		}
		id, err := tasks.Create(ctx, tk)
		require.NoError(t, err)
		return id
	}
	key := local.WorkerSessionKey("u1", "w1")
	scheduled := create(key, "w1", model.TaskTypeScheduled)
	countdown := create(key, "w1", model.TaskTypeCountdown)
	otherSchedule := create(local.WorkerSessionKey("u1", "w2"), "w2", model.TaskTypeScheduled)
	cleanup := localchat.NewChatCleanup(store.NewLocalChatStore(db), &fakeStopper{log: &cleanupLog{}}, tasks)

	require.NoError(t, cleanup.DeleteWorkerChats(ctx, model.Worker{ID: "w1"}))

	for _, id := range []string{scheduled, countdown} {
		got, err := tasks.GetByID(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, model.TaskStatusCancelled, got.Status, got.Type)
	}
	got, err := tasks.GetByID(ctx, otherSchedule)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusPending, got.Status)
	due, err := tasks.ClaimDueTasks(ctx, time.Now().UnixMilli(), nil)
	require.NoError(t, err)
	var dueIDs []string
	for _, d := range due {
		dueIDs = append(dueIDs, d.ID)
	}
	assert.Equal(t, []string{otherSchedule}, dueIDs)
}
