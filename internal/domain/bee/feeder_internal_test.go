package bee

import (
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/store"
)

func TestBuildPrompt(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "feishu:oc_abc:ou_xyz", Content: "hello world"},
	}
	got := buildPrompt(msgs)
	wantMeta := `<message_meta>{"from":"feishu","session_key":"feishu:oc_abc:ou_xyz","message_id":"msg-1"}</message_meta>`
	if !strings.HasPrefix(got, wantMeta) {
		t.Errorf("missing message_meta prefix\ngot:  %q", got)
	}
	if !strings.Contains(got, "<message_content>") {
		t.Errorf("missing message_content tag, got: %q", got)
	}
	if !strings.Contains(got, "</message_content>") {
		t.Errorf("missing closing message_content tag, got: %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("missing original content, got: %q", got)
	}
}

func TestBuildPrompt_MultipleMessages(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "sk1", Content: "first"},
		{ID: "msg-2", Platform: "feishu", SessionKey: "sk1", Content: "second"},
	}
	got := buildPrompt(msgs)
	if !strings.Contains(got, "msg-1") || !strings.Contains(got, "msg-2") {
		t.Errorf("missing message IDs, got: %q", got)
	}
	if strings.Count(got, "<message_meta>") != 2 {
		t.Errorf("expected 2 message_meta blocks, got: %q", got)
	}
}
