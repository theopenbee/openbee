package rpc

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

var log = logger.With(zap.String("component", "rpc"))

type ctxKey string

// Uses the same string value as CtxKeyWorkerID (auth middleware on gin.Context)
// so the two stay in sync without a hard import dependency between packages.
// Exported so tests can construct contexts that simulate worker calls.
const CtxWorkerIDKey ctxKey = CtxKeyWorkerID

// Mirrors CtxKeyScopes from auth middleware; see CtxWorkerIDKey comment above.
const CtxScopesKey ctxKey = CtxKeyScopes

// ExecutionStopper can kill a running worker process by execution ID.
type ExecutionStopper interface {
	StopExecution(executionID string) error
}

// SessionClearer clears dispatcher queues and session contexts for a session.
type SessionClearer interface {
	ClearSession(sessionKey string)
}

type TaskCanceller interface {
	CancelTask(ctx context.Context, taskID string) error
}

// Server dispatches tool calls.
type Server struct {
	workerStore          *store.WorkerStore
	manager              *worker.Manager
	taskStore            *store.TaskStore
	messageStore         *store.MessageStore
	outboundMessageStore *store.OutboundMessageStore
	senders              map[string]platform.PlatformSenderAdapter
	execStopper          ExecutionStopper
	sessionClearer       SessionClearer
	taskCanceller        TaskCanceller
	executionStore       *store.ExecutionStore
	constraintStore      *store.ConstraintStore
	sessionStore         *store.SessionStore
	departmentStore      *store.DepartmentStore

	workerNameCache sync.Map // workerID -> display name; lazily populated
}

func NewBeeServer(
	ws *store.WorkerStore,
	mgr *worker.Manager,
	ts *store.TaskStore,
	ms *store.MessageStore,
	oms *store.OutboundMessageStore,
	senders map[string]platform.PlatformSenderAdapter,
	execStopper ExecutionStopper,
	sessionClearer SessionClearer,
	taskCanceller TaskCanceller,
	es *store.ExecutionStore,
	constraintStore *store.ConstraintStore,
	sessionStore *store.SessionStore,
	ds *store.DepartmentStore,
) *Server {
	return &Server{
		workerStore:          ws,
		manager:              mgr,
		taskStore:            ts,
		messageStore:         ms,
		outboundMessageStore: oms,
		senders:              senders,
		execStopper:          execStopper,
		sessionClearer:       sessionClearer,
		taskCanceller:        taskCanceller,
		executionStore:       es,
		constraintStore:      constraintStore,
		sessionStore:         sessionStore,
		departmentStore:      ds,
	}
}

func (s *Server) workerIDContext(c *gin.Context) context.Context {
	ctx := context.WithValue(c.Request.Context(), CtxWorkerIDKey, c.GetString(CtxKeyWorkerID))
	scopes, _ := c.Get(CtxKeyScopes)
	return context.WithValue(ctx, CtxScopesKey, scopes)
}

// Tool errors are returned as 200 {"error": "..."} to match the RPC-over-HTTP convention.
func (s *Server) HandleCall(c *gin.Context) {
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
