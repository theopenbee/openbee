package bee

import (
	"context"
	"fmt"
	"log"
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
		log.Printf("feeder: recover feeding: %v", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		log.Printf("feeder: delete orphaned tasks: %v", err)
	}
	log.Printf("feeder: recovered %d feeding message(s)", len(ids))
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
		log.Printf("feeder: WARNING: %d unprocessed messages in queue (threshold: %d)", count, f.cfg.Feeder.QueueWarnThreshold)
	}

	msgs, err := f.msgStore.ClaimBatch(ctx, f.cfg.Feeder.BatchSize)
	if err != nil {
		log.Printf("feeder: claim batch: %v", err)
		return
	}
	if len(msgs) == 0 {
		return
	}

	if err := WriteCLAUDEMD(f.workDir, DefaultPersona); err != nil {
		log.Printf("feeder: write CLAUDE.md: %v", err)
		f.rollback(ctx, msgs)
		return
	}
	if err := claudemd.EnsureSystemRules(f.workDir, claudemd.RoleBee); err != nil {
		log.Printf("feeder: ensure system rules: %v", err)
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
		log.Printf("feeder: get session context for %s: %v", sessionKey, err)
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
		log.Printf("feeder: bee run failed for %s: %v", sessionKey, err)
		f.rollback(ctx, msgs)
		return
	}

	// Persist session_id before marking messages processed
	if err := f.sessionStore.UpsertSessionContext(ctx, sessionKey, store.BeeAgentID, sessionID); err != nil {
		log.Printf("feeder: upsert session context for %s: %v", sessionKey, err)
		// non-fatal: messages are marked processed, but the session ID is not persisted.
		// On the next tick, GetSessionContext returns "" and bee starts a new session,
		// losing conversational continuity silently.
	}

	msgIDs := make([]string, len(msgs))
	for i, m := range msgs {
		msgIDs[i] = m.ID
	}
	if err := f.msgStore.MarkBeeProcessed(ctx, msgIDs); err != nil {
		log.Printf("feeder: mark bee_processed for %s: %v", sessionKey, err)
	}
}

func (f *Feeder) rollback(ctx context.Context, msgs []store.ClaimedMessage) {
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	if err := f.taskStore.DeletePendingByMessageIDs(ctx, ids); err != nil {
		log.Printf("feeder: rollback delete tasks: %v", err)
	}
	if err := f.msgStore.ResetFeedingBatch(ctx, ids); err != nil {
		log.Printf("feeder: rollback messages: %v", err)
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
