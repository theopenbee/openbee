package command_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		assert.Equal(t, want, h.IsCommand(input))
	}
}

func TestListCommand_UsageOnExtraArgs(t *testing.T) {
	h, sender := makeListHandler(nil, nil)
	handled := h.HandleCommand(context.Background(), "/list a b", makeReplyTo())
	require.True(t, handled, "expected handled=true")
	assert.Equal(t, []string{i18n.M.Runtime.ListCommand.Usage}, sender.sent)
}

func TestListCommand_EmptyDirectory(t *testing.T) {
	h, sender := makeListHandler(nil, nil)
	handled := h.HandleCommand(context.Background(), "/list", makeReplyTo())
	require.True(t, handled, "expected handled=true")
	require.Len(t, sender.sent, 1)
	out := sender.sent[0]
	m := i18n.M.Runtime.ListCommand
	for _, want := range []string{fmt.Sprintf(m.HeaderAll, 0), m.EmptyAll} {
		assert.Contains(t, out, want)
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

	assert.Contains(t, out, fmt.Sprintf(m.HeaderAll, 3))
	// expected sort: 小乔 < 张三 < 李四 (by Go's default string < on UTF-8 bytes)
	idxXiao := strings.Index(out, "小乔")
	idxZhang := strings.Index(out, "张三")
	idxLi := strings.Index(out, "李四")
	require.GreaterOrEqual(t, idxXiao, 0)
	require.GreaterOrEqual(t, idxZhang, 0)
	require.GreaterOrEqual(t, idxLi, 0)
	assert.Less(t, idxXiao, idxZhang)
	assert.Less(t, idxZhang, idxLi)
	for _, want := range []string{
		fmt.Sprintf(m.Line, "小乔", m.StatusIdle, "负责 openbee 开发"),
		fmt.Sprintf(m.Line, "张三", m.StatusWorking, "前端开发"),
		fmt.Sprintf(m.Line, "李四", m.StatusError, "后端开发"),
	} {
		assert.Contains(t, out, want)
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
	assert.Contains(t, out, wantHeader)
	assert.NotContains(t, out, "alice", "alice should be filtered out")
	assert.Contains(t, out, "bob")
	assert.Contains(t, out, "carol")
}

func TestListCommand_KeywordCaseInsensitive(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "alice", Description: "openbee maintainer", Status: model.WorkerStatusIdle},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list OPENBEE", makeReplyTo())
	out := sender.sent[0]
	assert.Contains(t, out, "alice")
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
		assert.Contains(t, out, want)
	}
}

func TestListCommand_LookupError(t *testing.T) {
	h, sender := makeListHandler(nil, errors.New("boom"))
	handled := h.HandleCommand(context.Background(), "/list", makeReplyTo())
	require.True(t, handled, "expected handled=true")
	require.Len(t, sender.sent, 1)
	assert.Equal(t, i18n.M.Runtime.ListCommand.LookupFailed, sender.sent[0])
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
		assert.Contains(t, out, want)
	}
}

func TestListCommand_UnknownStatusFallsBack(t *testing.T) {
	workers := []model.Worker{
		{ID: "w1", Name: "a", Description: "x", Status: model.WorkerStatus("paused")},
	}
	h, sender := makeListHandler(workers, nil)
	h.HandleCommand(context.Background(), "/list", makeReplyTo())
	out := sender.sent[0]
	assert.Contains(t, out, "paused")
}
