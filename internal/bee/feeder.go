package bee

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/claude"
	"github.com/theopenbee/openbee/internal/claudemd"
	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
	"go.uber.org/zap"
)

var log = logger.With(zap.String("component", "feeder"))

// BeeRunner abstracts the bee process invocation (real or test double).
type BeeRunner interface {
	Run(ctx context.Context, workDir, prompt, sessionID string, resume bool) (*claude.Process, <-chan claude.Output, error)
}

// Feeder polls platform_messages for unprocessed messages and feeds them to bee.
type Feeder struct {
	msgStore     *store.MessageStore
	taskStore    *store.TaskStore
	sessionStore *store.SessionStore
	execStore    *store.ExecutionStore
	runner       BeeRunner
	workDir      string
	cfg          config.BeeConfig
}

// NewFeeder creates a Feeder.
func NewFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner BeeRunner, workDir string, cfg config.BeeConfig) *Feeder {
	return &Feeder{
		msgStore:     ms,
		taskStore:    ts,
		sessionStore: ss,
		execStore:    es,
		runner:       runner,
		workDir:      workDir,
		cfg:          cfg,
	}
}

// RecoverFeeding resets any messages stuck in 'feeding' status back to 'received'
// and deletes their associated pending tasks.
// Must be called synchronously at startup BEFORE TaskScheduler.RecoverRunning.
func (f *Feeder) RecoverFeeding(ctx context.Context) {
	ids, err := f.msgStore.ResetFeedingToReceived(ctx)
	if err != nil {
		log.Error("recover feeding", zap.Error(err))
		return
	}
	if len(ids) == 0 {
		return
	}
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		log.Error("delete orphaned tasks", zap.Error(err))
	}
	log.Info("recovered feeding messages", zap.Int("count", len(ids)))
}

