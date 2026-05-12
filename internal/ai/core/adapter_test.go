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
	b := &core.Composite{
		Invoker:   &fakeInvoker{ch: ch},
		Collector: &fakeCollector{},
		Extractor: &fakeExtractor{captured: &capturedLogPath, result: "x"},
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
	b := &core.Composite{
		Invoker:   &fakeInvoker{err: wantErr},
		Collector: &fakeCollector{},
		Extractor: &fakeExtractor{},
	}
	_, err := b.Run(context.Background(), "/wd", "", ai.RunOptions{}, "/log")
	if !errors.Is(err, wantErr) {
		t.Errorf("want wantErr, got %v", err)
	}
}

func TestBaseAdapter_CollectDelegates(t *testing.T) {
	want := []ai.TokenUsage{{Model: "m", InputTokens: 7}}
	b := &core.Composite{
		Invoker:   &fakeInvoker{},
		Collector: &fakeCollector{usages: want},
		Extractor: &fakeExtractor{},
	}
	got, err := b.CollectTokenUsage(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Model != "m" || got[0].InputTokens != 7 {
		t.Errorf("delegation broken; got %+v", got)
	}
}

type fakeExtractor struct {
	captured *string
	result   string
}

func (f *fakeExtractor) Extract(logPath string) string {
	if f.captured != nil {
		*f.captured = logPath
	}
	return f.result
}

