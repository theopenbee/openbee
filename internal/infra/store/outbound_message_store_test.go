package store

import (
	"context"
	"testing"
)

func setupOutboundStore(t *testing.T) *OutboundMessageStore {
	t.Helper()
	db, err := InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewOutboundMessageStore(db)
}

func seedOutbound(t *testing.T, s *OutboundMessageStore, msgs []OutboundMessage) {
	t.Helper()
	ctx := context.Background()
	for _, m := range msgs {
		if err := s.Create(ctx, m); err != nil {
			t.Fatalf("seed outbound: %v", err)
		}
	}
}

func TestOutboundMessageStore_ListFiltered_NoFilter(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Content: "hello", Status: OutboundStatusSent, SourceType: SourceTypeBee, SentAt: 1000},
		{ID: "o2", SessionKey: "sk2", Platform: "local",  Content: "world", Status: OutboundStatusFailed, SourceType: SourceTypeWorker, SourceID: "w1", SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 2 {
		t.Errorf("total: want 2, got %d", total)
	}
	if len(msgs) != 2 {
		t.Errorf("len(msgs): want 2, got %d", len(msgs))
	}
}

func TestOutboundMessageStore_ListFiltered_BySessionKey(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 1000},
		{ID: "o2", SessionKey: "sk2", Platform: "feishu", Status: OutboundStatusSent, SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SessionKey: "sk1"}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 1 {
		t.Errorf("total: want 1, got %d", total)
	}
	if len(msgs) != 1 || msgs[0].ID != "o1" {
		t.Errorf("expected o1, got %+v", msgs)
	}
}

func TestOutboundMessageStore_ListFiltered_BySourceType(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeBee,    SentAt: 1000},
		{ID: "o2", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeWorker, SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SourceType: SourceTypeWorker}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 1 {
		t.Errorf("total: want 1, got %d", total)
	}
	if len(msgs) != 1 || msgs[0].ID != "o2" {
		t.Errorf("expected o2, got %+v", msgs)
	}
}

func TestOutboundMessageStore_ListFiltered_BySourceID(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeWorker, SourceID: "worker-A", SentAt: 1000},
		{ID: "o2", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SourceType: SourceTypeWorker, SourceID: "worker-B", SentAt: 2000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SourceID: "worker-A"}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 1 || msgs[0].ID != "o1" {
		t.Errorf("expected o1, got total=%d msgs=%+v", total, msgs)
	}
}

func TestOutboundMessageStore_ListFiltered_BySentAtRange(t *testing.T) {
	s := setupOutboundStore(t)
	seedOutbound(t, s, []OutboundMessage{
		{ID: "o1", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 1000},
		{ID: "o2", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 2000},
		{ID: "o3", SessionKey: "sk1", Platform: "feishu", Status: OutboundStatusSent, SentAt: 3000},
	})

	msgs, total, err := s.ListFiltered(context.Background(), OutboundMessageFilter{SentAtFrom: 1500, SentAtTo: 2500}, 50, 0)
	if err != nil {
		t.Fatalf("ListFiltered: %v", err)
	}
	if total != 1 || msgs[0].ID != "o2" {
		t.Errorf("expected o2, got total=%d msgs=%+v", total, msgs)
	}
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
	if err != nil {
		t.Fatalf("ListFiltered page1: %v", err)
	}
	if total != 3 {
		t.Errorf("total: want 3, got %d", total)
	}
	if len(msgs) != 2 {
		t.Errorf("page1 len: want 2, got %d", len(msgs))
	}
	// Results ordered by sent_at DESC — first item is o3
	if msgs[0].ID != "o3" {
		t.Errorf("page1[0]: want o3, got %s", msgs[0].ID)
	}

	// page 2 (limit=2, offset=2) → remaining 1
	msgs2, _, err := s.ListFiltered(context.Background(), OutboundMessageFilter{}, 2, 2)
	if err != nil {
		t.Fatalf("ListFiltered page2: %v", err)
	}
	if len(msgs2) != 1 || msgs2[0].ID != "o1" {
		t.Errorf("page2: want [o1], got %+v", msgs2)
	}
}