// Run polls for unprocessed messages on each tick. Call in a goroutine.
func (f *Feeder) Run(ctx context.Context) {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			f.tick(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (f *Feeder) tick(ctx context.Context) {
	count, _ := f.msgStore.CountReceived(ctx)
	if count > QueueWarnThreshold {
		log.Warn("unprocessed messages in queue", zap.Int("count", count), zap.Int("threshold", QueueWarnThreshold))
	}

	msgs, err := f.msgStore.ClaimBatch(ctx, 1)
	if err != nil {
		log.Error("claim batch", zap.Error(err))
		return
	}
	if len(msgs) == 0 {
		return
	}

	if err := WriteCLAUDEMD(f.workDir, DefaultPersona); err != nil {
		log.Error("write CLAUDE.md", zap.Error(err))
		f.rollback(ctx, msgs)
		return
	}
	if err := claudemd.EnsureSystemRules(f.workDir, claudemd.RoleBee); err != nil {
		log.Error("ensure system rules", zap.Error(err))
		// non-fatal: continue even if system rules update fails
	}

	groups := make(map[string][]store.ClaimedMessage)
	for _, m := range msgs {
		groups[m.SessionKey] = append(groups[m.SessionKey], m)
	}

	var wg sync.WaitGroup
	for sessionKey, group := range groups {
		wg.Add(1)
		go func(sessionKey string, group []store.ClaimedMessage) {
			defer wg.Done()
			f.processBeeGroup(ctx, sessionKey, group)
		}(sessionKey, group)
	}
	wg.Wait()
}

// processBeeGroup invokes bee for a single sessionKey's messages, managing session continuity.
func (f *Feeder) processBeeGroup(ctx context.Context, sessionKey string, msgs []store.ClaimedMessage) {
	// Look up existing session for this sessionKey
	sessionID, err := f.sessionStore.GetSessionContext(ctx, sessionKey, store.BeeAgentID)
	if err != nil {
		log.Error("get session context", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.rollback(ctx, msgs)
		return
	}
	resume := sessionID != ""
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	prompt := buildPrompt(msgs)
	beeCtx, cancel := context.WithTimeout(ctx, f.cfg.Feeder.Timeout)
	defer cancel()

	proc, outputCh, err := f.runner.Run(beeCtx, f.workDir, prompt, sessionID, resume)
	if err != nil {
		log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.rollback(ctx, msgs)
		return
	}

	// Create execution record only after process starts successfully.
	exec, execErr := f.execStore.CreateBeeExecution(sessionID, prompt)
	if execErr != nil {
		log.Error("create bee execution", zap.String("sessionKey", sessionKey), zap.Error(execErr))
		// non-fatal: continue without execution tracking
	}
	if execErr == nil && proc != nil {
		if pidErr := f.execStore.UpdatePID(exec.ID, proc.PID()); pidErr != nil {
			log.Error("update execution pid", zap.Error(pidErr))
		}
	}

	logs, drainErr := f.drainBeeOutput(outputCh)

	if execErr == nil {
		if logsErr := f.execStore.UpdateLogs(exec.ID, logs); logsErr != nil {
			log.Error("update execution logs", zap.Error(logsErr))
		}
		finalStatus := model.ExecStatusCompleted
		resultMsg := ""
		if drainErr != nil {
			finalStatus = model.ExecStatusFailed
			resultMsg = drainErr.Error()
		}
		if resErr := f.execStore.UpdateResult(exec.ID, resultMsg, finalStatus); resErr != nil {
			log.Error("update execution result", zap.Error(resErr))
		}
	}

	if drainErr != nil {
		log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(drainErr))
		f.rollback(ctx, msgs)
		return
	}

	// Persist session_id before marking messages processed.
	if resume {
		currentID, checkErr := f.sessionStore.GetSessionContext(ctx, sessionKey, store.BeeAgentID)
		if checkErr == nil && currentID == "" {
			log.Info("session cleared during bee execution, skipping context upsert",
				zap.String("sessionKey", sessionKey))
		} else {
			if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID); err != nil {
				log.Error("upsert session context", zap.String("sessionKey", sessionKey), zap.Error(err))
			}
		}
	} else {
		if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID); err != nil {
			log.Error("upsert session context", zap.String("sessionKey", sessionKey), zap.Error(err))
		}
	}

	msgIDs := make([]string, len(msgs))
	for i, m := range msgs {
		msgIDs[i] = m.ID
	}
	if err := f.msgStore.MarkBeeProcessed(ctx, msgIDs); err != nil {
		log.Error("mark bee_processed", zap.String("sessionKey", sessionKey), zap.Error(err))
	}
}

func (f *Feeder) rollback(ctx context.Context, msgs []store.ClaimedMessage) {
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		log.Error("rollback delete tasks", zap.Error(err))
	}
	if err := f.msgStore.ResetFeedingBatch(ctx, ids); err != nil {
		log.Error("rollback messages", zap.Error(err))
	}
}

// drainBeeOutput consumes the output channel and accumulates logs in memory.
// Returns accumulated log string (partial even on error) and nil on OutputDone,
// or non-nil error on OutputError or channel closed without completion.
func (f *Feeder) drainBeeOutput(ch <-chan claude.Output) (string, error) {
	var sb strings.Builder
	var done bool
	for out := range ch {
		switch out.Type {
		case claude.OutputStdout:
			sb.WriteString(out.Content)
			sb.WriteByte('\n')
		case claude.OutputStderr:
			sb.WriteString(out.Content)
			sb.WriteByte('\n')
		case claude.OutputError:
			sb.WriteString(out.Content)
			sb.WriteByte('\n')
			return sb.String(), fmt.Errorf("bee exited with error: %s", out.Content)
		case claude.OutputDone:
			done = true
		}
	}
	if !done {
		return sb.String(), fmt.Errorf("bee output channel closed without completion signal")
	}
	return sb.String(), nil
}

func buildPrompt(msgs []store.ClaimedMessage) string {
	var sb strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&sb, "消息来源: %s | 会话: %s | 消息ID: %s\n内容: %s\n",
			m.Platform, m.SessionKey, m.ID, m.Content)
	}
	return sb.String()
}
