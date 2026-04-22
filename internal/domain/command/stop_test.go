package command_test

import (
	"context"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/command"
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

type fakeStopNotifier struct {
	notified []string
}

func (f *fakeStopNotifier) NotifyTaskFailure(_ context.Context, messageID string, _ model.FailureInfo) error {
	f.notified = append(f.notified, messageID)
	return nil
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
		&fakeStopNotifier{},
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
	notifier := &fakeStopNotifier{}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, notifier,
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	if !stopper.stopped {
		t.Error("expected StopSession to be called")
	}
	if len(notifier.notified) != 2 {
		t.Errorf("expected 2 notifications, got %d", len(notifier.notified))
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	reply := sender.sent[0]
	if reply == "" {
		t.Error("expected non-empty reply")
	}
}

func TestStop_BeeRunning_NoMessages(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: true}
	msgStore := &fakeStopMsgStore{ids: nil}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, &fakeStopNotifier{},
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

	h := command.NewStopCommandHandler(stopper, msgStore, &fakeStopNotifier{},
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
}

func TestStop_OnlyMessages(t *testing.T) {
	stopper := &fakeBeeStopper{wasRunning: false}
	msgStore := &fakeStopMsgStore{ids: []string{"msg-3"}}
	notifier := &fakeStopNotifier{}
	sender := &fakeStopSender{}

	h := command.NewStopCommandHandler(stopper, msgStore, notifier,
		map[string]platform.PlatformSenderAdapter{"feishu": sender})
	h.HandleCommand(context.Background(), "/stop", makeStopReplyTo())

	if len(notifier.notified) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifier.notified))
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
}
