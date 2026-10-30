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

type combinedRecordingSender struct {
	recordingSender
}

func (s *combinedRecordingSender) SendsTextWithMedia() bool { return true }

func TestLoggingPlatformSenderAdapter_SendsTextWithMedia(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	outboundStore := store.NewOutboundMessageStore(db)

	combined := store.NewLoggingPlatformSenderAdapter(&combinedRecordingSender{}, outboundStore, "alpha")
	plain := store.NewLoggingPlatformSenderAdapter(&recordingSender{}, outboundStore, "beta")

	assert.True(t, platform.SendsTextWithMedia(combined))
	assert.False(t, platform.SendsTextWithMedia(plain))
}

func TestLoggingPlatformSenderAdapter_RecordsKindAndPayload(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	outboundStore := store.NewOutboundMessageStore(db)
	adapter := store.NewLoggingPlatformSenderAdapter(&recordingSender{}, outboundStore, "local")

	require.NoError(t, adapter.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "local:s1", Content: "handoff", Kind: store.OutboundKindHandoff, Payload: `{"task_id":"t1"}`,
	}))
	require.NoError(t, adapter.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "local:s2", Content: "plain",
	}))

	require.Eventually(t, func() bool {
		a, errA := outboundStore.ListBySessionKey(context.Background(), "local:s1", 0, 10)
		b, errB := outboundStore.ListBySessionKey(context.Background(), "local:s2", 0, 10)
		return errA == nil && errB == nil && len(a) == 1 && len(b) == 1
	}, time.Second, 10*time.Millisecond)
	handoff, err := outboundStore.ListBySessionKey(context.Background(), "local:s1", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, store.OutboundKindHandoff, handoff[0].Kind)
	assert.Equal(t, `{"task_id":"t1"}`, handoff[0].Payload)
	plain, err := outboundStore.ListBySessionKey(context.Background(), "local:s2", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, store.OutboundKindMessage, plain[0].Kind)
	assert.Empty(t, plain[0].Payload)
}

func TestLoggingPlatformSenderAdapter_KeepsSendOrder(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	outboundStore := store.NewOutboundMessageStore(db)
	inner := &recordingSender{}
	adapter := store.NewLoggingPlatformSenderAdapter(inner, outboundStore, "local")

	for _, msg := range []platform.OutboundMessage{
		{SessionKey: "local:s1", Content: "text first"},
		{SessionKey: "local:s1", MediaPath: "/tmp/chart.png"},
		{SessionKey: "local:s1", Content: "text last"},
	} {
		require.NoError(t, adapter.Send(context.Background(), msg))
	}

	require.Len(t, inner.sent, 3)
	assert.Less(t, inner.sent[0].SentAt, inner.sent[1].SentAt)
	assert.Less(t, inner.sent[1].SentAt, inner.sent[2].SentAt)
	require.Eventually(t, func() bool {
		msgs, err := outboundStore.ListBySessionKey(context.Background(), "local:s1", 0, 10)
		return err == nil && len(msgs) == 3
	}, time.Second, 10*time.Millisecond)
	msgs, err := outboundStore.ListBySessionKey(context.Background(), "local:s1", 0, 10)
	require.NoError(t, err)
	for i, m := range msgs {
		assert.Equal(t, inner.sent[i].ID, m.ID)
		assert.Equal(t, inner.sent[i].SentAt, m.SentAt)
	}
}

type failingSender struct{}

func (failingSender) Send(context.Context, platform.OutboundMessage) error {
	return errors.New("stage media: no such file")
}

func TestOutboundListBySessionKey_SkipsFailedSends(t *testing.T) {
	db, err := store.InitDB(t.TempDir() + "/test.db")
	require.NoError(t, err)
	defer db.Close()
	outboundStore := store.NewOutboundMessageStore(db)
	ok := store.NewLoggingPlatformSenderAdapter(&recordingSender{}, outboundStore, "local")
	bad := store.NewLoggingPlatformSenderAdapter(failingSender{}, outboundStore, "local")

	require.NoError(t, ok.Send(context.Background(), platform.OutboundMessage{SessionKey: "local:s1", Content: "hi"}))
	require.Error(t, bad.Send(context.Background(), platform.OutboundMessage{SessionKey: "local:s1", MediaPath: "/missing.png"}))

	require.Eventually(t, func() bool {
		_, total, err := outboundStore.ListFiltered(context.Background(), store.OutboundMessageFilter{SessionKey: "local:s1"}, 10, 0)
		return err == nil && total == 2
	}, time.Second, 10*time.Millisecond)
	msgs, err := outboundStore.ListBySessionKey(context.Background(), "local:s1", 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "hi", msgs[0].Content)
}
