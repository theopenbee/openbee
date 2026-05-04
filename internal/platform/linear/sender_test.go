package linear

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/theopenbee/openbee/internal/platform"
)

func TestSender_PostsCommentWithParentID(t *testing.T) {
	parent := "C0"
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1", ParentCommentID: &parent})

	fc := &fakeClient{viewer: User{ID: "BOT"}}
	s := &LinearSender{client: fc}
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hello",
		ReplyTo: platform.InboundMessage{Raw: string(rawBytes)},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(fc.created) != 1 {
		t.Fatalf("expected 1 CreateComment call, got %d", len(fc.created))
	}
	c := fc.created[0]
	if c.IssueID != "I1" || c.ParentID == nil || *c.ParentID != "C0" {
		t.Errorf("unexpected call: %+v", c)
	}
	// Body must begin with the self-marker prefix and end with the caller's content.
	if c.Body != "[openbee-bot]\n\nhello" {
		t.Errorf("body = %q, want %q", c.Body, "[openbee-bot]\n\nhello")
	}
}

func TestSender_RejectsMediaPath(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})
	s := &LinearSender{client: &fakeClient{}}
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content:   "x",
		MediaPath: "/tmp/foo.png",
		ReplyTo:   platform.InboundMessage{Raw: string(rawBytes)},
	})
	if err == nil {
		t.Error("expected error for MediaPath")
	}
}
