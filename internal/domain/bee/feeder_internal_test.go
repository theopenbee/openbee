package bee

import (
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

func TestBuildPrompt_NoHint(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "feishu:oc_abc:ou_xyz", Content: "hello world"},
	}
	got := buildPrompt(msgs, "")
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

func TestBuildPrompt_WithHint(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "sk1", Content: "hi"},
	}
	got := buildPrompt(msgs, "use openbee-bee skill.")
	if !strings.HasPrefix(got, "use openbee-bee skill.\n") {
		t.Errorf("skill hint must be first line\ngot: %q", got)
	}
	if !strings.Contains(got, "<message_meta>") {
		t.Errorf("missing message_meta, got: %q", got)
	}
	if !strings.Contains(got, "hi") {
		t.Errorf("missing message content, got: %q", got)
	}
}

func TestBuildPrompt_MultipleMessages(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "sk1", Content: "first"},
		{ID: "msg-2", Platform: "feishu", SessionKey: "sk1", Content: "second"},
	}
	got := buildPrompt(msgs, "")
	if !strings.Contains(got, "msg-1") || !strings.Contains(got, "msg-2") {
		t.Errorf("missing message IDs, got: %q", got)
	}
	if strings.Count(got, "<message_meta>") != 2 {
		t.Errorf("expected 2 message_meta blocks, got: %q", got)
	}
}

func TestBuildPrompt_MultipleMessages_WithHint(t *testing.T) {
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "feishu", SessionKey: "sk1", Content: "first"},
		{ID: "msg-2", Platform: "feishu", SessionKey: "sk1", Content: "second"},
	}
	got := buildPrompt(msgs, "use openbee-bee skill.")
	if !strings.HasPrefix(got, "use openbee-bee skill.\n") {
		t.Errorf("skill hint must be first line\ngot: %q", got)
	}
	if strings.Count(got, "<message_meta>") != 2 {
		t.Errorf("expected 2 message_meta blocks, got: %q", got)
	}
}

func TestBuildPrompt_NeverHasPlatformContext(t *testing.T) {
	platform.RegisterExtractor("testplatform2", func(_ string) string {
		return `{"testplatform2":{"sender":{"open_id":"ou_abc"}}}`
	})
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "testplatform2", SessionKey: "testplatform2:oc_xyz:ou_abc", Content: "hello", Raw: "any-raw"},
	}
	got := buildPrompt(msgs, "")

	if strings.Contains(got, `"platform_context"`) {
		t.Errorf("platform_context must never appear in bee message_meta, got: %q", got)
	}
}
