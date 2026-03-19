package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/store"
	"github.com/theopenbee/openbee/internal/worker"
)

var log = logger.With(zap.String("component", "mcp"))

// JSON-RPC 2.0 types

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func errResponse(id any, code int, msg string) rpcResponse {
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	}
}

func okResponse(id any, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

// ExecutionStopper can kill a running worker process by execution ID.
type ExecutionStopper interface {
	StopExecution(executionID string) error
}

// SessionClearer clears dispatcher queues and session contexts for a session.
type SessionClearer interface {
	ClearSession(sessionKey string)
}

// MCPServer manages SSE sessions and dispatches JSON-RPC tool calls.
type MCPServer struct {
	workerStore    *store.WorkerStore
	manager        *worker.Manager
	taskStore      *store.TaskStore
	messageStore   *store.MessageStore
	senders        map[string]platform.PlatformSenderAdapter
	execStopper    ExecutionStopper
	sessionClearer SessionClearer
	executionStore *store.ExecutionStore
	memoryStore    *store.MemoryStore
	sessionStore   *store.SessionStore

	mu       sync.Mutex
	sessions map[string]chan rpcResponse // session_id -> response channel
}

// NewServer creates an MCPServer. Call RegisterRoutes to attach it to a Gin router group.
func NewServer(
	ws *store.WorkerStore,
	mgr *worker.Manager,
	ts *store.TaskStore,
	ms *store.MessageStore,
	senders map[string]platform.PlatformSenderAdapter,
	execStopper ExecutionStopper,
	sessionClearer SessionClearer,
	es *store.ExecutionStore,
	memStore *store.MemoryStore,
	sessionStore *store.SessionStore,
) *MCPServer {
	return &MCPServer{
		workerStore:    ws,
		manager:        mgr,
		taskStore:      ts,
		messageStore:   ms,
		senders:        senders,
		execStopper:    execStopper,
		sessionClearer: sessionClearer,
		executionStore: es,
		memoryStore:    memStore,
		sessionStore:   sessionStore,
		sessions:       make(map[string]chan rpcResponse),
	}
}

// RegisterRoutes attaches auth middleware, /sse, and /messages to the provided router group.
func (s *MCPServer) RegisterRoutes(rg *gin.RouterGroup, apiKey string) {
	rg.Use(APIKeyMiddleware(apiKey))
	rg.GET("/sse", s.handleSSE)
	rg.POST("/messages", s.handleMessages)
}

// handleSSE establishes the SSE connection, creates a session, and streams responses.
func (s *MCPServer) handleSSE(c *gin.Context) {
	sessionID := uuid.New().String()
	ch := make(chan rpcResponse, 16)

	s.mu.Lock()
	s.sessions[sessionID] = ch
	s.mu.Unlock()

	log.Info("MCP SSE connected", zap.String("session", sessionID), zap.String("client", c.ClientIP()))

	defer func() {
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		log.Info("MCP SSE handler exited", zap.String("session", sessionID))
	}()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// Send endpoint event so client knows where to POST.
	// API key is included because Claude CLI's MCP SDK uses the endpoint URL directly
	// and does not add X-API-Key header automatically.
	apiKey := c.Query("api_key")
	params := url.Values{}
	params.Set("session_id", sessionID)
	if apiKey != "" {
		params.Set("api_key", apiKey)
	}
	endpointURL := fmt.Sprintf("/mcp/messages?%s", params.Encode())
	n, err := fmt.Fprintf(c.Writer, "event: endpoint\ndata: %s\n\n", endpointURL)
	log.Info("MCP SSE wrote endpoint event", zap.String("session", sessionID), zap.Int("bytes", n), zap.Any("err", err))
	c.Writer.Flush()
	log.Info("MCP SSE flushed endpoint event", zap.String("session", sessionID))

	ctx := c.Request.Context()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("MCP SSE context done", zap.String("session", sessionID), zap.Any("reason", ctx.Err()))
			return
		case <-heartbeat.C:
			log.Info("MCP SSE sending heartbeat", zap.String("session", sessionID))
			fmt.Fprintf(c.Writer, ": heartbeat\n\n")
			c.Writer.Flush()
		case resp, ok := <-ch:
			if !ok {
				log.Info("MCP SSE channel closed", zap.String("session", sessionID))
				return
			}
			data, _ := json.Marshal(resp)
			log.Info("MCP SSE sending message event", zap.String("session", sessionID), zap.Any("id", resp.ID))
			fmt.Fprintf(c.Writer, "event: message\ndata: %s\n\n", data)
			c.Writer.Flush()
		}
	}
}

// handleMessages receives a JSON-RPC request and pushes the response to the SSE channel.
func (s *MCPServer) handleMessages(c *gin.Context) {
	sessionID := c.Query("session_id")

	s.mu.Lock()
	ch, ok := s.sessions[sessionID]
	s.mu.Unlock()

	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown session_id"})
		return
	}

	var req rpcRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ch <- errResponse(nil, -32700, "parse error: "+err.Error())
		c.Status(http.StatusAccepted)
		return
	}

	resp := s.dispatch(req)

	// Notifications (no ID) get no response
	if req.ID != nil {
		ch <- resp
	}

	c.Status(http.StatusAccepted)
}

// dispatch routes a JSON-RPC request to the appropriate handler.
func (s *MCPServer) dispatch(req rpcRequest) rpcResponse {
	switch req.Method {
	case "initialize":
		return okResponse(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]string{"name": "openbee-mcp", "version": "1.0.0"},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		})

	case "initialized":
		// Notification — no response needed
		return rpcResponse{}

	case "ping":
		return okResponse(req.ID, map[string]any{})

	case "notifications/cancelled", "notifications/initialized":
		// Client notifications — no response needed
		return rpcResponse{}

	case "tools/list":
		return okResponse(req.ID, map[string]any{"tools": toolSchemas()})

	case "tools/call":
		return s.handleToolCall(req)

	default:
		return errResponse(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

// handleToolCall dispatches tools/call to the appropriate tool handler.
func (s *MCPServer) handleToolCall(req rpcRequest) rpcResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	result, err := s.callTool(params.Name, params.Arguments)
	if err != nil {
		// Tool execution errors are returned as tool results with isError flag,
		// not as JSON-RPC errors. This lets the LLM client distinguish between
		// protocol errors and tool failures.
		return okResponse(req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		})
	}

	data, _ := json.Marshal(result)
	return okResponse(req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(data)}},
	})
}
