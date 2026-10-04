package bee

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// TestBuildPrompt merges the buildPrompt scenarios for hint/no-hint and
// single/multiple messages.
func TestBuildPrompt(t *testing.T) {
	cases := []struct {
		name          string
		msgs          []store.ClaimedMessage
		hint          string
		wantPrefix    string
		wantContains  []string
		wantMetaCount int // 0 means skip the count check
	}{
		{
			name: "NoHint",
			msgs: []store.ClaimedMessage{
				{ID: "msg-1", Platform: "feishu", SessionKey: "feishu:oc_abc:ou_xyz", Content: "hello world"},
			},
			wantPrefix:   `<message_meta>{"from":"feishu","session_key":"feishu:oc_abc:ou_xyz","message_id":"msg-1"}</message_meta>`,
			wantContains: []string{"<message_content>", "</message_content>", "hello world"},
		},
		{
			name: "WithHint",
			msgs: []store.ClaimedMessage{
				{ID: "msg-1", Platform: "feishu", SessionKey: "sk1", Content: "hi"},
			},
			hint:         "use openbee-bee skill.",
			wantPrefix:   "use openbee-bee skill.\n",
			wantContains: []string{"<message_meta>", "hi"},
		},
		{
			name: "MultipleMessages",
			msgs: []store.ClaimedMessage{
				{ID: "msg-1", Platform: "feishu", SessionKey: "sk1", Content: "first"},
				{ID: "msg-2", Platform: "feishu", SessionKey: "sk1", Content: "second"},
			},
			wantContains:  []string{"msg-1", "msg-2"},
			wantMetaCount: 2,
		},
		{
			name: "MultipleMessages_WithHint",
			msgs: []store.ClaimedMessage{
				{ID: "msg-1", Platform: "feishu", SessionKey: "sk1", Content: "first"},
				{ID: "msg-2", Platform: "feishu", SessionKey: "sk1", Content: "second"},
			},
			hint:          "use openbee-bee skill.",
			wantPrefix:    "use openbee-bee skill.\n",
			wantMetaCount: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPrompt(tc.msgs, tc.hint)
			if tc.wantPrefix != "" {
				assert.True(t, strings.HasPrefix(got, tc.wantPrefix))
			}
			for _, want := range tc.wantContains {
				assert.Contains(t, got, want)
			}
			if tc.wantMetaCount > 0 {
				assert.Equal(t, tc.wantMetaCount, strings.Count(got, "<message_meta>"))
			}
		})
	}
}

func TestBuildPrompt_NeverHasPlatformContext(t *testing.T) {
	platform.RegisterExtractor("testplatform2", func(_ string) string {
		return `{"testplatform2":{"sender":{"open_id":"ou_abc"}}}`
	})
	msgs := []store.ClaimedMessage{
		{ID: "msg-1", Platform: "testplatform2", SessionKey: "testplatform2:oc_xyz:ou_abc", Content: "hello"},
	}
	got := buildPrompt(msgs, "")

	assert.NotContains(t, got, `"platform_context"`, "platform_context must never appear in bee message_meta")
}
