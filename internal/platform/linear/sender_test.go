package linear

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, err)
	require.Len(t, fc.created, 1)
	c := fc.created[0]
	assert.Equal(t, "I1", c.IssueID)
	require.NotNil(t, c.ParentID) // guard: dereferenced below
	assert.Equal(t, "C0", *c.ParentID)
	// Body must begin with the self-marker prefix and end with the caller's content.
	assert.Equal(t, "[openbee-bot]\n\nhello", c.Body)
}

func TestSender_AppendsUploadedMarkdownToBody(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})

	tmp := t.TempDir()
	imgPath := tmp + "/snap.png"
	require.NoError(t, os.WriteFile(imgPath, []byte("PNG"), 0o644))

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
	require.NoError(t, err)
	require.Len(t, fc.created, 1)
	wantBody := "[openbee-bot]\n\nsee attached\n\n![snap.png](https://uploads.linear.app/snap.png)"
	assert.Equal(t, wantBody, fc.created[0].Body)
}
