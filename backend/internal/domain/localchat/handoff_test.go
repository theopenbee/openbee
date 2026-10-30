package localchat_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/theopenbee/openbee/internal/domain/localchat"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
)

type recordingSender struct {
	mu   sync.Mutex
	sent []platform.OutboundMessage
	err  error
}

func (s *recordingSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, msg)
	return s.err
}

func (s *recordingSender) messages() []platform.OutboundMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]platform.OutboundMessage(nil), s.sent...)
}

type failingTasks struct{ err error }

func (f failingTasks) Create(context.Context, model.Task) (string, error) { return "", f.err }

type handoffEnv struct {
	db     *sql.DB
	msgs   *store.MessageStore
	tasks  *store.TaskStore
	sender *recordingSender
	events <-chan string
	h      *localchat.Handoff
}

func newHandoffEnv(t *testing.T, tasks localchat.HandoffTasks) *handoffEnv {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	hub := local.NewSSEHub()
	events, unsubscribe := hub.Subscribe("u1")
	t.Cleanup(unsubscribe)

	env := &handoffEnv{
		db:     db,
		msgs:   store.NewMessageStore(db),
		tasks:  store.NewTaskStore(db),
		sender: &recordingSender{},
		events: events,
	}
	if tasks == nil {
		tasks = env.tasks
	}
	env.h = localchat.NewHandoff(env.msgs, tasks, env.sender, local.NewEventPublisher(hub))
	return env
}

func (e *handoffEnv) nextEvent(t *testing.T) local.Event {
	t.Helper()
	select {
	case raw := <-e.events:
		var ev local.Event
		require.NoError(t, json.Unmarshal([]byte(raw), &ev))
		return ev
	default:
		t.Fatal("expected an event, got none")
		return local.Event{}
	}
}

func (e *handoffEnv) countRows(t *testing.T, sessionKey string) int {
	t.Helper()
	var n int
	require.NoError(t, e.db.QueryRow(
		`SELECT COUNT(*) FROM bee_platform_messages WHERE session_key = ?`, sessionKey).Scan(&n))
	return n
}

func beeTask(workerID, instruction string) model.Task {
	return model.Task{
		MessageID:   "origin-msg",
		SessionKey:  "local:u1",
		Platform:    local.PlatformID,
		WorkerID:    workerID,
		Instruction: instruction,
		Type:        model.TaskTypeImmediate,
		Status:      model.TaskStatusPending,
	}
}

func TestHandoffCreatesWorkerRowTaskAndCard(t *testing.T) {
	env := newHandoffEnv(t, nil)
	ctx := context.Background()

	taskID, err := env.h.Create(ctx, beeTask("w1", "write the report"))
	require.NoError(t, err)
	require.NotEmpty(t, taskID)

	workerKey := "local:u1:worker:w1"
	got, err := env.tasks.GetByID(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, workerKey, got.SessionKey)
	assert.Equal(t, local.PlatformID, got.Platform)
	assert.Equal(t, "w1", got.WorkerID)
	assert.Equal(t, "write the report", got.Instruction)
	assert.NotEqual(t, "origin-msg", got.MessageID)

	var sessionKey, platformID, content, senderType, status string
	require.NoError(t, env.db.QueryRow(
		`SELECT session_key, platform, content, sender_type, status FROM bee_platform_messages WHERE id = ?`,
		got.MessageID).Scan(&sessionKey, &platformID, &content, &senderType, &status))
	assert.Equal(t, workerKey, sessionKey)
	assert.Equal(t, local.PlatformID, platformID)
	assert.Equal(t, "write the report", content)
	assert.Equal(t, store.SenderTypeBee, senderType)
	assert.Equal(t, store.MsgStatusBeeProcessed, status)

	cards := env.sender.messages()
	require.Len(t, cards, 1)
	assert.Equal(t, "local:u1", cards[0].SessionKey)
	assert.Equal(t, store.SourceTypeBee, cards[0].SourceType)
	assert.Equal(t, store.OutboundKindHandoff, cards[0].Kind)
	assert.Equal(t, "write the report", cards[0].Content)
	assert.Equal(t, "origin-msg", cards[0].InboundMsgID)
	var info local.HandoffInfo
	require.NoError(t, json.Unmarshal([]byte(cards[0].Payload), &info))
	assert.Equal(t, taskID, info.TaskID)
	assert.Equal(t, "w1", info.WorkerID)
	assert.Equal(t, model.TaskTypeImmediate, info.TaskType)
	assert.Equal(t, "worker:w1", info.ConversationID)

	ev := env.nextEvent(t)
	assert.Equal(t, local.EventMessage, ev.Type)
	assert.Equal(t, "worker:w1", ev.ConversationID)
	require.NotNil(t, ev.Message)
	assert.Equal(t, got.MessageID, ev.Message.ID)
	assert.Equal(t, local.Sender{Type: local.SenderBee}, ev.Message.Sender)
	assert.Equal(t, local.KindMessage, ev.Message.Kind)
	assert.Equal(t, "write the report", ev.Message.Content)
	assert.NotZero(t, ev.Message.TS)
}

