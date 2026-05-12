package core_test

import (
	"context"
	"errors"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
	core "github.com/theopenbee/openbee/internal/ai/core"
)

type fakeInvoker struct {
	proc ai.Process
	ch   <-chan ai.Output
	err  error
}

func (f *fakeInvoker) Run(ctx context.Context, workDir, prompt string,
	opts ai.RunOptions, logPath string) (ai.Process, <-chan ai.Output, error) {
	return f.proc, f.ch, f.err
}

type fakeCollector struct {
	usages []ai.TokenUsage
	err    error
}

func (f *fakeCollector) Collect(ctx context.Context, sessionID string) ([]ai.TokenUsage, error) {
	return f.usages, f.err
}

func TestBaseAdapter_RunBindsExtract(t *testing.T) {
	ch := make(chan ai.Output)
	close(ch)
	var capturedLogPath string
	b := &core.BaseAdapter{
		Invoker:   &fakeInvoker{ch: ch},
		Collector: &fakeCollector{},
		Extract:   func(logPath string) string { capturedLogPath = logPath; return "x" },
	}
	res, err := b.Run(context.Background(), "/wd", "p", ai.RunOptions{}, "/the/log")
	if err != nil {
		t.Fatal(err)
	}
	if r := res.ExtractResult(); r != "x" {
		t.Errorf("got %q", r)
	}
	if capturedLogPath != "/the/log" {
		t.Errorf("logPath not bound; got %q", capturedLogPath)
	}
}

func TestBaseAdapter_RunPropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	b := &core.BaseAdapter{
		Invoker:   &fakeInvoker{err: wantErr},
		Collector: &fakeCollector{},
		Extract:   func(string) string { return "" },
	}
	_, err := b.Run(context.Background(), "/wd", "", ai.RunOptions{}, "/log")
	if !errors.Is(err, wantErr) {
		t.Errorf("want wantErr, got %v", err)
	}
}

func TestBaseAdapter_PrepareIsNoop(t *testing.T) {
	b := &core.BaseAdapter{}
	if err := b.Prepare("/wd", ai.PrepareOptions{}); err != nil {
		t.Error(err)
	}
}

func TestBaseAdapter_CollectDelegates(t *testing.T) {
	want := []ai.TokenUsage{{Model: "m", InputTokens: 7}}
	b := &core.BaseAdapter{
		Invoker:   &fakeInvoker{},
		Collector: &fakeCollector{usages: want},
		Extract:   func(string) string { return "" },
	}
	got, err := b.CollectTokenUsage(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Model != "m" || got[0].InputTokens != 7 {
		t.Errorf("delegation broken; got %+v", got)
	}
}
