package linear

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/media"
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
		uploader: &uploader{client: fc, media: media.NewService(), maxSize: 10 * 1024 * 1024, http: http.DefaultClient},
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

