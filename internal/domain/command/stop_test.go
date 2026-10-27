package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

type fakeBeeStopper struct {
	stopped    bool
	wasRunning bool
}

func (f *fakeBeeStopper) StopSession(_ string) bool {
	f.stopped = true
	return f.wasRunning
}

type fakeStopMsgStore struct {
	ids []string
	err error
}

func (f *fakeStopMsgStore) FailReceived(_ context.Context, _ string) ([]string, error) {
	return f.ids, f.err
}

type fakeStopSender struct {
	sent []string
}

func (f *fakeStopSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	f.sent = append(f.sent, msg.Content)
	return nil
}

func makeStopReplyTo() platform.InboundMessage {
	return platform.InboundMessage{
		Platform:   "beta",
		SessionKey: "beta:chat1:userA",
		Content:    "/stop",
	}
}

func TestStop_IsCommand(t *testing.T) {
	h := command.NewStopCommandHandler(
		&fakeBeeStopper{},
		&fakeStopMsgStore{},
		nil,
		nil,
		nil,
	)
	assert.True(t, h.IsCommand("/stop"), "expected IsCommand('/stop') = true")
	assert.False(t, h.IsCommand("/stopp"), "expected IsCommand('/stopp') = false")
	assert.False(t, h.IsCommand("/clear"), "expected IsCommand('/clear') = false")
}

func TestStop_BeeRunning_PendingMessages(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: true}
	msgStore := &fakeStopMsgStore{ids: []string{"msg-1", "msg-2"}}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, nil, nil,
		platform.SenderMap{"beta": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	assert.True(t, stopper.stopped, "expected StopSession to be called")
	require.Len(t, sender.sent, 1)
	assert.NotEmpty(t, sender.sent[0], "expected non-empty reply")
}

func TestStop_BeeRunning_NoMessages(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: true}
	msgStore := &fakeStopMsgStore{ids: nil}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, nil, nil,
		platform.SenderMap{"beta": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	require.Len(t, sender.sent, 1)
}

func TestStop_NothingToStop(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: false}
	msgStore := &fakeStopMsgStore{ids: nil}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, nil, nil,
		platform.SenderMap{"beta": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	require.Len(t, sender.sent, 1)
}

func TestStop_OnlyMessages(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: false}
	msgStore := &fakeStopMsgStore{ids: []string{"msg-3"}}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, nil, nil,
		platform.SenderMap{"beta": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	require.Len(t, sender.sent, 1)
}

type fakeWorkerStopper struct {
	result session.StopWorkerResult
	err    error
	calls  []string // sessionKey::workerID
}

func (f *fakeWorkerStopper) StopWorker(_ context.Context, sessionKey string, w model.Worker) (session.StopWorkerResult, error) {
	f.calls = append(f.calls, sessionKey+"::"+w.ID)
	return f.result, f.err
}

func newStopWorkerHandler(workers command.WorkerNameLookup, stop command.WorkerStopper, sender *fakeStopSender) *command.StopCommandHandler {
	return command.NewStopCommandHandler(&fakeBeeStopper{}, &fakeStopMsgStore{}, workers, stop,
		platform.SenderMap{"beta": sender})
}

func workerLookup(name string, workers ...model.Worker) *fakeClearWorkerLookup {
	return &fakeClearWorkerLookup{
		fakeWorkerByIDsLookup: &fakeWorkerByIDsLookup{},
		byName:                map[string][]model.Worker{name: workers},
	}
}

func TestStop_Worker_StopsTasks(t *testing.T) {
	sender := &fakeStopSender{}
	stop := &fakeWorkerStopper{result: session.StopWorkerResult{CancelledTasks: 2}}
	h := newStopWorkerHandler(workerLookup("alice", model.Worker{ID: "w-1", Name: "alice"}), stop, sender)

	h.HandleCommand(context.Background(), "/stop alice", makeStopReplyTo())

	require.Equal(t, []string{"beta:chat1:userA::w-1"}, stop.calls)
	require.Len(t, sender.sent, 1)
	assert.Contains(t, sender.sent[0], "alice")
}

