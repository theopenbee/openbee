package command_test

import (
	"context"
	"strings"
	"testing"
	"time"

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

func TestStatusCommand_HappyPath(t *testing.T) {
	now := time.Now().Unix()
	agents := []store.SessionAgent{
		{AgentID: "w1", AgentType: "worker", Engine: "claude", Name: "貂蝉", UpdatedAt: now - 120},   // 2m
		{AgentID: "w2", AgentType: "worker", Engine: "codex", Name: "吕布", UpdatedAt: now - 18000}, // 5h
	}
	nowMs := time.Now().UnixMilli()
	tasks := []model.Task{
		{ID: "t1", WorkerID: "w1", Instruction: "新增 /status 指令的实现", ExecutionID: "a1b2c3d4e5f6", CreatedAt: nowMs - 83000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
		{ID: "t2", WorkerID: "w2", Instruction: "修复登录 bug", ExecutionID: "e5f6a7b89999", CreatedAt: nowMs - 12000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	workers := map[string]model.Worker{
		"w1": {ID: "w1", Name: "貂蝉"},
		"w2": {ID: "w2", Name: "吕布"},
	}
	h, sender := makeStatusHandler(agents, tasks, workers)
	handled := h.HandleCommand(context.Background(), "/status", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	out := sender.sent[0]

	// Header + section headers
	for _, want := range []string{
		"当前会话状态：",
		"已激活 bee（2）：",
		"进行中任务（2）：",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// Bee lines
	for _, want := range []string{
		"- 貂蝉   引擎: claude   最近活跃: 2m 前",
		"- 吕布   引擎: codex   最近活跃: 5h 前",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// Task lines
	for _, want := range []string{
		"- [貂蝉] 新增 /status 指令的实现   已运行 1m   exec: a1b2c3d4",
		"- [吕布] 修复登录 bug   已运行 12s   exec: e5f6a7b8",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestStatusCommand_EmptyBeesAndTasks(t *testing.T) {
	h, sender := makeStatusHandler(nil, nil, nil)
	h.HandleCommand(context.Background(), "/status", makeReplyTo())
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	out := sender.sent[0]
	for _, want := range []string{
		"已激活 bee（0）：",
		"进行中任务（0）：",
		"  (无)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	// "(无)" must appear at least twice (one per empty section).
	if strings.Count(out, "  (无)") != 2 {
		t.Errorf("expected exactly two empty markers, got:\n%s", out)
	}
}

func TestStatusCommand_BeesOnly_NoTasks(t *testing.T) {
	now := time.Now().Unix()
	agents := []store.SessionAgent{
		{AgentID: "w1", AgentType: "worker", Engine: "claude", Name: "貂蝉", UpdatedAt: now - 30},
	}
	h, sender := makeStatusHandler(agents, nil, map[string]model.Worker{"w1": {ID: "w1", Name: "貂蝉"}})
	h.HandleCommand(context.Background(), "/status", makeReplyTo())
	out := sender.sent[0]
	if !strings.Contains(out, "已激活 bee（1）：") {
		t.Errorf("missing bees header\n%s", out)
	}
	if !strings.Contains(out, "进行中任务（0）：") {
		t.Errorf("missing tasks header\n%s", out)
	}
	if strings.Count(out, "  (无)") != 1 {
		t.Errorf("expected exactly one empty marker, got:\n%s", out)
	}
}

func TestStatusCommand_TasksOnly_NoBees(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	tasks := []model.Task{
		{ID: "t1", WorkerID: "w1", Instruction: "do thing", ExecutionID: "deadbeef0000", CreatedAt: nowMs - 5000, Status: model.TaskStatusRunning, Type: model.TaskTypeImmediate},
	}
	workers := map[string]model.Worker{"w1": {ID: "w1", Name: "貂蝉"}}
	h, sender := makeStatusHandler(nil, tasks, workers)
	h.HandleCommand(context.Background(), "/status", makeReplyTo())
	out := sender.sent[0]
	if !strings.Contains(out, "已激活 bee（0）：") {
		t.Errorf("missing bees header\n%s", out)
	}
	if !strings.Contains(out, "进行中任务（1）：") {
		t.Errorf("missing tasks header\n%s", out)
	}
	if !strings.Contains(out, "[貂蝉] do thing") {
		t.Errorf("missing task line\n%s", out)
	}
	if strings.Count(out, "  (无)") != 1 {
		t.Errorf("expected exactly one empty marker, got:\n%s", out)
	}
}
