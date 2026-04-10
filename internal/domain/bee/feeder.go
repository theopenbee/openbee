package bee

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"go.uber.org/zap"
)

var log = logger.With(zap.String("component", "feeder"))

// FailureNotifier sends a notification to the user when a message is permanently failed.
type FailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
}

// Option configures a Feeder.
type Option func(*Feeder)

// WithFailureNotifier sets the notifier used to inform users when a message exhausts retries.
func WithFailureNotifier(n FailureNotifier) Option {
	return func(f *Feeder) { f.failureNotifier = n }
}

// WithWorkerDispatch enables @mention direct dispatch by providing the worker lookup store.
func WithWorkerDispatch(lookup *store.WorkerStore) Option {
	return func(f *Feeder) {
		f.workerLookup = lookup
	}
}

// Feeder polls platform_messages for unprocessed messages and feeds them to bee.
type Feeder struct {
	msgStore        *store.MessageStore
	taskStore       *store.TaskStore
	sessionStore    *store.SessionStore
	execStore       *store.ExecutionStore
	runner          ai.EngineAdapter
	workDir         string
	cfg             config.BeeConfig
	failureNotifier FailureNotifier
	sem             chan struct{} // bounds concurrent bee processes
	workerLookup    *store.WorkerStore
}

// NewFeeder creates a Feeder.
func NewFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner ai.EngineAdapter, workDir string, cfg config.BeeConfig, opts ...Option) *Feeder {
	f := &Feeder{
		msgStore:     ms,
		taskStore:    ts,
		sessionStore: ss,
		execStore:    es,
		runner:       runner,
		workDir:      workDir,
		cfg:          cfg,
		sem:          make(chan struct{}, cfg.Feeder.MaxConcurrentBee),
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

// RecoverFeeding resets any messages stuck in 'feeding' status back to 'received'
// and deletes their associated pending tasks.
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
	if err := f.runner.SetupWorkspace(f.workDir, ai.RoleBee, ai.WorkspaceOptions{}); err != nil {
		log.Error("setup bee workspace", zap.Error(err))
	}
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

	// Only claim as many messages as there are available semaphore slots,
	// so every claimed message can be dispatched immediately without blocking.
	available := cap(f.sem) - len(f.sem)
	if available == 0 {
		return
	}

	msgs, err := f.msgStore.ClaimBatch(ctx, available)
	if err != nil {
		log.Error("claim batch", zap.Error(err))
		return
	}
	if len(msgs) == 0 {
		return
	}

	for _, m := range msgs {
		f.sem <- struct{}{} // always succeeds: len(msgs) <= available slots
		go func() {
			defer func() { <-f.sem }()
			f.processBeeGroup(ctx, m.SessionKey, []store.ClaimedMessage{m})
		}()
	}
}

// processBeeGroup invokes bee for a single sessionKey's messages, managing session continuity.
func (f *Feeder) processBeeGroup(ctx context.Context, sessionKey string, msgs []store.ClaimedMessage) {
	if f.tryDirectDispatch(ctx, msgs) {
		return
	}

	sessionID, storedEngine, err := f.sessionStore.GetSessionContext(ctx, sessionKey, store.BeeAgentID)
	if err != nil {
		log.Error("get session context", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.rollback(ctx, msgs, err.Error())
		return
	}
	// Discard session if it belongs to a different engine. Empty storedEngine means
	// legacy data with no engine recorded — skip the check to preserve existing sessions.
	currentEngine := f.cfg.EffectiveEngine()
	if sessionID != "" && storedEngine != "" && storedEngine != currentEngine {
		log.Info("engine changed, discarding stale bee session",
			zap.String("stored", storedEngine), zap.String("current", currentEngine))
		sessionID = ""
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

	// Create execution record first — we need exec.ID before launching the process
	// so we can prepare the log path (which is based on the ID).
	exec, err := f.execStore.CreateBeeExecution(sessionID, prompt)
	if err != nil {
		log.Error("create bee execution", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.rollback(ctx, msgs, err.Error())
		return
	}

	logPath, err := f.execStore.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		log.Error("prepare log path", zap.String("execID", exec.ID), zap.Error(err))
		f.execStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		f.rollback(ctx, msgs, err.Error())
		return
	}

	beeCtx, cancel := context.WithTimeout(ctx, f.cfg.Feeder.Timeout)
	defer cancel()

	proc, outputCh, err := f.runner.Run(beeCtx, f.workDir, prompt, ai.RunOptions{SessionID: sessionID, Resume: resume}, logPath)
	if err != nil {
		log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.execStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		f.rollback(ctx, msgs, err.Error())
		return
	}

	if pidErr := f.execStore.UpdatePID(exec.ID, proc.PID()); pidErr != nil {
		log.Error("update execution pid", zap.Error(pidErr))
	}

	engineSessionID, drainErr := f.waitBeeOutput(outputCh)
	if engineSessionID != "" {
		sessionID = engineSessionID
	}

	finalStatus := model.ExecStatusCompleted
	resultMsg := ""
	if drainErr != nil {
		finalStatus = model.ExecStatusFailed
		resultMsg = f.runner.ExtractResult(logPath)
		if resultMsg == "" {
			resultMsg = drainErr.Error()
		}
	}
	if resErr := f.execStore.UpdateResult(exec.ID, resultMsg, finalStatus); resErr != nil {
		log.Error("update execution result", zap.Error(resErr))
	}

	if drainErr != nil {
		log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(drainErr))
		f.rollback(ctx, msgs, drainErr.Error())
		return
	}

	// Persist session_id before marking messages processed.
	// On resume, skip if the session was cleared mid-execution (concurrent clear wins).
	upsert := true
	if resume {
		currentID, _, checkErr := f.sessionStore.GetSessionContext(ctx, sessionKey, store.BeeAgentID)
		if checkErr == nil && currentID == "" {
			log.Info("session cleared during bee execution, skipping context upsert",
				zap.String("sessionKey", sessionKey))
			upsert = false
		}
	}
	if upsert {
		if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID, currentEngine); err != nil {
			log.Error("upsert session context", zap.String("sessionKey", sessionKey), zap.Error(err))
		}
	}

	if err := f.msgStore.MarkBeeProcessed(ctx, messageIDs(msgs)); err != nil {
		log.Error("mark bee_processed", zap.String("sessionKey", sessionKey), zap.Error(err))
	}
}

func (f *Feeder) rollback(ctx context.Context, msgs []store.ClaimedMessage, reason string) {
	ids := messageIDs(msgs)
	var failedMsgs []store.ClaimedMessage
	for _, m := range msgs {
		if m.RetryCount+1 >= MaxRetries {
			failedMsgs = append(failedMsgs, m)
		}
	}
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		log.Error("rollback delete tasks", zap.Error(err))
	}
	if err := f.msgStore.RollbackWithRetry(ctx, ids, MaxRetries); err != nil {
		log.Error("rollback with retry", zap.Error(err))
		return
	}
	for _, m := range failedMsgs {
		log.Warn("message exhausted retries", zap.String("messageID", m.ID))
		if f.failureNotifier != nil {
			info := model.FailureInfo{
				Reason:     reason,
				RetryCount: m.RetryCount + 1,
				MaxRetries: MaxRetries,
			}
			if notifyErr := f.failureNotifier.NotifyTaskFailure(ctx, m.ID, info); notifyErr != nil {
				log.Error("notify bee failure", zap.String("messageID", m.ID), zap.Error(notifyErr))
			}
		}
	}
}

