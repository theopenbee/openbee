package bee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"go.uber.org/zap"
)

var log = logger.With(zap.String("component", "feeder"))

type messageMeta struct {
	From       string `json:"from"`
	SessionKey string `json:"session_key"`
	MessageID  string `json:"message_id"`
}

// FailureNotifier sends a notification to the user when a message is permanently failed.
type FailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
}

// Option configures a Feeder.
type Option func(*Feeder)

// WithFailureNotifier sets the notifier used to inform users when a message fails.
func WithFailureNotifier(n FailureNotifier) Option {
	return func(f *Feeder) { f.failureNotifier = n }
}

// WithWorkerDispatch enables direct dispatch via "@workerName" or " workerName" prefix.
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
	engineCfg       *enginecfg.Store
	failureNotifier FailureNotifier
	sem             chan struct{} // bounds concurrent bee processes
	workerLookup    *store.WorkerStore
	runningMu       sync.Mutex
	running         map[string]context.CancelFunc
}

// NewFeeder creates a Feeder.
func NewFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner ai.EngineAdapter, workDir string, cfg config.BeeConfig, engineCfg *enginecfg.Store, opts ...Option) *Feeder {
	f := &Feeder{
		msgStore:     ms,
		taskStore:    ts,
		sessionStore: ss,
		execStore:    es,
		runner:       runner,
		workDir:      workDir,
		cfg:          cfg,
		engineCfg:    engineCfg,
		sem:          make(chan struct{}, cfg.Feeder.MaxConcurrentBee),
		running: make(map[string]context.CancelFunc),
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

func (f *Feeder) StopSession(sessionKey string) bool {
	f.runningMu.Lock()
	cancel, ok := f.running[sessionKey]
	f.runningMu.Unlock()
	if ok {
		cancel()
	}
	return ok
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
	if err := os.MkdirAll(f.workDir, 0o755); err != nil {
		log.Error("create bee workspace", zap.Error(err))
	}
	if err := f.runner.Prepare(f.workDir, ai.PrepareOptions{Role: ai.RoleBee}); err != nil {
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

	// Snapshot the engine once so all session-context ops key off the same engine,
	// even if /engine fires mid-execution.
	engineName := f.engineCfg.Get()

	sessionID, err := f.sessionStore.GetSessionContextForEngine(ctx, sessionKey, store.BeeAgentID, engineName)
	if err != nil {
		log.Error("get session context", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.failMessages(ctx, msgs, err.Error())
		return
	}
	resume := sessionID != ""
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// Pre-flight: ensures the session ID is visible before the process starts.
	if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID, engineName); err != nil {
		log.Error("pre-flight upsert bee session context", zap.String("sessionKey", sessionKey), zap.Error(err))
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

	systemPrompt := ""
	if !resume {
		systemPrompt = ai.BuildSystemPrompt(ai.RoleBee, nil)
	}
	prompt := buildPrompt(msgs)

	// Create execution record first — we need exec.ID before launching the process
	// so we can prepare the log path (which is based on the ID).
	exec, err := f.execStore.CreateBeeExecution(sessionID, prompt, engineName)
	if err != nil {
		log.Error("create bee execution", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.failMessages(ctx, msgs, err.Error())
		return
	}

	logPath, err := f.execStore.PrepareLogPath(exec.ID, exec.StartedAt)
	if err != nil {
		log.Error("prepare log path", zap.String("execID", exec.ID), zap.Error(err))
		f.execStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		f.failMessages(ctx, msgs, err.Error())
		return
	}

	beeCtx, cancel := context.WithTimeout(ctx, f.cfg.Engine.Timeout.Bee)
	f.runningMu.Lock()
	f.running[sessionKey] = cancel
	f.runningMu.Unlock()
	defer func() {
		cancel()
		f.runningMu.Lock()
		delete(f.running, sessionKey)
		f.runningMu.Unlock()
	}()

	runRes, err := f.runner.Run(beeCtx, f.workDir, prompt, ai.RunOptions{
		SessionID:    sessionID,
		Resume:       resume,
		SystemPrompt: systemPrompt,
	}, logPath)
	if err != nil {
		log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(err))
		f.execStore.UpdateResult(exec.ID, err.Error(), model.ExecStatusFailed)
		// Rollback the pre-flight record; process never started so no session was established.
		if !resume {
			if _, delErr := f.sessionStore.DeleteSessionContextForEngine(ctx, sessionKey, store.BeeAgentID, engineName); delErr != nil {
				log.Error("rollback pre-flight session context", zap.String("sessionKey", sessionKey), zap.Error(delErr))
			}
		}
		f.failMessages(ctx, msgs, err.Error())
		return
	}

	if pidErr := f.execStore.UpdatePID(exec.ID, runRes.Process.PID()); pidErr != nil {
		log.Error("update execution pid", zap.Error(pidErr))
	}

	drainErr := f.waitBeeOutput(runRes.Output)

	finalStatus := model.ExecStatusCompleted
	resultMsg := runRes.ExtractResult(logPath)
	if drainErr != nil {
		finalStatus = model.ExecStatusFailed
		if resultMsg == "" {
			resultMsg = drainErr.Error()
		}
	}
	if resErr := f.execStore.UpdateResult(exec.ID, resultMsg, finalStatus); resErr != nil {
		log.Error("update execution result", zap.Error(resErr))
	}

	if drainErr != nil {
		log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(drainErr))
		f.failMessages(ctx, msgs, resultMsg)
		return
	}

	// On resume, skip if the session was cleared mid-execution (concurrent clear wins).
	upsert := true
	if resume {
		currentID, checkErr := f.sessionStore.GetSessionContextForEngine(ctx, sessionKey, store.BeeAgentID, engineName)
		if checkErr == nil && currentID == "" {
			log.Info("session cleared during bee execution, skipping context upsert",
				zap.String("sessionKey", sessionKey))
			upsert = false
		}
	}
	if upsert {
		if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID, engineName); err != nil {
			log.Error("upsert session context", zap.String("sessionKey", sessionKey), zap.Error(err))
		}
	}

	if err := f.msgStore.MarkBeeProcessed(ctx, messageIDs(msgs)); err != nil {
		log.Error("mark bee_processed", zap.String("sessionKey", sessionKey), zap.Error(err))
	}
}

func (f *Feeder) failMessages(ctx context.Context, msgs []store.ClaimedMessage, reason string) {
	ids := messageIDs(msgs)
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		log.Error("fail messages delete tasks", zap.Error(err))
	}
	if err := f.msgStore.MarkFailed(ctx, ids); err != nil {
		log.Error("mark messages failed", zap.Error(err))
		return
	}
	if f.failureNotifier == nil {
		return
	}
	for _, m := range msgs {
		log.Warn("message failed", zap.String("messageID", m.ID), zap.String("reason", reason))
		if notifyErr := f.failureNotifier.NotifyTaskFailure(ctx, m.ID, model.FailureInfo{Reason: reason, WorkerName: "bee"}); notifyErr != nil {
			log.Error("notify bee failure", zap.String("messageID", m.ID), zap.Error(notifyErr))
		}
	}
}

// waitBeeOutput consumes the output channel and waits for a lifecycle signal.
// Returns nil on OutputDone, or an error on OutputError or channel close without signal.
func (f *Feeder) waitBeeOutput(ch <-chan ai.Output) error {
	for out := range ch {
		switch out.Type {
		case ai.OutputDone:
			return nil
		case ai.OutputError:
			return errors.New(out.Content)
		}
	}
	return fmt.Errorf("output channel closed without completion signal")
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
	sb.Grow(len(msgs) * 128)
	for i, m := range msgs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		meta := messageMeta{
			From:       m.Platform,
			SessionKey: m.SessionKey,
			MessageID:  m.ID,
		}
		b, _ := json.Marshal(meta)
		fmt.Fprintf(&sb, "<message_meta>%s</message_meta>\n<message_content>\n%s\n</message_content>\n", b, m.Content)
	}
	return sb.String()
}

func parseDirectMention(content string) (workerName, instruction string, ok bool) {
	if len(content) == 0 {
		return "", "", false
	}
	rest := content
	if content[0] == '@' {
		rest = content[1:]
	}
	idx := strings.IndexAny(rest, " \n")
	if idx <= 0 {
		return "", "", false
	}
	workerName = rest[:idx]
	instruction = strings.TrimSpace(rest[idx+1:])
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
		log.Error("direct: create task record", zap.Error(err))
		return false
	}

	log.Info("direct: dispatched task to worker via scheduler",
		zap.String("name", workerName), zap.String("workerID", worker.ID))

	if err := f.msgStore.MarkBeeProcessed(ctx, messageIDs(msgs)); err != nil {
		log.Error("direct: mark bee_processed", zap.Error(err))
	}
	return true
}
