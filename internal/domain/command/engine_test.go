package command_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

func TestMain(m *testing.M) {
	if err := i18n.Load("zh"); err != nil {
		panic("failed to load zh locale: " + err.Error())
	}
	os.Exit(m.Run())
}

// --- fakes ---

type fakeWorkerRepo struct {
	workers   map[string]model.Worker // name → worker
	updateErr error
}

func (f *fakeWorkerRepo) GetByName(name string) (model.Worker, error) {
	w, ok := f.workers[name]
	if !ok {
		return model.Worker{}, fmt.Errorf("get worker by name: %w", sql.ErrNoRows)
	}
	return w, nil
}
func (f *fakeWorkerRepo) UpdateEngine(_ string, _ string) error {
	return f.updateErr
}

type fakeSysConfig struct {
	vals   map[string]string
	setErr error
}

func (f *fakeSysConfig) Set(_ context.Context, key, value string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.vals[key] = value
	return nil
}

type fakeSender struct {
	sent []string
}

func (f *fakeSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	f.sent = append(f.sent, msg.Content)
	return nil
}

type fakeValidator struct {
	engines []string
}

func (v *fakeValidator) ValidateEngine(name string) error {
	for _, e := range v.engines {
		if e == name {
			return nil
		}
	}
	return fmt.Errorf("unknown engine %q", name)
}

func (v *fakeValidator) EnabledEngines() []string { return v.engines }

type fakeBeeBusyChecker struct {
	activeMessages bool
	activeBeeExecs bool
	err            error
}

func (f *fakeBeeBusyChecker) HasActiveMessages(_ context.Context) (bool, error) {
	return f.activeMessages, f.err
}
func (f *fakeBeeBusyChecker) HasActiveBeeExecutions(_ context.Context) (bool, error) {
	return f.activeBeeExecs, f.err
}

type fakeWorkerBusyChecker struct {
	activeExecs bool
	activeTasks bool
	err         error
}

func (f *fakeWorkerBusyChecker) HasActiveExecutionsByWorkerID(_ context.Context, _ string) (bool, error) {
	return f.activeExecs, f.err
}
func (f *fakeWorkerBusyChecker) HasActiveImmediateTasksByWorkerID(_ context.Context, _ string) (bool, error) {
	return f.activeTasks, f.err
}

var defaultValidator = &fakeValidator{engines: ai.AllEngines()}
var notBeeBusy = &fakeBeeBusyChecker{}
var notWorkerBusy = &fakeWorkerBusyChecker{}

func makeReplyTo() platform.InboundMessage {
	return platform.InboundMessage{
		Platform:   "feishu",
		SessionKey: "feishu:chat1:user1",
	}
}

func makeHandler(workers map[string]model.Worker) (*command.EngineCommandHandler, *fakeSender, *fakeSysConfig, *enginecfg.Store) {
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	engineCfg := enginecfg.NewStore("")
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, engineCfg)
	return h, sender, cfg, engineCfg
}

// --- tests ---

func TestEngineCommand_NotACommand(t *testing.T) {
	h, sender, _, _ := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "hello world", makeReplyTo())
	assert.False(t, handled, "should not handle non-command")
	assert.Empty(t, sender.sent, "should not send reply for non-command")
}

func TestEngineCommand_SwitchBeeEngine(t *testing.T) {
	h, sender, cfg, engineCfg := makeHandler(nil)
	engineCfg.Set("claude")
	handled := h.HandleCommand(context.Background(), "/engine codex", makeReplyTo())
	require.True(t, handled, "expected handled=true")
	assert.Equal(t, "codex", engineCfg.Get())
	assert.Equal(t, "codex", cfg.vals[model.SystemConfigKeyDefaultEngine])
	require.Len(t, sender.sent, 1, "expected one reply")
	assert.Equal(t, "已将默认 engine 切换为 codex", sender.sent[0])
}

func TestEngineCommand_SwitchWorkerEngine(t *testing.T) {
	workers := map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}}
	h, sender, _, _ := makeHandler(workers)
	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	require.True(t, handled, "expected handled=true")
	assert.Equal(t, []string{`已将员工 "alice" 的 engine 切换为 codex`}, sender.sent)
}

func TestEngineCommand_InvalidEngine(t *testing.T) {
	h, sender, _, _ := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "/engine xyz", makeReplyTo())
	require.True(t, handled, "expected handled=true")
	require.Len(t, sender.sent, 1, "expected one reply")
	assert.Equal(t, "未知的 engine: xyz，支持的 engine：claude / codex / pi", sender.sent[0])
}

func TestEngineCommand_WorkerNotFound(t *testing.T) {
	h, sender, _, _ := makeHandler(map[string]model.Worker{})
	handled := h.HandleCommand(context.Background(), "/engine claude nobody", makeReplyTo())
	require.True(t, handled, "expected handled=true")
	assert.Equal(t, []string{`员工 "nobody" 不存在`}, sender.sent)
}

