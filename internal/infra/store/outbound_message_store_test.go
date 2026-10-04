package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOutboundStore(t *testing.T) *OutboundMessageStore {
	t.Helper()
	return NewOutboundMessageStore(newTestDB(t))
}

func seedOutbound(t *testing.T, s *OutboundMessageStore, msgs []OutboundMessage) {
	t.Helper()
	ctx := context.Background()
	for _, m := range msgs {
		require.NoError(t, s.Create(ctx, m), "seed outbound")
	}
}

func TestOutboundMessageStore_ListFiltered_NoFilter(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Content: "hello", Status: OutboundStatusSent, SourceType: SourceTypeBee, SentAt: 1000},
		{ID: "o2", SessionKey: "sk2", Platform: "local", Content: "world", Status: OutboundStatusFailed, SourceType: SourceTypeWorker, SourceID: "w1", SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Len(t, msgs, 2)
}

func TestOutboundMessageStore_ListFiltered_BySessionKey(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 1000},
		{ID: "o2", SessionKey: "sk2", Platform: "feishu", Status: OutboundStatusSent, SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SessionKey: "sk1"}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, msgs, 1)
	assert.Equal(t, "o1", msgs[0].ID)
}

func TestOutboundMessageStore_ListFiltered_BySourceType(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeBee, SentAt: 1000},
		{ID: "o2", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeWorker, SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SourceType: SourceTypeWorker}, 50, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, msgs, 1)
	assert.Equal(t, "o2", msgs[0].ID)
}

func TestOutboundMessageStore_ListFiltered_BySourceID(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeWorker, SourceID: "worker-A", SentAt: 1000},
		{ID: "o2", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeWorker, SourceID: "worker-B", SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SourceID: "worker-A"}, 50, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	assert.Equal(t, "o1", msgs[0].ID)
}

func TestOutboundMessageStore_ListFiltered_BySentAtRange(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 1000},
		{ID: "o2", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 2000},
		{ID: "o3", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 3000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SentAtFrom: 1500, SentAtTo: 2500}, 50, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	assert.Equal(t, "o2", msgs[0].ID)
}

func TestOutboundMessageStore_ListFiltered_Pagination(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 1000},
		{ID: "o2", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 2000},
		{ID: "o3", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 3000},
	})

	// page 1 (limit=2, offset=0) → most recent 2
	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{}, 2, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, msgs, 2)
	// Results ordered by sent_at DESC — first item is o3
	assert.Equal(t, "o3", msgs[0].ID)

	// page 2 (limit=2, offset=2) → remaining 1
	msgs2, _, err := s.ListFiltered(context.Background(), OutboundMessageFilter{}, 2, 2)
	require.NoError(t, err)
	require.Len(t, msgs2, 1)
	assert.Equal(t, "o1", msgs2[0].ID)
}
