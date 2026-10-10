package store_test

import (
	"context"
	"errors"
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

type preparingSender struct {
	recordingSender
	err error
}

func (s *preparingSender) PrepareOutbound(_ context.Context, msg platform.OutboundMessage) (platform.OutboundMessage, error) {
	msg.SessionKey = "local:owner"
	msg.MediaPath = "/staged/" + msg.ID
	return msg, s.err
}

func waitForOutbound(t *testing.T, outboundStore *store.OutboundMessageStore, sessionKey string) store.OutboundMessage {
	t.Helper()
	var msgs []store.OutboundMessage
	require.Eventually(t, func() bool {
		var err error
		msgs, err = outboundStore.ListBySessionKey(context.Background(), sessionKey, 0, 10)
		return err == nil && len(msgs) == 1
	}, time.Second, 10*time.Millisecond)
	return msgs[0]
}

func TestLoggingPlatformSenderAdapter_RecordsPreparedMessage(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	outboundStore := store.NewOutboundMessageStore(db)
	inner := &preparingSender{}
	adapter := store.NewLoggingPlatformSenderAdapter(inner, outboundStore, "local")

	require.NoError(t, adapter.Send(context.Background(), platform.OutboundMessage{SessionKey: "local:default", MediaPath: "/work/a.png"}))

	require.Len(t, inner.sent, 1)
	assert.Equal(t, "local:owner", inner.sent[0].SessionKey)
	record := waitForOutbound(t, outboundStore, "local:owner")
	assert.Equal(t, "/staged/"+record.ID, record.MediaPath)
	assert.Equal(t, store.OutboundStatusSent, record.Status)
}

func TestLoggingPlatformSenderAdapter_PrepareFailureSkipsSend(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	outboundStore := store.NewOutboundMessageStore(db)
	inner := &preparingSender{err: errors.New("stage failed")}
	adapter := store.NewLoggingPlatformSenderAdapter(inner, outboundStore, "local")

	err = adapter.Send(context.Background(), platform.OutboundMessage{SessionKey: "local:default", MediaPath: "/work/a.png"})

	require.ErrorContains(t, err, "stage failed")
	assert.Empty(t, inner.sent)
	record := waitForOutbound(t, outboundStore, "local:owner")
	assert.Equal(t, store.OutboundStatusFailed, record.Status)
	assert.Equal(t, "stage failed", record.Error)
}