func TestHandoffCardCarriesScheduleFields(t *testing.T) {
	at := int64(1893456000000)
	countdown := beeTask("w1", "remind me")
	countdown.Type = model.TaskTypeCountdown
	countdown.ScheduledAt = &at
	scheduled := beeTask("w1", "daily digest")
	scheduled.Type = model.TaskTypeScheduled
	scheduled.CronExpr = "0 9 * * *"
	scheduled.NextRunAt = &at

	cases := []struct {
		name string
		task model.Task
	}{
		{"countdown", countdown},
		{"scheduled", scheduled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newHandoffEnv(t, nil)

			_, err := env.h.Create(context.Background(), tc.task)
			require.NoError(t, err)

			cards := env.sender.messages()
			require.Len(t, cards, 1)
			var info local.HandoffInfo
			require.NoError(t, json.Unmarshal([]byte(cards[0].Payload), &info))
			assert.Equal(t, tc.task.Type, info.TaskType)
			assert.Equal(t, tc.task.CronExpr, info.CronExpr)
			assert.Equal(t, tc.task.ScheduledAt, info.ScheduledAt)
		})
	}
}

func TestHandoffRollsBackOnTaskError(t *testing.T) {
	boom := errors.New("task store down")
	env := newHandoffEnv(t, failingTasks{err: boom})

	taskID, err := env.h.Create(context.Background(), beeTask("w1", "write the report"))

	require.ErrorIs(t, err, boom)
	assert.Empty(t, taskID)
	assert.Zero(t, env.countRows(t, "local:u1:worker:w1"))
	assert.Empty(t, env.sender.messages())
	select {
	case raw := <-env.events:
		t.Fatalf("unexpected event after rollback: %s", raw)
	default:
	}
}

func TestHandoffCardSendFailureIsNotReturned(t *testing.T) {
	env := newHandoffEnv(t, nil)
	env.sender.err = errors.New("sse down")

	taskID, err := env.h.Create(context.Background(), beeTask("w1", "write the report"))

	require.NoError(t, err)
	require.NotEmpty(t, taskID)
	assert.Equal(t, 1, env.countRows(t, "local:u1:worker:w1"))
	assert.Len(t, env.sender.messages(), 1)
}

func TestHandoffRejectsNonBeeSessionKey(t *testing.T) {
	env := newHandoffEnv(t, nil)
	task := beeTask("w1", "write the report")
	task.SessionKey = "local:u1:worker:w2"

	_, err := env.h.Create(context.Background(), task)

	require.Error(t, err)
	assert.Zero(t, env.countRows(t, "local:u1:worker:w1"))
	assert.Empty(t, env.sender.messages())
}

func TestAppliesOnlyToLocalBeeChat(t *testing.T) {
	cases := []struct {
		name       string
		platformID string
		sessionKey string
		want       bool
	}{
		{"local bee chat", "local", "local:u1", true},
		{"local worker chat", "local", "local:u1:worker:w1", false},
		{"telegram", "telegram", "telegram:c1:u1", false},
		{"local platform with foreign key", "local", "telegram:c1:u1", false},
		{"telegram platform with local key", "telegram", "local:u1", false},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, localchat.Applies(tc.platformID, tc.sessionKey))
		})
	}
}
