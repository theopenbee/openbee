package bee

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// executionStopper can kill a running process by execution ID.
type executionStopper interface {
	StopExecution(executionID string) error
}

// sessionClearer clears dispatcher queues and session contexts for a session.
type sessionClearer interface {
	ClearSession(sessionKey string)
}

// CommandInterceptor intercepts system slash commands before Feeder dispatches to Bee.
type CommandInterceptor struct {
	sessionStore *store.SessionStore
	execStore    *store.ExecutionStore
	taskStore    *store.TaskStore
	execStopper  executionStopper
	dispatcher   sessionClearer
	senders      map[string]platform.PlatformSenderAdapter
	engine       string
}

func NewCommandInterceptor(
	ss *store.SessionStore,
	es *store.ExecutionStore,
	ts *store.TaskStore,
	stopper executionStopper,
	clearer sessionClearer,
	senders map[string]platform.PlatformSenderAdapter,
	engine string,
) *CommandInterceptor {
	return &CommandInterceptor{
		sessionStore: ss,
		execStore:    es,
		taskStore:    ts,
		execStopper:  stopper,
		dispatcher:   clearer,
		senders:      senders,
		engine:       engine,
	}
}

// Intercept returns true if the message was handled and Feeder should skip normal dispatch.
func (c *CommandInterceptor) Intercept(ctx context.Context, sessionKey string, msgs []store.ClaimedMessage) (bool, error) {
	if len(msgs) == 0 {
		return false, nil
	}
	primary := msgs[len(msgs)-1]
	if !strings.EqualFold(strings.TrimSpace(primary.Content), "/stop") {
		return false, nil
	}
	return true, c.handleStop(ctx, sessionKey, primary)
}

// handleStop performs the stop sequence: stop bee executions, cancel worker tasks,
// clear dispatcher queues, and reply to the user. Sub-step errors are logged as
// warnings and do not abort the sequence; the function always returns nil.
func (c *CommandInterceptor) handleStop(ctx context.Context, sessionKey string, msg store.ClaimedMessage) error {
	stopped := false

	sessionID, err := c.sessionStore.GetSessionContextForEngine(ctx, sessionKey, store.BeeAgentID, c.engine)
	if err != nil {
		log.Warn("stop command: get session context", zap.String("sessionKey", sessionKey), zap.Error(err))
	} else if sessionID != "" {
		execs, listErr := c.execStore.ListActiveBySessionID(sessionID)
		if listErr != nil {
			log.Warn("stop command: list executions", zap.String("sessionID", sessionID), zap.Error(listErr))
		}
		for _, e := range execs {
			if stopErr := c.execStopper.StopExecution(e.ID); stopErr != nil {
				log.Warn("stop command: stop execution", zap.String("execID", e.ID), zap.Error(stopErr))
			} else {
				stopped = true
			}
		}
	}

	n, err := c.taskStore.CancelBySessionKey(ctx, sessionKey)
	if err != nil {
		log.Warn("stop command: cancel tasks", zap.String("sessionKey", sessionKey), zap.Error(err))
	} else if n > 0 {
		stopped = true
	}

	c.dispatcher.ClearSession(sessionKey)

	const (
		msgStopped    = "已停止当前会话的所有任务"
		msgNothingRan = "当前会话没有正在运行的任务"
	)
	replyContent := msgStopped
	if !stopped {
		replyContent = msgNothingRan
	}
	c.sendReply(ctx, msg, replyContent)
	return nil
}

func (c *CommandInterceptor) sendReply(ctx context.Context, msg store.ClaimedMessage, content string) {
	sender, ok := c.senders[msg.Platform]
	if !ok {
		log.Warn("stop command: no sender for platform", zap.String("platform", msg.Platform))
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
		log.Warn("stop command: send reply", zap.Error(err))
	}
}
