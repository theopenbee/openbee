package local_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func setupSenderDB(t *testing.T) (*store.LocalReplyStore, *sql.DB) {
	t.Helper()
	db, err := store.InitDB(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store.NewLocalReplyStore(db), db
}

func TestLocalSender_Send_WritesReplyAndBroadcasts(t *testing.T) {
	replyStore, _ := setupSenderDB(t)
	hub := local.NewSSEHub()
	sender := local.NewLocalSender(replyStore, hub)

	ch, unsub := hub.Subscribe("local:sess-1")
	defer unsub()

	msg := platform.OutboundMessage{
		ReplyTo: platform.InboundMessage{SessionKey: "local:sess-1"},
		Content: "Reply content",
	}
	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Verify DB write
	replies, err := replyStore.ListBySession(context.Background(), "local:sess-1")
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(replies) != 1 || replies[0].Content != "Reply content" {
		t.Errorf("unexpected replies: %+v", replies)
	}

	// Verify SSE broadcast
	select {
	case data := <-ch:
		var payload map[string]any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			t.Fatalf("broadcast data is not valid JSON: %v", err)
		}
		if payload["content"] != "Reply content" {
			t.Errorf("broadcast content mismatch: %v", payload)
		}
	default:
		t.Fatal("expected SSE broadcast but channel was empty")
	}
}

func TestLocalSender_Send_UsesReplyToSessionKey(t *testing.T) {
	replyStore, _ := setupSenderDB(t)
	hub := local.NewSSEHub()
	sender := local.NewLocalSender(replyStore, hub)

	// OutboundMessage.SessionKey is empty — only ReplyTo.SessionKey should be used
	msg := platform.OutboundMessage{
		SessionKey: "",
		ReplyTo:    platform.InboundMessage{SessionKey: "local:correct-session"},
		Content:    "test",
	}
	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	replies, _ := replyStore.ListBySession(context.Background(), "local:correct-session")
	if len(replies) != 1 {
		t.Errorf("expected reply in correct session, got %d replies", len(replies))
	}
}
