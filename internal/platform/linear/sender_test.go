package linear

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

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

func TestSender_AppendsUploadedMarkdownToBody(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})

	tmp := t.TempDir()
	imgPath := tmp + "/snap.png"
	if err := os.WriteFile(imgPath, []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}

	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer s3.Close()

	fc := &fakeClient{viewer: User{ID: "BOT"}}
	fc.uploadImpl = func(name, mime string, size int) (FileUploadTicket, error) {
		return FileUploadTicket{
			AssetURL:  "https://uploads.linear.app/snap.png",
			UploadURL: s3.URL + "/sig",
		}, nil
	}

	s := &LinearSender{
		client:   fc,
		uploader: &uploader{client: fc, maxSize: 10 * 1024 * 1024, http: http.DefaultClient},
	}

	err := s.Send(context.Background(), platform.OutboundMessage{
		Content:   "see attached",
		MediaPath: imgPath,
		ReplyTo:   platform.InboundMessage{Raw: string(rawBytes)},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(fc.created) != 1 {
		t.Fatalf("expected 1 CreateComment, got %d", len(fc.created))
	}
	wantBody := "[openbee-bot]\n\nsee attached\n\n![snap.png](https://uploads.linear.app/snap.png)"
	if fc.created[0].Body != wantBody {
		t.Errorf("body = %q, want %q", fc.created[0].Body, wantBody)
	}
}

func TestSender_DeletesReactionAfterReply(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})
	pending := &sync.Map{}
	ch := make(chan string, 1)
	ch <- "R7"
	pending.Store("issue:I1", ch)

	fc := &fakeClient{viewer: User{ID: "BOT"}}
	s := &LinearSender{client: fc, pendingReactions: pending}
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hi",
		ReplyTo: platform.InboundMessage{
			Raw:               string(rawBytes),
			PlatformMessageID: "issue:I1",
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fc.mu.Lock()
		n := len(fc.reactionDeleted)
		fc.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.reactionDeleted) != 1 || fc.reactionDeleted[0] != "R7" {
		t.Errorf("reactionDeleted = %v, want [R7]", fc.reactionDeleted)
	}
	if _, ok := pending.Load("issue:I1"); ok {
		t.Error("pendingReactions still has key issue:I1 after Send")
	}
}

func TestSender_NoPendingReaction_StillSucceeds(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})
	fc := &fakeClient{viewer: User{ID: "BOT"}}
	s := &LinearSender{client: fc, pendingReactions: &sync.Map{}}
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hi",
		ReplyTo: platform.InboundMessage{
			Raw:               string(rawBytes),
			PlatformMessageID: "issue:I1",
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(fc.reactionDeleted) != 0 {
		t.Errorf("reactionDeleted = %v, want empty", fc.reactionDeleted)
	}
}

func TestSender_ReactionDeleteFails_StillSucceeds(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})
	pending := &sync.Map{}
	ch := make(chan string, 1)
	ch <- "R8"
	pending.Store("issue:I1", ch)

	fc := &fakeClient{
		viewer: User{ID: "BOT"},
		deleteReactionImpl: func(id string) error {
			return errors.New("boom")
		},
	}
	s := &LinearSender{client: fc, pendingReactions: pending}
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hi",
		ReplyTo: platform.InboundMessage{
			Raw:               string(rawBytes),
			PlatformMessageID: "issue:I1",
		},
	})
	if err != nil {
		t.Fatalf("Send must not propagate reaction delete failure; got %v", err)
	}
}
