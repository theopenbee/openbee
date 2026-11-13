package rpc_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/rpc"
)

type buttonMockSender struct {
	*mockSender
	combined bool
}

func (s *buttonMockSender) SendsButtons() bool       { return true }
func (s *buttonMockSender) SendsTextWithMedia() bool { return s.combined }

func sendOptions(t *testing.T, sender platform.PlatformSenderAdapter, workerName string, args map[string]any) error {
	t.Helper()
	s, db := setupServerWithSender(t, "telegram", sender)
	ctx := context.Background()
	_, err := store.NewMessageStore(db).Create(ctx, "msg-options", "telegram:chat1:userA", "telegram", "hi", `{}`, "", 0)
	require.NoError(t, err)
	if workerName != "" {
		w := mustCreateWorker(t, s, map[string]any{"name": workerName})
		ctx = context.WithValue(ctx, rpc.CtxWorkerIDKey, w.ID)
	}
	args["message_id"] = "msg-options"
	_, err = s.CallTool(ctx, "send_message", mustMarshal(t, args))
	return err
}

func TestSendMessageOptions_WorkerButtonsRouteBackToWorker(t *testing.T) {
	spy := &buttonMockSender{mockSender: &mockSender{}}
	require.NoError(t, sendOptions(t, spy, "MaoMao", map[string]any{
		"content": "Merge now?",
		"options": []string{"Merge", " Wait "},
	}))

	require.Len(t, spy.sent, 1)
	assert.Equal(t, "MaoMao\nMerge now?", spy.sent[0].Content)
	assert.Equal(t, []platform.Button{
		{Label: "Merge", Text: "@MaoMao Merge"},
		{Label: "Wait", Text: "@MaoMao Wait"},
	}, spy.sent[0].Buttons)
}

func TestSendMessageOptions_WorkerNameWithSpaceSendsPlainOption(t *testing.T) {
	spy := &buttonMockSender{mockSender: &mockSender{}}
	require.NoError(t, sendOptions(t, spy, "Mao Mao", map[string]any{
		"content": "Merge now?",
		"options": []string{"Merge"},
	}))

	require.Len(t, spy.sent, 1)
	assert.Equal(t, []platform.Button{{Label: "Merge", Text: "Merge"}}, spy.sent[0].Buttons)
}

func TestSendMessageOptions_BeeButtonsSendPlainOption(t *testing.T) {
	spy := &buttonMockSender{mockSender: &mockSender{}}
	require.NoError(t, sendOptions(t, spy, "", map[string]any{
		"content": "Which worker?",
		"options": []string{"Alice", "Bob"},
	}))

	require.Len(t, spy.sent, 1)
	assert.Equal(t, "Which worker?", spy.sent[0].Content)
	assert.Equal(t, []platform.Button{{Label: "Alice", Text: "Alice"}, {Label: "Bob", Text: "Bob"}}, spy.sent[0].Buttons)
}

func TestSendMessageOptions_ListedWithoutButtonSupport(t *testing.T) {
	spy := &mockSender{}
	require.NoError(t, sendOptions(t, spy, "", map[string]any{
		"content": "Which worker?",
		"options": []string{"Alice", "Bob"},
	}))

	require.Len(t, spy.sent, 1)
	assert.Equal(t, "Which worker?\n\n1. Alice\n2. Bob", spy.sent[0].Content)
	assert.Empty(t, spy.sent[0].Buttons)
}

func TestSendMessageOptions_ButtonsStayOnTheText(t *testing.T) {
	cases := []struct {
		name     string
		combined bool
		wantSent int
	}{
		{name: "Combined", combined: true, wantSent: 1},
		{name: "Separate", combined: false, wantSent: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := &buttonMockSender{mockSender: &mockSender{}, combined: tc.combined}
			require.NoError(t, sendOptions(t, spy, "", map[string]any{
				"content":    "Use this file?",
				"media_path": "/tmp/report.pdf",
				"options":    []string{"Yes"},
			}))

			require.Len(t, spy.sent, tc.wantSent)
			assert.Equal(t, "Use this file?", spy.sent[0].Content)
			assert.Equal(t, []platform.Button{{Label: "Yes", Text: "Yes"}}, spy.sent[0].Buttons)
			if tc.wantSent == 2 {
				assert.Equal(t, "/tmp/report.pdf", spy.sent[1].MediaPath)
				assert.Empty(t, spy.sent[1].Buttons)
			}
		})
	}
}

func TestSendMessageOptions_Validation(t *testing.T) {
	many := make([]string, 11)
	for i := range many {
		many[i] = "x"
	}
	cases := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{name: "NeedsContent", args: map[string]any{"media_path": "/tmp/a.png", "options": []string{"Yes"}}, wantErr: "options need content"},
		{name: "TooMany", args: map[string]any{"content": "?", "options": many}, wantErr: "at most 10 options"},
		{name: "Empty", args: map[string]any{"content": "?", "options": []string{"Yes", "  "}}, wantErr: "must not be empty"},
		{name: "Multiline", args: map[string]any{"content": "?", "options": []string{"a\nb"}}, wantErr: "single line"},
		{name: "TooLong", args: map[string]any{"content": "?", "options": []string{strings.Repeat("长", 101)}}, wantErr: "at most 100 characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := &buttonMockSender{mockSender: &mockSender{}}
			err := sendOptions(t, spy, "", tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, spy.sent)
		})
	}
}
