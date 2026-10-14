package feishu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform"
)

type feishuRequest struct {
	path  string
	query string
	body  map[string]any
}

func newFeishuTestSender(t *testing.T) (*FeishuSender, func() []feishuRequest) {
	t.Helper()
	var mu sync.Mutex
	var reqs []feishuRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			io.WriteString(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`) //nolint:errcheck
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(raw, &body) //nolint:errcheck
		mu.Lock()
		reqs = append(reqs, feishuRequest{path: r.URL.Path, query: r.URL.RawQuery, body: body})
		mu.Unlock()
		io.WriteString(w, `{"code":0,"msg":"ok","data":{}}`) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	client := lark.NewClient("app", "secret", lark.WithOpenBaseUrl(srv.URL))
	sender := &FeishuSender{larkClient: client, pendingReactions: &sync.Map{}}
	return sender, func() []feishuRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]feishuRequest{}, reqs...)
	}
}

func TestFeishuSender_ProactiveCreatesMessageInSessionChat(t *testing.T) {
	sender, requests := newFeishuTestSender(t)
	stale := `{"event":{"message":{"chat_id":"oc_old","chat_type":"group","message_id":"om_old"}}}`

	err := sender.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "feishu:oc_group:ou_user",
		Content:    "daily report",
		ReplyTo:    platform.InboundMessage{Platform: PlatformID, SessionKey: "feishu:oc_group:ou_user", Raw: stale},
		Proactive:  true,
	})
	require.NoError(t, err)

	got := requests()
	require.Len(t, got, 1)
	assert.Equal(t, "/open-apis/im/v1/messages", got[0].path)
	assert.Contains(t, got[0].query, "receive_id_type=chat_id")
	assert.Equal(t, "oc_group", got[0].body["receive_id"])
	assert.Equal(t, "post", got[0].body["msg_type"])
}

func TestFeishuSender_GroupReplyStillRepliesToMessage(t *testing.T) {
	sender, requests := newFeishuTestSender(t)
	raw := `{"event":{"message":{"chat_id":"oc_group","chat_type":"group","message_id":"om_1"}}}`

	err := sender.Send(context.Background(), platform.OutboundMessage{
		Content: "done",
		ReplyTo: platform.InboundMessage{Platform: PlatformID, SessionKey: "feishu:oc_group:ou_user", Raw: raw},
	})
	require.NoError(t, err)

	got := requests()
	require.Len(t, got, 1)
	assert.Equal(t, "/open-apis/im/v1/messages/om_1/reply", got[0].path)
}

func TestFeishuSender_ProactiveRejectsInvalidSessionKey(t *testing.T) {
	sender, requests := newFeishuTestSender(t)

	err := sender.Send(context.Background(), platform.OutboundMessage{
		SessionKey: "wecom:chat:user",
		Content:    "hi",
		Proactive:  true,
	})
	require.Error(t, err)
	assert.Empty(t, requests())
}