func TestStop_WorkerFromBeeChatTargetsWorkerChat(t *testing.T) {
	const workerKey = "local:u1:worker:w-1"
	fx := makeClearFixture(nil, nil, nil)
	fx.tasks.workerTasks = map[string][]model.Task{
		"w-1": {{ID: "t1", WorkerID: "w-1", SessionKey: workerKey, Type: model.TaskTypeImmediate, Status: model.TaskStatusRunning}},
	}
	fx.tasks.workerCancelled = map[string]int64{"w-1": 1}
	execs := fakeRunningExecs{"t1": "exec-1"}
	svc := session.NewClearService(session.ClearServiceDeps{
		Sessions:     fx.sessions,
		Tasks:        fx.tasks,
		ExecStopper:  fx.stopper,
		Dispatcher:   fx.disp,
		RunningExecs: execs,
		EngineCfg:    enginecfg.NewStore("claude"),
	})
	sender := &fakeStopSender{}
	h := newStopWorkerHandler(workerLookup("alice", model.Worker{ID: "w-1", Name: "alice"}), svc, sender)

	replyTo := makeStopReplyTo()
	replyTo.SessionKey = "local:u1"
	h.HandleCommand(context.Background(), "/stop alice", replyTo)

	require.Len(t, sender.sent, 1)
	assert.Equal(t, []string{workerKey + "::w-1"}, fx.tasks.workerListCalls)
	assert.Equal(t, []string{workerKey + "::w-1"}, fx.tasks.workerCancelCalls)
	assert.Equal(t, []string{workerKey + "::w-1"}, fx.disp.clearedWorkers)
	assert.Equal(t, []string{"exec-1"}, fx.stopper.stopped)
	assert.Empty(t, fx.sessions.deletedSessions)
}

func TestStop_Worker_NothingToStop(t *testing.T) {
	sender := &fakeStopSender{}
	stop := &fakeWorkerStopper{result: session.StopWorkerResult{CancelledTasks: 0}}
	h := newStopWorkerHandler(workerLookup("alice", model.Worker{ID: "w-1", Name: "alice"}), stop, sender)

	h.HandleCommand(context.Background(), "/stop alice", makeStopReplyTo())

	require.Len(t, stop.calls, 1)
	require.Len(t, sender.sent, 1)
}

// TestStop_Worker_Errors merges the three /stop-worker error paths that must
// not call StopWorker: unknown name, ambiguous (duplicate) name, and a
// lookup-store error.
func TestStop_Worker_Errors(t *testing.T) {
	cases := []struct {
		name   string
		lookup *fakeClearWorkerLookup
		cmd    string
	}{
		{
			name:   "NotFound",
			lookup: workerLookup("alice" /* no workers */),
			cmd:    "/stop bob",
		},
		{
			name: "Duplicate",
			lookup: workerLookup("alice",
				model.Worker{ID: "w-1", Name: "alice"},
				model.Worker{ID: "w-2", Name: "alice"}),
			cmd: "/stop alice",
		},
		{
			name:   "LookupError",
			lookup: &fakeClearWorkerLookup{fakeWorkerByIDsLookup: &fakeWorkerByIDsLookup{err: errors.New("db down")}},
			cmd:    "/stop alice",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sender := &fakeStopSender{}
			stop := &fakeWorkerStopper{}
			h := newStopWorkerHandler(tc.lookup, stop, sender)

			h.HandleCommand(context.Background(), tc.cmd, makeStopReplyTo())

			assert.Empty(t, stop.calls, "expected StopWorker NOT to be called")
			require.Len(t, sender.sent, 1)
		})
	}
}

func TestStop_TooManyArgs(t *testing.T) {
	sender := &fakeStopSender{}
	stop := &fakeWorkerStopper{}
	h := newStopWorkerHandler(workerLookup("alice"), stop, sender)

	handled := h.HandleCommand(context.Background(), "/stop alice bob", makeStopReplyTo())

	assert.True(t, handled, "expected /stop with extra args to be handled")
	assert.Empty(t, stop.calls)
	require.Len(t, sender.sent, 1)
}
