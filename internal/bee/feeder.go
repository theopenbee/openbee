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
	"github.com/theopenbee/openbee/internal/worker"
	"go.uber.org/zap"
)

var log = logger.With(zap.String("component", "feeder"))

// BeeRunner abstracts the bee process invocation (real or test double).
type BeeRunner interface {
	Run(ctx context.Context, workDir, prompt, sessionID string, resume bool) (*claude.Process, <-chan claude.Output, error)
}

// FailureNotifier sends a notification to the user when a message is permanently failed.
// Mirrors task_dispatcher.FailureNotifier; kept local to avoid an import cycle.
type FailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID, reason string) error
}

// Option configures a Feeder.
type Option func(*Feeder)

// WithFailureNotifier sets the notifier used to inform users when a message exhausts retries.
func WithFailureNotifier(n FailureNotifier) Option {
	return func(f *Feeder) { f.failureNotifier = n }
}

// WithLogRegistry injects a shared log registry so bee executions provide live logs.
func WithLogRegistry(r *worker.ActiveLogRegistry) Option {
	return func(f *Feeder) { f.logRegistry = r }
}

// Feeder polls platform_messages for unprocessed messages and feeds them to bee.
type Feeder struct {
	msgStore        *store.MessageStore
	taskStore       *store.TaskStore
	sessionStore    *store.SessionStore
	execStore       *store.ExecutionStore
	runner          BeeRunner
	workDir         string
	cfg             config.BeeConfig
	failureNotifier FailureNotifier
	logRegistry     *worker.ActiveLogRegistry // nil if not configured
}

// NewFeeder creates a Feeder.
func NewFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner BeeRunner, workDir string, cfg config.BeeConfig, opts ...Option) *Feeder {
	f := &Feeder{
		msgStore:     ms,
		taskStore:    ts,
		sessionStore: ss,
		execStore:    es,
		runner:       runner,
		workDir:      workDir,
		cfg:          cfg,
	}
	for _, o := range opts {
		o(f)
	}
	return f
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
		f.rollback(ctx, msgs, "内部错误：无法写入配置文件")
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
		f.rollback(ctx, msgs, "内部错误：无法读取会话上下文")
		return
	}
	resume := sessionID != ""
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	for i, m := range msgs {
		merged, err := f.msgStore.FetchMergedContent(ctx, m.ID)
		if err != nil {
			log.Error("fetch merged content", zap.String("msgID", m.ID), zap.Error(err))
			continue
		}
		if len(merged) > 0 {
			msgs[i].Content = strings.Join(merged, "\n\n---\n\n") + "\n\n---\n\n" + m.Content
		}
	}

	prompt := buildPrompt(msgs)
	beeCtx, cancel := context.WithTimeout(ctx, f.cfg.Feeder.Timeout)
	defer cancel()

	proc, outputCh, err := f.runner.Run(beeCtx, f.workDir, prompt, sessionID, resume)
	if err != nil {
		log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.rollback(ctx, msgs, "AI 处理失败，请稍后重试")
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

	// Register with live log registry (only if registry is configured and execution was created).
	writeLine := func(string) {}
	if f.logRegistry != nil && execErr == nil {
		writeLine = f.logRegistry.Register(exec.ID)
	}

	logs, drainErr := f.drainBeeOutput(outputCh, writeLine)

	if execErr == nil {
		if _, logsErr := f.execStore.WriteLog(exec.ID, exec.StartedAt, logs); logsErr != nil {
			log.Error("write execution logs", zap.Error(logsErr))
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
		// Unregister after disk write.
		if f.logRegistry != nil {
			f.logRegistry.Unregister(exec.ID)
		}
	}

	if drainErr != nil {
		log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(drainErr))
		f.rollback(ctx, msgs, "AI 处理失败，请稍后重试")
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

func (f *Feeder) rollback(ctx context.Context, msgs []store.ClaimedMessage, userMsg string) {
	ids := make([]string, len(msgs))
	var failedIDs []string
	for i, m := range msgs {
		ids[i] = m.ID
		if m.RetryCount+1 >= MaxRetries {
			failedIDs = append(failedIDs, m.ID)
		}
	}
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		log.Error("rollback delete tasks", zap.Error(err))
	}
	if err := f.msgStore.RollbackWithRetry(ctx, ids, MaxRetries); err != nil {
		log.Error("rollback with retry", zap.Error(err))
		return
	}
	for _, id := range failedIDs {
		log.Warn("message exhausted retries", zap.String("messageID", id))
		if f.failureNotifier != nil {
			if notifyErr := f.failureNotifier.NotifyTaskFailure(ctx, id, userMsg); notifyErr != nil {
				log.Error("notify bee failure", zap.String("messageID", id), zap.Error(notifyErr))
			}
		}
	}
}

// drainBeeOutput consumes the output channel and accumulates logs in memory.
// Returns accumulated log string (partial even on error) and nil on OutputDone,
// or non-nil error on OutputError or channel closed without completion.
// writeLine is called for each output line; use a no-op func to disable live logging.
func (f *Feeder) drainBeeOutput(ch <-chan claude.Output, writeLine func(string)) (string, error) {
	var sb strings.Builder
	var done bool
	for out := range ch {
		switch out.Type {
		case claude.OutputStdout:
			sb.WriteString(out.Content)
			sb.WriteByte('\n')
			writeLine(out.Content)
		case claude.OutputStderr:
			sb.WriteString(out.Content)
			sb.WriteByte('\n')
			writeLine(out.Content)
		case claude.OutputError:
			sb.WriteString(out.Content)
			sb.WriteByte('\n')
			writeLine(out.Content)
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
	for i, m := range msgs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		fmt.Fprintf(&sb, "---\nfrom: %s\nsession_key: %s\nmessage_id: %s\n---\n\n%s\n",
			m.Platform, m.SessionKey, m.ID, m.Content)
	}
	return sb.String()
}