func TestEngineCommand_NoArgs(t *testing.T) {
	h, sender, _, _ := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "/engine", makeReplyTo())
	require.True(t, handled, "expected handled=true")
	assert.Equal(t, []string{"用法：\n/engine {engine} — 切换默认 engine\n/engine {engine} {workerName} — 切换指定员工的 engine"}, sender.sent)
}

// TestEngineCommand_Busy merges the busy-checker scenarios that used to be
// separate tests: each case builds its own bee/worker busy checkers and
// verifies the resulting reply.
func TestEngineCommand_Busy(t *testing.T) {
	cases := []struct {
		name       string
		workers    map[string]model.Worker
		beeBusy    command.BeeBusyChecker
		workerBusy command.WorkerBusyChecker
		updateErr  error
		cmd        string
		wantReply  string
	}{
		{
			name:       "BeeBusy_ActiveMessages",
			workers:    map[string]model.Worker{},
			beeBusy:    command.NewBeeBusyChecker(&fakeBeeBusyChecker{activeMessages: true}, notBeeBusy),
			workerBusy: command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy),
			cmd:        "/engine codex",
			wantReply:  "当前有消息正在接收或处理中，无法切换引擎，请等待完成后再试。",
		},
		{
			name:       "BeeBusy_ActiveBeeExecutions",
			workers:    map[string]model.Worker{},
			beeBusy:    command.NewBeeBusyChecker(notBeeBusy, &fakeBeeBusyChecker{activeBeeExecs: true}),
			workerBusy: command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy),
			cmd:        "/engine codex",
			wantReply:  "当前有执行中的 execution，无法切换引擎，请等待完成后再试。",
		},
		{
			name:       "WorkerBusy_ActiveExecutions",
			workers:    map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}},
			beeBusy:    command.NewBeeBusyChecker(notBeeBusy, notBeeBusy),
			workerBusy: command.NewWorkerBusyChecker(&fakeWorkerBusyChecker{activeExecs: true}, notWorkerBusy),
			cmd:        "/engine codex alice",
			wantReply:  "当前有执行中的 execution，无法切换引擎，请等待完成后再试。",
		},
		{
			name:       "WorkerBusy_ActiveTasks",
			workers:    map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}},
			beeBusy:    command.NewBeeBusyChecker(notBeeBusy, notBeeBusy),
			workerBusy: command.NewWorkerBusyChecker(notWorkerBusy, &fakeWorkerBusyChecker{activeTasks: true}),
			cmd:        "/engine codex alice",
			wantReply:  "当前有即时任务正在等待或执行中，无法切换引擎，请等待完成后再试。",
		},
		{
			// KEY scenario: alice is free, but bee is busy — alice's switch must succeed.
			name:       "WorkerSwitch_NotBlockedByOtherWorker",
			workers:    map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}},
			beeBusy:    command.NewBeeBusyChecker(&fakeBeeBusyChecker{activeMessages: true}, notBeeBusy),
			workerBusy: command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy),
			cmd:        "/engine codex alice",
			wantReply:  `已将员工 "alice" 的 engine 切换为 codex`,
		},
		{
			// No target (bee or worker) is named, so busy checks never run.
			name:       "BusyDoesNotBlockUsage",
			beeBusy:    command.NewBeeBusyChecker(&fakeBeeBusyChecker{activeMessages: true, activeBeeExecs: true}, &fakeBeeBusyChecker{activeBeeExecs: true}),
			workerBusy: command.NewWorkerBusyChecker(&fakeWorkerBusyChecker{activeExecs: true}, &fakeWorkerBusyChecker{activeTasks: true}),
			cmd:        "/engine",
			wantReply:  "用法：\n/engine {engine} — 切换默认 engine\n/engine {engine} {workerName} — 切换指定员工的 engine",
		},
		{
			name:       "SwitchWorkerEngine_UpdateError",
			workers:    map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}},
			beeBusy:    command.NewBeeBusyChecker(notBeeBusy, notBeeBusy),
			workerBusy: command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy),
			updateErr:  errors.New("update error"),
			cmd:        "/engine codex alice",
			wantReply:  "切换失败，请稍后重试",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sender := &fakeSender{}
			cfg := &fakeSysConfig{vals: make(map[string]string)}
			repo := &fakeWorkerRepo{workers: tc.workers, updateErr: tc.updateErr}
			senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
			h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, tc.beeBusy, tc.workerBusy, enginecfg.NewStore(""))

			handled := h.HandleCommand(context.Background(), tc.cmd, makeReplyTo())
			require.True(t, handled, "expected handled=true")
			assert.Equal(t, []string{tc.wantReply}, sender.sent)
		})
	}
}

func TestEngineCommand_SwitchBeeEngine_DBError(t *testing.T) {
	engineCfg := enginecfg.NewStore("claude")
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string), setErr: errors.New("db error")}
	repo := &fakeWorkerRepo{workers: map[string]model.Worker{}}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, engineCfg)

	handled := h.HandleCommand(context.Background(), "/engine codex", makeReplyTo())
	require.True(t, handled, "expected handled=true")
	assert.Equal(t, "claude", engineCfg.Get())
	assert.Equal(t, []string{"切换失败，请稍后重试"}, sender.sent)
}
