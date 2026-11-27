package msgingest_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/domain/command"
	"github.com/theopenbee/openbee/internal/domain/msgingest"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

type replySender struct {
	sent chan platform.OutboundMessage
}

func (s *replySender) Send(_ context.Context, msg platform.OutboundMessage) error {
	s.sent <- msg
	return nil
}

func runUnknownCommandGateway(t *testing.T, st *mockMsgStore) (*msgingest.Gateway, *replySender) {
	t.Helper()
	sender := &replySender{sent: make(chan platform.OutboundMessage, 1)}
	chain := msgingest.ChainHandlers(noopHandler{}, command.NewUnknownCommandHandler(platform.SenderMap{"local": sender}))
	g := msgingest.New(st, 0, chain, msgingest.WithCommandHistory())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go g.Run(ctx)
	return g, sender
}

func TestGateway_UnknownCommandRepliesInsteadOfReachingBee(t *testing.T) {
	st := newMock()
	g, sender := runUnknownCommandGateway(t, st)

	g.Dispatch(platform.InboundMessage{Platform: "local", SessionKey: "local:u1", Content: "/clera"})

	select {
	case reply := <-sender.sent:
		assert.Equal(t, store.SourceTypeSystem, reply.SourceType)
		assert.Contains(t, reply.Content, "/clera")
		assert.Contains(t, reply.Content, "/clear")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for the unknown command reply")
	}
	require.Len(t, st.batches, 1)
	assert.Equal(t, store.MsgStatusCommand, st.batches[0][0].Status)
	select {
	case m := <-g.Out():
		t.Fatalf("unknown command reached the bee: %+v", m)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestGateway_PathMessageStillReachesBee(t *testing.T) {
	st := newMock()
	g, sender := runUnknownCommandGateway(t, st)

	g.Dispatch(platform.InboundMessage{Platform: "local", SessionKey: "local:u1", Content: "/Users/a/b.txt check this"})

	select {
	case m := <-g.Out():
		assert.Equal(t, "/Users/a/b.txt check this", m.Content)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for the message to reach the bee")
	}
	assert.Empty(t, sender.sent)
}
