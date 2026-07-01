package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/command"
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
		Platform:   "feishu",
		SessionKey: "feishu:chat1:userA",
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
	if !h.IsCommand("/stop") {
		t.Error("expected IsCommand('/stop') = true")
	}
	if h.IsCommand("/stopp") {
		t.Error("expected IsCommand('/stopp') = false")
	}
	if h.IsCommand("/clear") {
		t.Error("expected IsCommand('/clear') = false")
	}
}

func TestStop_BeeRunning_PendingMessages(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: true}
	msgStore := &fakeStopMsgStore{ids: []string{"msg-1", "msg-2"}}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, nil, nil,
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	if !stopper.stopped {
		t.Error("expected StopSession to be called")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	if sender.sent[0] == "" {
		t.Error("expected non-empty reply")
	}
}

func TestStop_BeeRunning_NoMessages(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: true}
	msgStore := &fakeStopMsgStore{ids: nil}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, nil, nil,
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
}

func TestStop_NothingToStop(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: false}
	msgStore := &fakeStopMsgStore{ids: nil}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, nil, nil,
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
}

func TestStop_OnlyMessages(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: false}
	msgStore := &fakeStopMsgStore{ids: []string{"msg-3"}}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, nil, nil,
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
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
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
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

	if len(stop.calls) != 1 || stop.calls[0] != "feishu:chat1:userA::w-1" {
		t.Fatalf("expected StopWorker called for w-1, got %v", stop.calls)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	if !strings.Contains(sender.sent[0], "alice") {
		t.Errorf("expected reply to name the worker, got %q", sender.sent[0])
	}
}

func TestStop_Worker_NothingToStop(t *testing.T) {
	sender := &fakeStopSender{}
	stop := &fakeWorkerStopper{result: session.StopWorkerResult{CancelledTasks: 0}}
	h := newStopWorkerHandler(workerLookup("alice", model.Worker{ID: "w-1", Name: "alice"}), stop, sender)

	h.HandleCommand(context.Background(), "/stop alice", makeStopReplyTo())

	if len(stop.calls) != 1 {
		t.Fatalf("expected StopWorker to be called, got %v", stop.calls)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
}

func TestStop_Worker_NotFound(t *testing.T) {
	sender := &fakeStopSender{}
	stop := &fakeWorkerStopper{}
	h := newStopWorkerHandler(workerLookup("alice" /* no workers */), stop, sender)

	h.HandleCommand(context.Background(), "/stop bob", makeStopReplyTo())

	if len(stop.calls) != 0 {
		t.Errorf("expected StopWorker NOT to be called, got %v", stop.calls)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
}

func TestStop_Worker_Duplicate(t *testing.T) {
	sender := &fakeStopSender{}
	stop := &fakeWorkerStopper{}
	h := newStopWorkerHandler(workerLookup("alice",
		model.Worker{ID: "w-1", Name: "alice"},
		model.Worker{ID: "w-2", Name: "alice"}), stop, sender)

	h.HandleCommand(context.Background(), "/stop alice", makeStopReplyTo())

	if len(stop.calls) != 0 {
		t.Errorf("expected StopWorker NOT to be called on ambiguous name, got %v", stop.calls)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
}

func TestStop_Worker_LookupError(t *testing.T) {
	sender := &fakeStopSender{}
	stop := &fakeWorkerStopper{}
	lookup := &fakeClearWorkerLookup{fakeWorkerByIDsLookup: &fakeWorkerByIDsLookup{err: errors.New("db down")}}
	h := newStopWorkerHandler(lookup, stop, sender)

	h.HandleCommand(context.Background(), "/stop alice", makeStopReplyTo())

	if len(stop.calls) != 0 {
		t.Errorf("expected StopWorker NOT to be called on lookup error, got %v", stop.calls)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
}

func TestStop_TooManyArgs(t *testing.T) {
	sender := &fakeStopSender{}
	stop := &fakeWorkerStopper{}
	h := newStopWorkerHandler(workerLookup("alice"), stop, sender)

	handled := h.HandleCommand(context.Background(), "/stop alice bob", makeStopReplyTo())

	if !handled {
		t.Error("expected /stop with extra args to be handled")
	}
	if len(stop.calls) != 0 {
		t.Errorf("expected StopWorker NOT to be called, got %v", stop.calls)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 usage reply, got %d", len(sender.sent))
	}
}
