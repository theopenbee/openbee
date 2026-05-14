package command_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/theopenbee/openbee/internal/bridge"
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

var defaultValidator = &fakeValidator{engines: bridge.AllEngines()}
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
	if handled {
		t.Error("should not handle non-command")
	}
	if len(sender.sent) != 0 {
		t.Error("should not send reply for non-command")
	}
}

func TestEngineCommand_SwitchBeeEngine(t *testing.T) {
	h, sender, cfg, engineCfg := makeHandler(nil)
	engineCfg.Set("claude")
	handled := h.HandleCommand(context.Background(), "/engine codex", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if engineCfg.Get() != "codex" {
		t.Errorf("expected engineCfg=codex, got %s", engineCfg.Get())
	}
	if cfg.vals[model.SystemConfigKeyDefaultEngine] != "codex" {
		t.Errorf("expected DB updated to codex, got %s", cfg.vals[model.SystemConfigKeyDefaultEngine])
	}
	if len(sender.sent) != 1 {
		t.Fatal("expected one reply")
	}
	if sender.sent[0] != "已将默认 engine 切换为 codex" {
		t.Errorf("unexpected reply: %s", sender.sent[0])
	}
}

func TestEngineCommand_SwitchWorkerEngine(t *testing.T) {
	workers := map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}}
	h, sender, _, _ := makeHandler(workers)
	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 || sender.sent[0] != `已将 Worker "alice" 的 engine 切换为 codex` {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_InvalidEngine(t *testing.T) {
	h, sender, _, _ := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "/engine xyz", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 {
		t.Fatal("expected one reply")
	}
	want := "未知的 engine: xyz，支持的 engine：claude / codex / pi / kimi"
	if sender.sent[0] != want {
		t.Errorf("unexpected reply:\ngot  %s\nwant %s", sender.sent[0], want)
	}
}

func TestEngineCommand_WorkerNotFound(t *testing.T) {
	h, sender, _, _ := makeHandler(map[string]model.Worker{})
	handled := h.HandleCommand(context.Background(), "/engine claude nobody", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := `Worker "nobody" 不存在`
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_NoArgs(t *testing.T) {
	h, sender, _, _ := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "/engine", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "用法：\n/engine {engine} — 切换默认 engine\n/engine {engine} {workerName} — 切换指定 worker 的 engine"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_BeeBusy_ActiveMessages(t *testing.T) {
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: map[string]model.Worker{}}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(&fakeBeeBusyChecker{activeMessages: true}, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "当前有消息正在接收或处理中，无法切换引擎，请等待完成后再试。"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_BeeBusy_ActiveBeeExecutions(t *testing.T) {
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: map[string]model.Worker{}}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, &fakeBeeBusyChecker{activeBeeExecs: true})
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "当前有执行中的 execution，无法切换引擎，请等待完成后再试。"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_WorkerBusy_ActiveExecutions(t *testing.T) {
	workers := map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}}
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(&fakeWorkerBusyChecker{activeExecs: true}, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "当前有执行中的 execution，无法切换引擎，请等待完成后再试。"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_WorkerBusy_ActiveTasks(t *testing.T) {
	workers := map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}}
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, &fakeWorkerBusyChecker{activeTasks: true})
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "当前有即时任务正在等待或执行中，无法切换引擎，请等待完成后再试。"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_WorkerSwitch_NotBlockedByOtherWorker(t *testing.T) {
	// KEY scenario: alice is free, but bee is busy — alice's switch must succeed
	workers := map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}}
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	// beeBusy has active messages (bee is busy) — but worker switch should not care
	beeBusy := command.NewBeeBusyChecker(&fakeBeeBusyChecker{activeMessages: true}, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	// Should succeed, not be blocked
	want := `已将 Worker "alice" 的 engine 切换为 codex`
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_BusyDoesNotBlockUsage(t *testing.T) {
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(&fakeBeeBusyChecker{activeMessages: true, activeBeeExecs: true}, &fakeBeeBusyChecker{activeBeeExecs: true})
	workerBusy := command.NewWorkerBusyChecker(&fakeWorkerBusyChecker{activeExecs: true}, &fakeWorkerBusyChecker{activeTasks: true})
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "用法：\n/engine {engine} — 切换默认 engine\n/engine {engine} {workerName} — 切换指定 worker 的 engine"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
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
	if !handled {
		t.Fatal("expected handled=true")
	}
	if engineCfg.Get() != "claude" {
		t.Errorf("expected engineCfg to remain claude, got %s", engineCfg.Get())
	}
	want := "切换失败，请稍后重试"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_SwitchWorkerEngine_UpdateError(t *testing.T) {
	workers := map[string]model.Worker{"alice": {ID: "w1", Name: "alice", Engine: "claude"}}
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers, updateErr: errors.New("update error")}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	beeBusy := command.NewBeeBusyChecker(notBeeBusy, notBeeBusy)
	workerBusy := command.NewWorkerBusyChecker(notWorkerBusy, notWorkerBusy)
	h := command.NewEngineCommandHandler(repo, cfg, senders, defaultValidator, beeBusy, workerBusy, enginecfg.NewStore(""))

	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "切换失败，请稍后重试"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}
