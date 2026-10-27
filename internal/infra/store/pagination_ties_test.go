package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListFiltered_BreaksTimestampTiesNewestFirst(t *testing.T) {
	ctx := context.Background()
	pages := func(t *testing.T, list func(limit, offset int) []string) {
		t.Helper()
		assert.Equal(t, []string{"c", "b"}, list(2, 0))
		assert.Equal(t, []string{"a"}, list(2, 2))
	}

	t.Run("workers", func(t *testing.T) {
		ws := NewWorkerStore(newTestDB(t, seedWorkerSQL("a"), seedWorkerSQL("b"), seedWorkerSQL("c")))
		pages(t, func(limit, offset int) []string {
			workers, _, err := ws.ListFiltered(ctx, WorkerFilter{}, limit, offset)
			require.NoError(t, err)
			var ids []string
			for _, w := range workers {
				ids = append(ids, w.ID)
			}
			return ids
		})
	})

	t.Run("inbound messages", func(t *testing.T) {
		ms := NewMessageStore(newTestDB(t, seedMessageSQL("a", "s"), seedMessageSQL("b", "s"), seedMessageSQL("c", "s")))
		pages(t, func(limit, offset int) []string {
			msgs, _, err := ms.ListFiltered(ctx, MessageFilter{}, limit, offset)
			require.NoError(t, err)
			var ids []string
			for _, m := range msgs {
				ids = append(ids, m.ID)
			}
			return ids
		})
	})

	t.Run("outbound messages", func(t *testing.T) {
		obs := NewOutboundMessageStore(newTestDB(t))
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, obs.Create(ctx, OutboundMessage{ID: id, SessionKey: "s", Platform: "p", Status: OutboundStatusSent, SentAt: 1}))
		}
		pages(t, func(limit, offset int) []string {
			msgs, _, err := obs.ListFiltered(ctx, OutboundMessageFilter{}, limit, offset)
			require.NoError(t, err)
			var ids []string
			for _, m := range msgs {
				ids = append(ids, m.ID)
			}
			return ids
		})
	})
}
