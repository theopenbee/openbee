package pluginchannel

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform"
)

type inboundRecorder struct {
	mu   sync.Mutex
	msgs []platform.InboundMessage
}

func (r *inboundRecorder) dispatch(m platform.InboundMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, m)
}

func (r *inboundRecorder) matching(substr string) []platform.InboundMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []platform.InboundMessage
	for _, m := range r.msgs {
		if strings.Contains(m.Content, substr) {
			out = append(out, m)
		}
	}
	return out
}

func newTestPlatform(t *testing.T, mode string) *Platform {
	t.Helper()
	p := New(Spec{
		ChannelID: "good",
		Dir:       t.TempDir(),
		Command:   fakePluginCommand(),
		Env:       fakePluginEnv(mode),
		Init:      InitializeParams{ProtocolVersion: 1, ChannelID: "good"},
	}, newTestMedia(t))
	p.sup.initialBackoff = 10 * time.Millisecond
	return p
}

func startPlatform(t *testing.T, p *Platform, rec *inboundRecorder) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = p.Receiver().Start(ctx, rec.dispatch) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-p.Done():
		case <-time.After(10 * time.Second):
			t.Error("platform did not stop")
		}
	})
	require.Eventually(t, func() bool {
		return p.sup.Call(context.Background(), "send", SendParams{SessionKey: "good:probe:x"}, nil) == nil
	}, 10*time.Second, 20*time.Millisecond)
}

func TestPlatform_SendMapsOutbound(t *testing.T) {
	rec := &inboundRecorder{}
	p := newTestPlatform(t, "ok")
	startPlatform(t, p, rec)
	assert.True(t, platform.SendsTextWithMedia(p.Sender()))

	err := p.Sender().Send(context.Background(), platform.OutboundMessage{
		SessionKey: "good:c1:u1",
		ReplyTo:    platform.InboundMessage{Platform: "good", SessionKey: "good:c1:u1", Raw: EncodeEnvelope("r1", "m1", nil)},
		Content:    "hello",
		MediaPath:  "/tmp/out/report.pdf",
	})
	require.NoError(t, err)

	var echoes []platform.InboundMessage
	require.Eventually(t, func() bool {
		echoes = rec.matching(`"session_key":"good:c1:u1"`)
		return len(echoes) > 0
	}, 5*time.Second, 10*time.Millisecond)
	require.Len(t, echoes, 1)
	var got SendParams
	require.NoError(t, json.Unmarshal([]byte(echoes[0].RawContent), &got))
	assert.Equal(t, SendParams{
		SessionKey:   "good:c1:u1",
		ChatID:       "c1",
		SessionScope: "u1",
		ReplyTo:      &ReplyTo{MessageID: "m1", Raw: "r1"},
		Content:      "hello",
		Media:        &Media{Path: "/tmp/out/report.pdf", Name: "report.pdf"},
	}, got)
}

func TestPlatform_ProactiveOmitsEmptyReplyTo(t *testing.T) {
	rec := &inboundRecorder{}
	p := newTestPlatform(t, "ok")
	startPlatform(t, p, rec)

	require.NoError(t, p.Sender().Send(context.Background(), platform.OutboundMessage{
		SessionKey: "good:c2:u2", Proactive: true, Content: "ping",
		ReplyTo: platform.InboundMessage{Platform: "good", SessionKey: "good:c2:u2"},
	}))
	var echoes []platform.InboundMessage
	require.Eventually(t, func() bool {
		echoes = rec.matching(`"session_key":"good:c2:u2"`)
		return len(echoes) == 1
	}, 5*time.Second, 10*time.Millisecond)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(echoes[0].RawContent), &got))
	assert.Equal(t, true, got["proactive"])
	assert.NotContains(t, got, "reply_to")
	assert.NotContains(t, got, "media")
}

func TestPlatform_InboundDispatch(t *testing.T) {
	rec := &inboundRecorder{}
	p := newTestPlatform(t, "emit_inbound")
	startPlatform(t, p, rec)

	var msgs []platform.InboundMessage
	require.Eventually(t, func() bool {
		msgs = rec.matching("hi")
		return len(msgs) > 0
	}, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, "good", msgs[0].Platform)
	assert.Equal(t, "good:c1:u1", msgs[0].SessionKey)
	assert.Equal(t, "good:m9", msgs[0].PlatformMessageID)
	assert.Equal(t, "hi", msgs[0].Content)
}

func TestPlatform_SendWhenDown(t *testing.T) {
	p := newTestPlatform(t, "ok")
	err := p.Sender().Send(context.Background(), platform.OutboundMessage{SessionKey: "good:c1:u1", Content: "x"})
	assert.ErrorIs(t, err, ErrNotRunning)
}

func TestPlatform_SendRejectsForeignSessionKey(t *testing.T) {
	p := newTestPlatform(t, "ok")
	err := p.Sender().Send(context.Background(), platform.OutboundMessage{SessionKey: "other:c1:u1", Content: "x"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotRunning)
}

func TestPlatform_SendRequiresContentOrMedia(t *testing.T) {
	p := newTestPlatform(t, "ok")
	err := p.Sender().Send(context.Background(), platform.OutboundMessage{SessionKey: "good:c1:u1"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotRunning)
}
