package ctlcmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/utils"
)

func runMessageSend(t *testing.T, args ...string) (string, map[string]any, error) {
	t.Helper()
	var gotTool string
	var gotArgs map[string]any
	run := func(toolName string, a any) error {
		gotTool = toolName
		gotArgs = a.(map[string]any)
		return nil
	}
	cmd := newMessageSendCommand(run, "send")
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	return gotTool, gotArgs, err
}

func TestMessageSend_MessageID(t *testing.T) {
	tool, args, err := runMessageSend(t, "--message-id", "msg-1", "--media-path", "/tmp/a.png")
	require.NoError(t, err)
	assert.Equal(t, utils.SendMessage, tool)
	assert.Equal(t, map[string]any{"message_id": "msg-1", "media_path": "/tmp/a.png"}, args)
}

func TestMessageSend_SessionKey(t *testing.T) {
	tool, args, err := runMessageSend(t, "--session-key", "feishu:oc_1:ou_1", "--media-path", "/tmp/a.png")
	require.NoError(t, err)
	assert.Equal(t, utils.SendMessage, tool)
	assert.Equal(t, map[string]any{"session_key": "feishu:oc_1:ou_1", "media_path": "/tmp/a.png"}, args)
}

func TestMessageSend_RequiresTarget(t *testing.T) {
	_, _, err := runMessageSend(t, "--media-path", "/tmp/a.png")
	require.Error(t, err)
}

func TestMessageSend_RejectsBothTargets(t *testing.T) {
	_, _, err := runMessageSend(t, "--message-id", "msg-1", "--session-key", "feishu:oc_1:ou_1", "--media-path", "/tmp/a.png")
	require.Error(t, err)
}
