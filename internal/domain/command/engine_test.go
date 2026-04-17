package command_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// --- fakes ---

type fakeWorkerRepo struct {
	workers map[string]model.Worker // name → worker
	updated []model.Worker
}

func (f *fakeWorkerRepo) GetByName(name string) (model.Worker, error) {
	w, ok := f.workers[name]
	if !ok {
		return model.Worker{}, fmt.Errorf("worker not found")
	}
	return w, nil
}
func (f *fakeWorkerRepo) Update(w model.Worker) (model.Worker, error) {
	f.updated = append(f.updated, w)
	return w, nil
}

type fakeSysConfig struct {
	vals map[string]string
}

func (f *fakeSysConfig) Set(_ context.Context, key, value string) error {
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

func makeReplyTo() platform.InboundMessage {
	return platform.InboundMessage{
		Platform:   "feishu",
		SessionKey: "feishu:chat1:user1",
	}
}

func makeHandler(workers map[string]model.Worker) (*command.EngineCommandHandler, *fakeSender, *fakeSysConfig) {
	sender := &fakeSender{}
	cfg := &fakeSysConfig{vals: make(map[string]string)}
	repo := &fakeWorkerRepo{workers: workers}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	h := command.NewEngineCommandHandler(repo, cfg, senders)
	return h, sender, cfg
}

// --- tests ---

func TestEngineCommand_NotACommand(t *testing.T) {
	h, sender, _ := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "hello world", makeReplyTo())
	if handled {
		t.Error("should not handle non-command")
	}
	if len(sender.sent) != 0 {
		t.Error("should not send reply for non-command")
	}
}

func TestEngineCommand_SwitchBeeEngine(t *testing.T) {
	enginecfg.Init("claude")
	h, sender, cfg := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "/engine codex", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if enginecfg.Get() != "codex" {
		t.Errorf("expected enginecfg=codex, got %s", enginecfg.Get())
	}
	if cfg.vals["default_engine"] != "codex" {
		t.Errorf("expected DB updated to codex, got %s", cfg.vals["default_engine"])
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
	h, sender, _ := makeHandler(workers)
	handled := h.HandleCommand(context.Background(), "/engine codex alice", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 || sender.sent[0] != `已将 Worker "alice" 的 engine 切换为 codex` {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}

func TestEngineCommand_InvalidEngine(t *testing.T) {
	h, sender, _ := makeHandler(nil)
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
	h, sender, _ := makeHandler(map[string]model.Worker{})
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
	h, sender, _ := makeHandler(nil)
	handled := h.HandleCommand(context.Background(), "/engine", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	want := "用法：\n/engine {engine} — 切换默认 engine\n/engine {engine} {workerName} — 切换指定 worker 的 engine"
	if len(sender.sent) != 1 || sender.sent[0] != want {
		t.Errorf("unexpected reply: %v", sender.sent)
	}
}
