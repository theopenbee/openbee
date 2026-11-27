package command_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/platform"
)

func makeUnknownHandler() (*command.UnknownCommandHandler, *fakeSender) {
	sender := &fakeSender{}
	return command.NewUnknownCommandHandler(platform.SenderMap{"beta": sender}), sender
}

func TestUnknownCommand_IsCommand(t *testing.T) {
	h, _ := makeUnknownHandler()
	cases := map[string]bool{
		"/clera":                      true,
		"/help":                       true,
		"/help me with this":          true,
		"/Clear":                      true,
		"/start":                      true,
		"/foo\nbar":                   true,
		"/my_cmd-2":                   true,
		"/clear":                      false,
		"/clear worker":               false,
		"/stop\nnow":                  false,
		"/Users/a/b.txt check":        false,
		"/tmp/x.log":                  false,
		"/a.b":                        false,
		"/":                           false,
		"/ hello":                     false,
		"/123":                        false,
		"/清除":                         false,
		"hello /clera":                false,
		" /clera":                     false,
		"":                            false,
		"/" + strings.Repeat("a", 33): false,
	}
	for input, want := range cases {
		assert.Equal(t, want, h.IsCommand(input), input)
	}
}

func TestUnknownCommand_RepliesWithAvailableCommands(t *testing.T) {
	h, sender := makeUnknownHandler()
	handled := h.HandleCommand(context.Background(), "/help me", makeReplyTo())
	require.True(t, handled)
	m := i18n.M.Runtime.UnknownCommand
	want := fmt.Sprintf(m.Unknown, "/help") + "\n" + fmt.Sprintf(m.Available, "/clear /stop /status /list /cancel /engine")
	assert.Equal(t, []string{want}, sender.sent)
}

func TestUnknownCommand_SuggestsClosestCommand(t *testing.T) {
	m := i18n.M.Runtime.UnknownCommand
	cases := map[string]string{
		"/clera":  "/clear",
		"/Clear":  "/clear",
		"/STATUS": "/status",
		"/sta":    "/status",
		"/ls":     "/list",
		"/cancle": "/cancel",
		"/engin":  "/engine",
		"/stpo":   "/stop",
	}
	for input, suggestion := range cases {
		h, sender := makeUnknownHandler()
		require.True(t, h.HandleCommand(context.Background(), input, makeReplyTo()), input)
		require.Len(t, sender.sent, 1, input)
		first, _, _ := strings.Cut(sender.sent[0], "\n")
		assert.Equal(t, fmt.Sprintf(m.Unknown, input)+fmt.Sprintf(m.Suggestion, suggestion), first, input)
	}
}

func TestUnknownCommand_NoSuggestionWhenNothingIsClose(t *testing.T) {
	m := i18n.M.Runtime.UnknownCommand
	for _, input := range []string{"/help", "/start", "/xyz", "/a"} {
		h, sender := makeUnknownHandler()
		require.True(t, h.HandleCommand(context.Background(), input, makeReplyTo()), input)
		require.Len(t, sender.sent, 1, input)
		first, _, _ := strings.Cut(sender.sent[0], "\n")
		assert.Equal(t, fmt.Sprintf(m.Unknown, input), first, input)
	}
}

func TestUnknownCommand_LeavesKnownCommandsAndChat(t *testing.T) {
	h, sender := makeUnknownHandler()
	for _, input := range []string{"/clear", "/stop now", "/status", "/list x", "/cancel", "/engine", "hello", "/tmp/x"} {
		assert.False(t, h.HandleCommand(context.Background(), input, makeReplyTo()), input)
	}
	assert.Empty(t, sender.sent)
}

func TestKnownCommands_AcceptAnyWhitespaceAfterName(t *testing.T) {
	h, _ := makeListHandler(nil, nil)
	assert.True(t, h.IsCommand("/list\tfoo"))
	assert.True(t, h.IsCommand("/list\nfoo"))
	assert.False(t, h.IsCommand("/listfoo"))
}
