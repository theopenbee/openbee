package bee

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

var ciLog = logger.With(zap.String("component", "command_interceptor"))

// executionStopper can kill a running process by execution ID.
type executionStopper interface {
	StopExecution(executionID string) error
}

// sessionClearer clears dispatcher queues and session contexts for a session.
type sessionClearer interface {
	ClearSession(sessionKey string)
}

// messageCanceller cancels unprocessed platform messages for a session.
type messageCanceller interface {
	CancelReceivedBySessionKey(ctx context.Context, sessionKey string) (int64, error)
}

const cmdStop = "/stop"

// CommandInterceptor intercepts system slash commands before Feeder dispatches to Bee.
type CommandInterceptor struct {
	sessionStore *store.SessionStore
	execStore    *store.ExecutionStore
	taskStore    *store.TaskStore
	execStopper  executionStopper
	dispatcher   sessionClearer
	msgCanceller messageCanceller
	senders      map[string]platform.PlatformSenderAdapter
	engine       string
	// inFlight tracks sessions with an active /stop goroutine to prevent duplicate concurrent stops.
	inFlight sync.Map
}

func NewCommandInterceptor(
	ss *store.SessionStore,
	es *store.ExecutionStore,
	ts *store.TaskStore,
	stopper executionStopper,
	clearer sessionClearer,
	canceller messageCanceller,
	senders map[string]platform.PlatformSenderAdapter,
	engine string,
) *CommandInterceptor {
	return &CommandInterceptor{
		sessionStore: ss,
		execStore:    es,
		taskStore:    ts,
		execStopper:  stopper,
		dispatcher:   clearer,
		msgCanceller: canceller,
		senders:      senders,
		engine:       engine,
	}
}

func isStopCommand(content string) bool {
	return strings.EqualFold(strings.TrimSpace(content), cmdStop)
}

// InterceptInbound implements msgingest.InboundInterceptor.
func (c *CommandInterceptor) InterceptInbound(ctx context.Context, msg platform.InboundMessage) bool {
	if !isStopCommand(msg.Content) {
		return false
	}
	ciLog.Debug("stop command received", zap.String("sessionKey", msg.SessionKey), zap.String("platform", msg.Platform))
	// Drop duplicate /stop if one is already in flight for this session.
	if _, loaded := c.inFlight.LoadOrStore(msg.SessionKey, struct{}{}); loaded {
		ciLog.Debug("stop command already in flight, dropping duplicate", zap.String("sessionKey", msg.SessionKey))
		return true
	}
	go func() {
		defer c.inFlight.Delete(msg.SessionKey)
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c.handleStop(stopCtx, store.ClaimedMessage{
			SessionKey: msg.SessionKey,
			Platform:   msg.Platform,
			Content:    msg.Content,
		})
	}()
	return true
}

// handleStop stops all active work for the session and replies to the user.
// Sub-step errors are logged and do not abort the sequence.
func (c *CommandInterceptor) handleStop(ctx context.Context, msg store.ClaimedMessage) {
	ciLog.Debug("stop command: handleStop started", zap.String("sessionKey", msg.SessionKey), zap.String("engine", c.engine))
	stopped := false

	// Step 1: stop active Bee executions.
	sessionID, err := c.sessionStore.GetSessionContextForEngine(ctx, msg.SessionKey, store.BeeAgentID, c.engine)
	if err != nil {
		ciLog.Warn("stop command: get session context", zap.String("sessionKey", msg.SessionKey), zap.Error(err))
	} else if sessionID == "" {
		ciLog.Debug("stop command: no session context found, skipping execution stop", zap.String("sessionKey", msg.SessionKey))
	} else {
		ciLog.Debug("stop command: found session context", zap.String("sessionKey", msg.SessionKey), zap.String("sessionID", sessionID))
		execIDs, listErr := c.execStore.ListActiveIDsBySessionID(sessionID)
		if listErr != nil {
			ciLog.Warn("stop command: list executions", zap.String("sessionID", sessionID), zap.Error(listErr))
		} else {
			ciLog.Debug("stop command: active executions found", zap.String("sessionID", sessionID), zap.Int("count", len(execIDs)), zap.Strings("execIDs", execIDs))
		}
		for _, id := range execIDs {
			if stopErr := c.execStopper.StopExecution(id); stopErr != nil {
				ciLog.Warn("stop command: stop execution", zap.String("execID", id), zap.Error(stopErr))
			} else {
				ciLog.Debug("stop command: execution stopped", zap.String("execID", id))
				stopped = true
			}
		}
	}

	// Step 2: cancel pending/running worker tasks.
	ciLog.Debug("stop command: cancelling worker tasks", zap.String("sessionKey", msg.SessionKey))
	n, err := c.taskStore.CancelBySessionKey(ctx, msg.SessionKey)
	if err != nil {
		ciLog.Warn("stop command: cancel tasks", zap.String("sessionKey", msg.SessionKey), zap.Error(err))
	} else {
		ciLog.Debug("stop command: worker tasks cancelled", zap.String("sessionKey", msg.SessionKey), zap.Int64("count", n))
		if n > 0 {
			stopped = true
		}
	}

	// Step 3: cancel unprocessed platform messages (received/merged).
	ciLog.Debug("stop command: cancelling platform messages", zap.String("sessionKey", msg.SessionKey))
	n, err = c.msgCanceller.CancelReceivedBySessionKey(ctx, msg.SessionKey)
	if err != nil {
		ciLog.Warn("stop command: cancel platform messages", zap.String("sessionKey", msg.SessionKey), zap.Error(err))
	} else {
		ciLog.Debug("stop command: platform messages cancelled", zap.String("sessionKey", msg.SessionKey), zap.Int64("count", n))
		if n > 0 {
			stopped = true
		}
	}

	// Step 4: clear dispatcher in-memory queues and session contexts.
	ciLog.Debug("stop command: clearing dispatcher session", zap.String("sessionKey", msg.SessionKey))
	c.dispatcher.ClearSession(msg.SessionKey)
	ciLog.Debug("stop command: dispatcher session cleared", zap.String("sessionKey", msg.SessionKey))

	// Step 5: reply to user.
	m := i18n.M.Runtime.CommandInterceptor
	replyContent := m.Stopped
	if !stopped {
		replyContent = m.NothingRan
	}
	ciLog.Debug("stop command: sending reply", zap.String("sessionKey", msg.SessionKey), zap.Bool("stopped", stopped))
	c.sendReply(ctx, msg, replyContent)
	ciLog.Debug("stop command: handleStop complete", zap.String("sessionKey", msg.SessionKey), zap.Bool("stopped", stopped))
}

func (c *CommandInterceptor) sendReply(ctx context.Context, msg store.ClaimedMessage, content string) {
	sender, ok := c.senders[msg.Platform]
	if !ok {
		ciLog.Warn("stop command: no sender for platform", zap.String("platform", msg.Platform))
		return
	}
	outbound := platform.OutboundMessage{
		SessionKey:   msg.SessionKey,
		Content:      content,
		InboundMsgID: msg.ID,
		ReplyTo: platform.InboundMessage{
			Platform:   msg.Platform,
			SessionKey: msg.SessionKey,
		},
		SourceType: store.SourceTypeSystem,
	}
	if err := sender.Send(ctx, outbound); err != nil {
		ciLog.Warn("stop command: send reply", zap.Error(err))
	}
}
