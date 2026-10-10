package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

type recordingSender struct {
	sent []platform.OutboundMessage
}

func (s *recordingSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	s.sent = append(s.sent, msg)
	return nil
}

func TestLoggingPlatformSenderAdapter_SharesIDWithInnerSender(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	outboundStore := store.NewOutboundMessageStore(db)
	inner := &recordingSender{}
	adapter := store.NewLoggingPlatformSenderAdapter(inner, outboundStore, "local")

	require.NoError(t, adapter.Send(context.Background(), platform.OutboundMessage{SessionKey: "local:s1", Content: "hi"}))

	require.Len(t, inner.sent, 1)
	id := inner.sent[0].ID
	require.NotEmpty(t, id)
	require.Eventually(t, func() bool {
		msgs, err := outboundStore.ListBySessionKey(context.Background(), "local:s1", 0, 10)
		return err == nil && len(msgs) == 1
	}, time.Second, 10*time.Millisecond)
	msgs, err := outboundStore.ListBySessionKey(context.Background(), "local:s1", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, id, msgs[0].ID)
}
