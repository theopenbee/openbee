package bee

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robobee/core/internal/claudemd"
	"github.com/robobee/core/internal/config"
	"github.com/robobee/core/internal/store"
)

// BeeRunner abstracts the bee process invocation (real or test double).
type BeeRunner interface {
	Run(ctx context.Context, workDir, prompt, sessionID string, resume bool) error
}

// Feeder polls platform_messages for unprocessed messages and feeds them to bee.
type Feeder struct {
	msgStore     *store.MessageStore
	taskStore    *store.TaskStore
	sessionStore *store.SessionStore
	runner       BeeRunner
	workDir      string
	cfg          config.BeeConfig
}

// NewFeeder creates a Feeder.
func NewFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, runner BeeRunner, workDir string, cfg config.BeeConfig) *Feeder {
	return &Feeder{
		msgStore:     ms,
		taskStore:    ts,
		sessionStore: ss,
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
		slog.Error("recover feeding", "component", "feeder", "error", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		slog.Error("delete orphaned tasks", "component", "feeder", "error", err)
	}
	slog.Info("recovered feeding messages", "component", "feeder", "count", len(ids))
}

// Run polls for unprocessed messages on each tick. Call in a goroutine.
func (f *Feeder) Run(ctx context.Context) {
	ticker := time.NewTicker(f.cfg.Feeder.Interval)
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
	if count > f.cfg.Feeder.QueueWarnThreshold {
		slog.Warn("unprocessed messages in queue", "component", "feeder", "count", count, "threshold", f.cfg.Feeder.QueueWarnThreshold)
	}

	msgs, err := f.msgStore.ClaimBatch(ctx, f.cfg.Feeder.BatchSize)
	if err != nil {
		slog.Error("claim batch", "component", "feeder", "error", err)
		return
	}
	if len(msgs) == 0 {
		return
	}

	if err := WriteCLAUDEMD(f.workDir, DefaultPersona); err != nil {
		slog.Error("write CLAUDE.md", "component", "feeder", "error", err)
		f.rollback(ctx, msgs)
		return
	}
	if err := claudemd.EnsureSystemRules(f.workDir, claudemd.RoleBee); err != nil {
		slog.Error("ensure system rules", "component", "feeder", "error", err)
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
		slog.Error("get session context", "component", "feeder", "sessionKey", sessionKey, "error", err)
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

	if err := f.runner.Run(beeCtx, f.workDir, prompt, sessionID, resume); err != nil {
		slog.Error("bee run failed", "component", "feeder", "sessionKey", sessionKey, "error", err)
		f.rollback(ctx, msgs)
		return
	}

	// Persist session_id before marking messages processed
	if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID); err != nil {
		slog.Error("upsert session context", "component", "feeder", "sessionKey", sessionKey, "error", err)
		// non-fatal: messages are marked processed, but the session ID is not persisted.
		// On the next tick, GetSessionContext returns "" and bee starts a new session,
		// losing conversational continuity silently.
	}

	msgIDs := make([]string, len(msgs))
	for i, m := range msgs {
		msgIDs[i] = m.ID
	}
	if err := f.msgStore.MarkBeeProcessed(ctx, msgIDs); err != nil {
		slog.Error("mark bee_processed", "component", "feeder", "sessionKey", sessionKey, "error", err)
	}
}

func (f *Feeder) rollback(ctx context.Context, msgs []store.ClaimedMessage) {
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		slog.Error("rollback delete tasks", "component", "feeder", "error", err)
	}
	if err := f.msgStore.ResetFeedingBatch(ctx, ids); err != nil {
		slog.Error("rollback messages", "component", "feeder", "error", err)
	}
}

func buildPrompt(msgs []store.ClaimedMessage) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "以下是 %d 条待处理用户消息，请为每条消息创建相应的任务。\n\n", len(msgs))
	for i, m := range msgs {
		fmt.Fprintf(&sb, "--- 消息 %d ---\n来源: %s | 会话: %s | 消息ID: %s\n内容: %s\n\n",
			i+1, m.Platform, m.SessionKey, m.ID, m.Content)
	}
	sb.WriteString("请使用 create_task 工具为每条消息中的每个任务指派创建任务记录。")
	return sb.String()
}
