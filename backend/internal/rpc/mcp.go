package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

const (
	ExecuteTool        = "execute"
	sendMessageTimeout = 50 * time.Second
)

const mcpInstructions = "openbee coordinates the user's AI workers. Reach the user only through send_message; your text output is never shown to them. Use execute to query and manage workers, tasks, sessions, constraints and message history."

const sendMessageDescription = `Send a message to the user on their chat platform. This is the only way to reach the user: your text output is never shown to them.

Pass message_id to reply to that message (from message_meta or task_meta), or session_key to post a new message to the conversation (scheduled and countdown tasks). content is the text. media_path is one local file to upload, such as an image or a document; send several files with several calls. With both, the text goes first.`

const executeDescription = `Run JavaScript against the openbee API and get the result back.

The code is the body of an async function: use await, and return the value you want back. Objects come back as JSON. console.log output comes back too. Write plain JavaScript: no TypeScript syntax, imports, fetch, file system or timers. Every openbee.* call returns a Promise that rejects with an Error when the call fails. Combine calls and filter results in one script instead of making many tool calls. A script stops after 50 seconds.

Example:
` + "```js" + `
const { items } = await openbee.list_workers();
return items.map((w) => ({ id: w.id, name: w.name }));
` + "```" + `

API:
` + "```ts" + `
%s
` + "```"

var sendMessageSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "message_id": { "type": "string", "description": "Reply to this message" },
    "session_key": { "type": "string", "description": "Post a new message to this conversation instead of replying" },
    "content": { "type": "string", "description": "Message text" },
    "media_path": { "type": "string", "description": "Absolute path of one local file to send" }
  },
  "additionalProperties": false
}`)

var executeSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "code": { "type": "string", "description": "JavaScript async function body that calls openbee.* and returns a value" }
  },
  "required": ["code"],
  "additionalProperties": false
}`)

type mcpServerKey struct{}

func (s *Server) MCPHandler() gin.HandlerFunc {
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		build, _ := r.Context().Value(mcpServerKey{}).(func() *mcp.Server)
		if build == nil {
			return nil
		}
		return build()
	}, &mcp.StreamableHTTPOptions{
		Stateless:                  true,
		JSONResponse:               true,
		DisableLocalhostProtection: true,
	})
	return func(gc *gin.Context) {
		c := callerFromGin(gc)
		var srv *mcp.Server
		build := func() *mcp.Server {
			if srv == nil {
				srv = s.newMCPServer(c)
			}
			return srv
		}
		ctx := context.WithValue(gc.Request.Context(), mcpServerKey{}, build)
		h.ServeHTTP(gc.Writer, gc.Request.WithContext(ctx))
	}
}

func callerFromGin(gc *gin.Context) caller {
	scopes, _ := gc.Get(CtxKeyScopes)
	list, _ := scopes.([]string)
	return caller{workerID: gc.GetString(CtxKeyWorkerID), scopes: list}
}

func (s *Server) newMCPServer(c caller) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: ai.MCPServerName, Version: "1"}, &mcp.ServerOptions{
		Instructions: mcpInstructions,
		Capabilities: &mcp.ServerCapabilities{},
	})
	srv.AddTool(&mcp.Tool{
		Name:        utils.SendMessage,
		Description: sendMessageDescription,
		InputSchema: sendMessageSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(ctx, sendMessageTimeout)
		defer cancel()
		args := req.Params.Arguments
		if len(args) == 0 {
			args = json.RawMessage("{}")
		}
		result, err := s.beeCallTool(c.context(ctx), utils.SendMessage, args)
		if err != nil {
			return toolError(err.Error()), nil
		}
		b, err := json.Marshal(result)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return toolText(string(b)), nil
	})
	srv.AddTool(&mcp.Tool{
		Name:        ExecuteTool,
		Description: fmt.Sprintf(executeDescription, c.declaration()),
		InputSchema: executeSchema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &params); err != nil || params.Code == "" {
			return toolError("code is required"), nil
		}
		out, err := s.runScript(ctx, c, params.Code)
		if err != nil {
			log.Debug("execute failed", zap.String("worker_id", c.workerID), zap.Error(err))
			if out != "" {
				return toolError("Script failed: " + err.Error() + "\n\n" + out), nil
			}
			return toolError("Script failed: " + err.Error()), nil
		}
		if out == "" {
			out = "Done. The script returned nothing."
		}
		return toolText(out), nil
	})
	return srv
}

func toolText(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func toolError(text string) *mcp.CallToolResult {
	r := toolText(text)
	r.IsError = true
	return r
}
