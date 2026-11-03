package rpc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/rpc"
)

const mcpTestSecret = "mcp-test-secret"

type headerTransport struct {
	header string
	value  string
}

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set(h.header, h.value)
	return http.DefaultTransport.RoundTrip(r)
}

func startMCP(t *testing.T, s *rpc.Server) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/mcp", rpc.JWTAuthMiddleware(mcpTestSecret), rpc.RequireBeeOrWorker(), s.MCPHandler())
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts.URL + "/mcp"
}

func connectMCP(t *testing.T, url, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   url,
		HTTPClient: &http.Client{Transport: headerTransport{header: "Authorization", value: "Bearer " + token}},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })
	return session
}

func beeToken(t *testing.T) string {
	t.Helper()
	token, err := auth.GenerateBeeToken(mcpTestSecret, time.Hour)
	require.NoError(t, err)
	return token
}

func workerToken(t *testing.T, workerID string, scopes ...string) string {
	t.Helper()
	token, err := auth.GenerateWorkerToken(mcpTestSecret, workerID, scopes, time.Hour)
	require.NoError(t, err)
	return token
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	return text.Text, res.IsError
}

func executeDescription(t *testing.T, session *mcp.ClientSession) string {
	t.Helper()
	res, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(res.Tools))
	desc := ""
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if tool.Name == rpc.ExecuteTool {
			desc = tool.Description
		}
	}
	assert.ElementsMatch(t, []string{"send_message", rpc.ExecuteTool}, names)
	return desc
}

func TestMCPRejectsMissingToken(t *testing.T) {
	url := startMCP(t, setupServerWithMessaging(t))
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestMCPBeeSeesFullAPI(t *testing.T) {
	session := connectMCP(t, startMCP(t, setupServerWithMessaging(t)), beeToken(t))
	desc := executeDescription(t, session)
	for _, name := range []string{"create_worker(", "create_task(", "clear_session(", "save_constraint(", "list_workers("} {
		assert.Contains(t, desc, name)
	}
}

func TestMCPWorkerSeesReadOnlyAPI(t *testing.T) {
	session := connectMCP(t, startMCP(t, setupServerWithMessaging(t)), workerToken(t, "w-1", auth.ScopeReadWorkers))
	desc := executeDescription(t, session)
	assert.Contains(t, desc, "list_workers(")
	assert.Contains(t, desc, "list_tasks(")
	for _, name := range []string{"create_worker(", "update_worker(", "create_task(", "clear_session(", "save_constraint("} {
		assert.NotContains(t, desc, name)
	}
}

func TestMCPExecuteReturnsJSON(t *testing.T) {
	s := setupServerWithMessaging(t)
	mustCreateWorker(t, s, map[string]any{"name": "MaoMao"})
	session := connectMCP(t, startMCP(t, s), beeToken(t))

	text, isErr := callTool(t, session, rpc.ExecuteTool, map[string]any{"code": `
const { items, total } = await openbee.list_workers();
console.log("found", total);
return items.map((w) => w.name);`})
	require.False(t, isErr, text)
	assert.Contains(t, text, `"MaoMao"`)
	assert.Contains(t, text, "console:\nfound 1")
}

func TestMCPExecuteReportsToolErrors(t *testing.T) {
	session := connectMCP(t, startMCP(t, setupServerWithMessaging(t)), beeToken(t))

	text, isErr := callTool(t, session, rpc.ExecuteTool, map[string]any{"code": `await openbee.get_worker({ worker_id: "missing" });`})
	assert.True(t, isErr)
	assert.Contains(t, text, "worker not found")

	text, isErr = callTool(t, session, rpc.ExecuteTool, map[string]any{"code": `
try {
  await openbee.get_worker({});
} catch (e) {
  return "caught: " + e.message;
}`})
	assert.False(t, isErr, text)
	assert.Equal(t, "caught: worker_id is required", text)
}

func TestMCPExecuteSyntaxError(t *testing.T) {
	session := connectMCP(t, startMCP(t, setupServerWithMessaging(t)), beeToken(t))
	text, isErr := callTool(t, session, rpc.ExecuteTool, map[string]any{"code": `const x: string = "a";`})
	assert.True(t, isErr)
	assert.Contains(t, text, "SyntaxError")
}

func TestMCPWorkerCannotCallWriteTools(t *testing.T) {
	s := setupServerWithMessaging(t)
	w := mustCreateWorker(t, s, map[string]any{"name": "MaoMao"})
	session := connectMCP(t, startMCP(t, s), workerToken(t, w.ID, auth.ScopeReadWorkers))

	text, isErr := callTool(t, session, rpc.ExecuteTool, map[string]any{"code": `return typeof openbee.update_worker;`})
	require.False(t, isErr, text)
	assert.Equal(t, "undefined", text)

	text, isErr = callTool(t, session, rpc.ExecuteTool, map[string]any{"code": `return (await openbee.list_workers()).total;`})
	require.False(t, isErr, text)
	assert.Equal(t, "1", text)
}

func TestMCPWorkerScopesStillApply(t *testing.T) {
	session := connectMCP(t, startMCP(t, setupServerWithMessaging(t)), workerToken(t, "w-1"))
	text, isErr := callTool(t, session, rpc.ExecuteTool, map[string]any{"code": `return await openbee.list_workers();`})
	assert.True(t, isErr)
	assert.Contains(t, text, "scope read:workers required")
}

func TestMCPSendMessage(t *testing.T) {
	spy := &mockSender{}
	s, db := setupServerWithSender(t, "telegram", spy)
	w := mustCreateWorker(t, s, map[string]any{"name": "MaoMao"})
	_, err := store.NewMessageStore(db).Create(context.Background(), "msg-mcp", "telegram:chat1:userA", "telegram", "hi", `{}`, "", 0)
	require.NoError(t, err)
	session := connectMCP(t, startMCP(t, s), workerToken(t, w.ID))

	text, isErr := callTool(t, session, "send_message", map[string]any{"message_id": "msg-mcp", "content": "done `x`"})
	require.False(t, isErr, text)
	assert.JSONEq(t, `{"status":"sent"}`, text)

	text, isErr = callTool(t, session, "send_message", map[string]any{"message_id": "msg-mcp"})
	assert.True(t, isErr)
	assert.Contains(t, text, "content")

	spy.mu.Lock()
	defer spy.mu.Unlock()
	require.Len(t, spy.sent, 1)
	assert.Equal(t, "MaoMao\ndone `x`", spy.sent[0].Content)
}
