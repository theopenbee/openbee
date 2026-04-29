package command_test

import (
	"context"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// --- fakes for /status ---

type fakeStatusSessionLister struct {
	agents []store.SessionAgent
	err    error
}

func (f *fakeStatusSessionLister) ListActiveSessionContexts(_ context.Context, _, _ string) ([]store.SessionAgent, error) {
	return f.agents, f.err
}

type fakeStatusTaskLister struct {
	tasks []model.Task
	err   error
}

func (f *fakeStatusTaskLister) ListBySessionKey(_ context.Context, _, _, _ string) ([]model.Task, error) {
	return f.tasks, f.err
}

type fakeStatusWorkerLookup struct {
	byID map[string]model.Worker
	err  error
}

func (f *fakeStatusWorkerLookup) GetByID(id string) (model.Worker, error) {
	if f.err != nil {
		return model.Worker{}, f.err
	}
	w, ok := f.byID[id]
	if !ok {
		return model.Worker{Name: ""}, nil
	}
	return w, nil
}

func makeStatusHandler(
	agents []store.SessionAgent,
	tasks []model.Task,
	workers map[string]model.Worker,
) (*command.StatusCommandHandler, *fakeSender) {
	sender := &fakeSender{}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	sessions := &fakeStatusSessionLister{agents: agents}
	taskList := &fakeStatusTaskLister{tasks: tasks}
	wl := &fakeStatusWorkerLookup{byID: workers}
	engineCfg := enginecfg.NewStore("claude")
	h := command.NewStatusCommandHandler(sessions, taskList, wl, senders, engineCfg)
	return h, sender
}

func TestStatusCommand_IsCommand(t *testing.T) {
	h, _ := makeStatusHandler(nil, nil, nil)
	cases := map[string]bool{
		"/status":   true,
		"/status x": true,
		"/statuses": false,
		"hello":     false,
		"":          false,
	}
	for input, want := range cases {
		if got := h.IsCommand(input); got != want {
			t.Errorf("IsCommand(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestStatusCommand_UsageOnExtraArgs(t *testing.T) {
	h, sender := makeStatusHandler(nil, nil, nil)
	handled := h.HandleCommand(context.Background(), "/status x", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 || sender.sent[0] != "用法：/status" {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}
