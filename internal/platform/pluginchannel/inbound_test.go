package pluginchannel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/media"
)

func newTestMedia(t *testing.T) *media.Service {
	t.Helper()
	return media.NewServiceAt(t.TempDir())
}

func TestToInbound_Basic(t *testing.T) {
	svc := newTestMedia(t)
	msg, err := ToInbound("discord", svc, InboundParams{
		ChatID: "1", SenderID: "2", MessageID: "m", Text: "hi", Raw: "r", Time: 5,
		Context: json.RawMessage(`{"guild_id":"9"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, "discord", msg.Platform)
	assert.Equal(t, "discord:1:2", msg.SessionKey)
	assert.Equal(t, "2", msg.SenderID)
	assert.Equal(t, "discord:m", msg.PlatformMessageID)
	assert.Equal(t, "hi", msg.Content)
	assert.Equal(t, "hi", msg.RawContent)
	assert.Equal(t, int64(5), msg.MessageTime)
	raw, msgID, ctx := DecodeEnvelope(msg.Raw)
	assert.Equal(t, "r", raw)
	assert.Equal(t, "m", msgID)
	assert.JSONEq(t, `{"guild_id":"9"}`, string(ctx))
}

func TestToInbound_SessionScope(t *testing.T) {
	msg, err := ToInbound("discord", newTestMedia(t), InboundParams{ChatID: "1", SenderID: "2", SessionScope: "issue-7", Text: "x"})
	require.NoError(t, err)
	assert.Equal(t, "discord:1:issue-7", msg.SessionKey)
}

func TestToInbound_NoMessageIDOrTime(t *testing.T) {
	msg, err := ToInbound("discord", newTestMedia(t), InboundParams{ChatID: "1", SenderID: "2", Text: "x"})
	require.NoError(t, err)
	assert.Empty(t, msg.PlatformMessageID)
	assert.Zero(t, msg.MessageTime)
}

func TestToInbound_Validation(t *testing.T) {
	cases := map[string]InboundParams{
		"empty chat":        {SenderID: "2", Text: "x"},
		"chat with colon":   {ChatID: "a:b", SenderID: "2", Text: "x"},
		"empty sender":      {ChatID: "1", Text: "x"},
		"no text or attach": {ChatID: "1", SenderID: "2"},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ToInbound("discord", newTestMedia(t), p)
			assert.Error(t, err)
		})
	}
}

func TestToInbound_Attachments(t *testing.T) {
	svc := newTestMedia(t)
	path := filepath.Join(svc.InboundDir(), "a.png")
	require.NoError(t, os.WriteFile(path, []byte("png"), 0o644))

	msg, err := ToInbound("discord", svc, InboundParams{
		ChatID: "1", SenderID: "2", Text: "hi",
		Attachments: []Attachment{{Type: "image", Path: path, Name: "a.png"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "hi\n"+svc.BuildPlaceholder("image", path, "a.png"), msg.Content)
	assert.Equal(t, "hi", msg.RawContent)
}

func TestToInbound_AttachmentOnly(t *testing.T) {
	svc := newTestMedia(t)
	p1 := filepath.Join(svc.InboundDir(), "a.ogg")
	p2 := filepath.Join(svc.InboundDir(), "b.mp4")
	msg, err := ToInbound("discord", svc, InboundParams{
		ChatID: "1", SenderID: "2",
		Attachments: []Attachment{{Type: "audio", Path: p1}, {Type: "video", Path: p2, Name: "b.mp4"}},
	})
	require.NoError(t, err)
	assert.Equal(t, svc.BuildPlaceholder("audio", p1, "")+"\n"+svc.BuildPlaceholder("video", p2, "b.mp4"), msg.Content)
}

func TestToInbound_AttachmentOutsideMediaDir(t *testing.T) {
	svc := newTestMedia(t)
	msg, err := ToInbound("discord", svc, InboundParams{
		ChatID: "1", SenderID: "2", Text: "look",
		Attachments: []Attachment{
			{Type: "image", Path: "/etc/passwd", Name: "passwd"},
			{Type: "spreadsheet", Path: filepath.Join(svc.InboundDir(), "..", "escape.csv"), Name: "escape.csv"},
			{Type: "image", Path: "relative.png", Name: "relative.png"},
		},
	})
	require.NoError(t, err)
	assert.NotContains(t, msg.Content, "/etc/passwd")
	assert.NotContains(t, msg.Content, "escape.csv\" path")
	assert.Contains(t, msg.Content, svc.BuildPlaceholder("image", "", "passwd"))
	assert.Contains(t, msg.Content, svc.BuildPlaceholder("document", "", "escape.csv"))
	assert.Contains(t, msg.Content, svc.BuildPlaceholder("image", "", "relative.png"))
	assert.Equal(t, 3, strings.Count(msg.Content, "<media:"))
}
