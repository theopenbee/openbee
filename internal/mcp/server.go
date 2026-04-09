package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/domain/worker"
)

var log = logger.With(zap.String("component", "mcp"))

type ctxKey string

// CtxWorkerIDKey carries the caller's worker ID through tool dispatch.
// It uses the same string value as CtxKeyWorkerID (set by auth middleware on gin.Context)
// so the two stay in sync without a hard import dependency between packages.
// Exported so tests can construct contexts that simulate worker calls.
const CtxWorkerIDKey ctxKey = CtxKeyWorkerID

// ExecutionStopper can kill a running worker process by execution ID.
type ExecutionStopper interface {
	StopExecution(executionID string) error
}

// SessionClearer clears dispatcher queues and session contexts for a session.
type SessionClearer interface {
	ClearSession(sessionKey string)
}

// MCPServer dispatches tool calls.
type MCPServer struct {
	workerStore     *store.WorkerStore
	manager         *worker.Manager
	taskStore       *store.TaskStore
	messageStore    *store.MessageStore
	senders         map[string]platform.PlatformSenderAdapter
	execStopper     ExecutionStopper
	sessionClearer  SessionClearer
	executionStore  *store.ExecutionStore
	memoryStore     *store.MemoryStore
	sessionStore    *store.SessionStore
	departmentStore *store.DepartmentStore

	workerNameCache sync.Map // workerID -> display name; lazily populated
}

// NewBeeServer creates a Bee MCP Server with all tools.
func NewBeeServer(
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
	ds *store.DepartmentStore,
) *MCPServer {
	return &MCPServer{
		workerStore:     ws,
		manager:         mgr,
		taskStore:       ts,
		messageStore:    ms,
		senders:         senders,
		execStopper:     execStopper,
		sessionClearer:  sessionClearer,
		executionStore:  es,
		memoryStore:     memStore,
		sessionStore:    sessionStore,
		departmentStore: ds,
	}
}

func (s *MCPServer) workerIDContext(c *gin.Context) context.Context {
	return context.WithValue(c.Request.Context(), CtxWorkerIDKey, c.GetString(CtxKeyWorkerID))
}

// Tool errors are returned as 200 {"error": "..."} to match the RPC-over-HTTP convention.
func (s *MCPServer) HandleCall(c *gin.Context) {
	var req struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	result, err := s.beeCallTool(s.workerIDContext(c), req.Name, req.Arguments)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}