// waitBeeOutput consumes the output channel and waits for a lifecycle signal.
// Returns the engine-assigned session ID (if any), nil error on OutputDone, non-nil on OutputError.
func (f *Feeder) waitBeeOutput(ch <-chan ai.Output) (string, error) {
	var engineSessionID string
	for out := range ch {
		switch out.Type {
		case ai.OutputDone:
			return engineSessionID, nil
		case ai.OutputError:
			return engineSessionID, errors.New(out.Content)
		case ai.OutputSessionID:
			engineSessionID = out.Content
		}
	}
	return engineSessionID, nil
}

func messageIDs(msgs []store.ClaimedMessage) []string {
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	return ids
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

func parseDirectMention(content string) (workerName, instruction string, ok bool) {
	rest, found := strings.CutPrefix(content, "@")
	if !found {
		return "", "", false
	}
	workerName, instruction, found = strings.Cut(rest, " ")
	if !found || workerName == "" {
		return "", "", false
	}
	instruction = strings.TrimSpace(instruction)
	return workerName, instruction, instruction != ""
}

func (f *Feeder) tryDirectDispatch(ctx context.Context, msgs []store.ClaimedMessage) bool {
	if f.workerLookup == nil {
		return false
	}
	if len(msgs) == 0 {
		return false
	}

	primary := msgs[len(msgs)-1]
	workerName, instruction, ok := parseDirectMention(primary.Content)
	if !ok {
		return false
	}

	worker, err := f.workerLookup.GetByName(workerName)
	if err != nil {
		log.Warn("@mention: worker not found, falling back to bee",
			zap.String("name", workerName))
		return false
	}

	_, err = f.taskStore.Create(ctx, model.Task{
		MessageID:   primary.ID,
		WorkerID:    worker.ID,
		Instruction: instruction,
		Type:        model.TaskTypeImmediate,
		Status:      model.TaskStatusPending,
	})
	if err != nil {
		log.Error("@mention: create task record", zap.Error(err))
		return false
	}

	log.Info("@mention: dispatched task to worker via scheduler",
		zap.String("name", workerName), zap.String("workerID", worker.ID))

	if err := f.msgStore.MarkBeeProcessed(ctx, messageIDs(msgs)); err != nil {
		log.Error("@mention: mark bee_processed", zap.Error(err))
	}
	return true
}
