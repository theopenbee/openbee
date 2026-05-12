package command_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

type fakeWorkerLister struct {
	workers []model.Worker
	err     error
}

func (f *fakeWorkerLister) List() ([]model.Worker, error) {
	return f.workers, f.err
}

func makeListHandler(workers []model.Worker, err error) (*command.ListCommandHandler, *fakeSender) {
	sender := &fakeSender{}
	senders := map[string]platform.PlatformSenderAdapter{"feishu": sender}
	lister := &fakeWorkerLister{workers: workers, err: err}
	return command.NewListCommandHandler(lister, senders), sender
}

func TestListCommand_IsCommand(t *testing.T) {
	h, _ := makeListHandler(nil, nil)
	cases := map[string]bool{
		"/list":         true,
		"/list keyword": true,
		"/listfoo":      false,
		"hello":         false,
		"":              false,
	}
	for input, want := range cases {
		if got := h.IsCommand(input); got != want {
			t.Errorf("IsCommand(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestListCommand_UsageOnExtraArgs(t *testing.T) {
	h, sender := makeListHandler(nil, nil)
	handled := h.HandleCommand(context.Background(), "/list a b", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 || sender.sent[0] != i18n.M.Runtime.ListCommand.Usage {
		t.Errorf("expected usage reply, got %v", sender.sent)
	}
}

func TestListCommand_EmptyDirectory(t *testing.T) {
	h, sender := makeListHandler(nil, nil)
	handled := h.HandleCommand(context.Background(), "/list", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	out := sender.sent[0]
	m := i18n.M.Runtime.ListCommand
	for _, want := range []string{fmt.Sprintf(m.HeaderAll, 0), m.EmptyAll} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestListCommand_AllWorkersSortedByName(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "张三", Description: "前端开发", Status: model.WorkerStatusWorking},
		{ID: "w2", Name: "李四", Description: "后端开发", Status: model.WorkerStatusError},
		{ID: "w3", Name: "小乔", Description: "负责 openbee 开发", Status: model.WorkerStatusIdle},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list", makeReplyTo())
	out := sender.sent[0]
	m := i18n.M.Runtime.ListCommand

	if !strings.Contains(out, fmt.Sprintf(m.HeaderAll, 3)) {
		t.Errorf("missing header\n%s", out)
	}
	// expected sort: 小乔 < 张三 < 李四 (by Go's default string < on UTF-8 bytes)
	idxXiao := strings.Index(out, "小乔")
	idxZhang := strings.Index(out, "张三")
	idxLi := strings.Index(out, "李四")
	if idxXiao < 0 || idxZhang < 0 || idxLi < 0 {
		t.Fatalf("missing one of the worker names:\n%s", out)
	}
	if !(idxXiao < idxZhang && idxZhang < idxLi) {
		t.Errorf("workers not in expected sort order; got positions xiao=%d zhang=%d li=%d\n%s",
			idxXiao, idxZhang, idxLi, out)
	}
	for _, want := range []string{
		fmt.Sprintf(m.Line, "小乔", m.StatusIdle, "负责 openbee 开发"),
		fmt.Sprintf(m.Line, "张三", m.StatusWorking, "前端开发"),
		fmt.Sprintf(m.Line, "李四", m.StatusError, "后端开发"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing line %q\n%s", want, out)
		}
	}
}

func TestListCommand_KeywordSubstringMatch(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "alice", Description: "frontend dev", Status: model.WorkerStatusIdle},
		{ID: "w2", Name: "bob", Description: "openbee backend", Status: model.WorkerStatusIdle},
		{ID: "w3", Name: "carol", Description: "QA on openbee", Status: model.WorkerStatusIdle},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list openbee", makeReplyTo())
	out := sender.sent[0]

	wantHeader := fmt.Sprintf(i18n.M.Runtime.ListCommand.HeaderSearch, "openbee", 2)
	if !strings.Contains(out, wantHeader) {
		t.Errorf("missing search header %q\n%s", wantHeader, out)
	}
	if strings.Contains(out, "alice") {
		t.Errorf("alice should be filtered out\n%s", out)
	}
	if !strings.Contains(out, "bob") || !strings.Contains(out, "carol") {
		t.Errorf("expected bob and carol\n%s", out)
	}
}

func TestListCommand_KeywordCaseInsensitive(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "alice", Description: "openbee maintainer", Status: model.WorkerStatusIdle},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list OPENBEE", makeReplyTo())
	out := sender.sent[0]
	if !strings.Contains(out, "alice") {
		t.Errorf("expected case-insensitive match for OPENBEE\n%s", out)
	}
}

func TestListCommand_KeywordNoMatch(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "alice", Description: "frontend", Status: model.WorkerStatusIdle},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list zzznope", makeReplyTo())
	out := sender.sent[0]
	m := i18n.M.Runtime.ListCommand
	for _, want := range []string{fmt.Sprintf(m.HeaderSearch, "zzznope", 0), m.EmptySearch} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n%s", want, out)
		}
	}
}

func TestListCommand_LookupError(t *testing.T) {
	h, sender := makeListHandler(nil, errors.New("boom"))
	handled := h.HandleCommand(context.Background(), "/list", makeReplyTo())
	if !handled {
		t.Fatal("expected handled=true")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(sender.sent))
	}
	if sender.sent[0] != i18n.M.Runtime.ListCommand.LookupFailed {
		t.Errorf("expected lookup_failed reply, got %q", sender.sent[0])
	}
}

func TestListCommand_StatusLabels(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "a", Description: "x", Status: model.WorkerStatusIdle},
		{ID: "w2", Name: "b", Description: "x", Status: model.WorkerStatusWorking},
		{ID: "w3", Name: "c", Description: "x", Status: model.WorkerStatusError},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list", makeReplyTo())
	out := sender.sent[0]
	m := i18n.M.Runtime.ListCommand
	for _, want := range []string{m.StatusIdle, m.StatusWorking, m.StatusError} {
		if !strings.Contains(out, want) {
			t.Errorf("missing status label %q\n%s", want, out)
		}
	}
}

func TestListCommand_UnknownStatusFallsBack(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "a", Description: "x", Status: model.WorkerStatus("paused")},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list", makeReplyTo())
	out := sender.sent[0]
	if !strings.Contains(out, "paused") {
		t.Errorf("expected unknown status to fall through verbatim\n%s", out)
	}
}
