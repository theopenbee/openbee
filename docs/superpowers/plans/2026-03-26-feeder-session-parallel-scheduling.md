# Feeder Session-Parallel Scheduling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Feeder 能够并发处理不同 session 的消息，同时保持同一 session 内严格串行。核心变更：修改 `ClaimBatch` SQL 实现 per-session 去重、在 `Feeder` 中引入 semaphore 控制并发上限、`tick()` 去掉 `wg.Wait()` 变为非阻塞。

**Architecture:** `FeederConfig` 新增 `MaxConcurrentBee` 配置（默认 5）。`ClaimBatch` 改为：每次取至多 N 条消息，每个 session_key 最多 1 条，且跳过已有 `feeding` 消息的 session。`tick()` 计算可用 semaphore 槽位数，按此数量 claim，为每条消息独立启动 goroutine，立即返回。

**Tech Stack:** Go stdlib (`sync`, channel-based semaphore), SQLite, 现有 `config`, `store`, `bee` 包。

---

## File Map

| 文件 | 操作 | 职责 |
|------|------|------|
| `internal/config/config.go` | Modify | `FeederConfig` 新增 `MaxConcurrentBee int`；`applyDefaults` 补充默认值 5 |
| `internal/store/message_store.go` | Modify | `ClaimBatch` SQL 改为 per-session 最早一条 + 跳过 feeding session |
| `internal/store/message_store_test.go` | Modify | 补充新 `ClaimBatch` 行为的单元测试 |
| `internal/bee/feeder.go` | Modify | `Feeder` 新增 `sem chan struct{}`；重写 `tick()`；删除 session 分组死代码 |
| `internal/bee/feeder_test.go` | Modify | 补充并行 session 测试；验证 semaphore 上限行为 |

---

## Task 1: `FeederConfig` 新增 `MaxConcurrentBee`

**文件：**
- Modify: `internal/config/config.go`

- [ ] **Step 1: 在 `FeederConfig` 中添加字段**

在 `config.go` 中修改 `FeederConfig`：

```go
type FeederConfig struct {
	Timeout          time.Duration `yaml:"timeout"`
	MaxConcurrentBee int           `yaml:"max_concurrent_bee"`
}
```

- [ ] **Step 2: 在 `applyDefaults` 中添加默认值**

在 `applyDefaults` 中 `Feeder.Timeout` 的赋值之后添加：

```go
if cfg.Bee.Feeder.MaxConcurrentBee == 0 {
    cfg.Bee.Feeder.MaxConcurrentBee = 5
}
```

- [ ] **Step 3: 验证**

运行 `go build ./internal/config/...`，确保编译通过。

---

## Task 2: 修改 `ClaimBatch` SQL

**文件：**
- Modify: `internal/store/message_store.go`
- Modify: `internal/store/message_store_test.go`

- [ ] **Step 1: 写失败测试**

在 `internal/store/message_store_test.go` 中新增以下测试（追加在现有 `ClaimBatch` 相关测试之后）：

```go
func TestMessageStore_ClaimBatch_SkipsFeedingSession(t *testing.T) {
    db, err := InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    s := NewMessageStore(db)
    ctx := context.Background()

    // Insert two messages for the same session; mark the first as feeding.
    now := time.Now().UnixMilli()
    db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m1', 'sk1', 'feishu', 'msg1', 'feeding', ?, ?, ?)`, now, now, now)
    db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m2', 'sk1', 'feishu', 'msg2', 'received', ?, ?, ?)`, now+1, now, now)

    msgs, err := s.ClaimBatch(ctx, 10)
    if err != nil {
        t.Fatalf("ClaimBatch: %v", err)
    }
    if len(msgs) != 0 {
        t.Errorf("expected 0 messages (session already feeding), got %d", len(msgs))
    }
}

func TestMessageStore_ClaimBatch_OnePerSession(t *testing.T) {
    db, err := InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    s := NewMessageStore(db)
    ctx := context.Background()

    now := time.Now().UnixMilli()
    // Two messages for sk1 (different times), one for sk2.
    db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m1', 'sk1', 'feishu', 'first', 'received', ?, ?, ?)`, now, now, now)
    db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m2', 'sk1', 'feishu', 'second', 'received', ?, ?, ?)`, now+1, now, now)
    db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
              VALUES ('m3', 'sk2', 'feishu', 'other', 'received', ?, ?, ?)`, now, now, now)

    msgs, err := s.ClaimBatch(ctx, 10)
    if err != nil {
        t.Fatalf("ClaimBatch: %v", err)
    }
    if len(msgs) != 2 {
        t.Fatalf("expected 2 messages (one per session), got %d", len(msgs))
    }
    ids := map[string]bool{}
    for _, m := range msgs {
        ids[m.ID] = true
    }
    if !ids["m1"] {
        t.Error("expected m1 (earliest for sk1) to be claimed, not m2")
    }
    if !ids["m3"] {
        t.Error("expected m3 (sk2) to be claimed")
    }
}

func TestMessageStore_ClaimBatch_RespectsLimit(t *testing.T) {
    db, err := InitDB(t.TempDir() + "/test.db")
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    s := NewMessageStore(db)
    ctx := context.Background()

    now := time.Now().UnixMilli()
    for i := 0; i < 5; i++ {
        sk := fmt.Sprintf("sk%d", i)
        id := fmt.Sprintf("m%d", i)
        db.Exec(`INSERT INTO bee_platform_messages (id, session_key, platform, content, status, received_at, created_at, updated_at)
                  VALUES (?, ?, 'feishu', 'msg', 'received', ?, ?, ?)`, id, sk, now+int64(i), now, now)
    }

    msgs, err := s.ClaimBatch(ctx, 3)
    if err != nil {
        t.Fatalf("ClaimBatch: %v", err)
    }
    if len(msgs) != 3 {
        t.Errorf("expected 3 messages (limit), got %d", len(msgs))
    }
}
```

运行测试，确认失败（新 SQL 尚未实现）：
```
go test ./internal/store/... -run TestMessageStore_ClaimBatch_SkipsFeedingSession
go test ./internal/store/... -run TestMessageStore_ClaimBatch_OnePerSession
go test ./internal/store/... -run TestMessageStore_ClaimBatch_RespectsLimit
```

- [ ] **Step 2: 实现新的 `ClaimBatch` SQL**

将 `internal/store/message_store.go` 中 `ClaimBatch` 的查询部分替换：

```go
// ClaimBatch atomically selects up to batchSize 'received' messages — at most one per
// session_key — skipping any session that already has a message in 'feeding' status.
// Within each session, the message with the earliest received_at is selected (FIFO).
func (s *MessageStore) ClaimBatch(ctx context.Context, batchSize int) ([]ClaimedMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx,
		`SELECT id, session_key, platform, content, retry_count
		 FROM bee_platform_messages m
		 WHERE status = 'received'
		   AND session_key NOT IN (
		       SELECT session_key FROM bee_platform_messages WHERE status = 'feeding'
		   )
		   AND received_at = (
		       SELECT MIN(received_at)
		       FROM bee_platform_messages m2
		       WHERE m2.session_key = m.session_key
		         AND m2.status = 'received'
		   )
		 ORDER BY received_at ASC
		 LIMIT ?`, batchSize)
	if err != nil {
		return nil, fmt.Errorf("select batch: %w", err)
	}
	var msgs []ClaimedMessage
	for rows.Next() {
		var m ClaimedMessage
		if err := rows.Scan(&m.ID, &m.SessionKey, &m.Platform, &m.Content, &m.RetryCount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan: %w", err)
		}
		msgs = append(msgs, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}

	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	args := make([]any, 0, len(ids)+2)
	args = append(args, "feeding", time.Now().UnixMilli())
	for _, id := range ids {
		args = append(args, id)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE bee_platform_messages SET status = ?, updated_at = ? WHERE id IN (`+inPlaceholders(len(ids))+`)`, args...); err != nil {
		return nil, fmt.Errorf("update feeding: %w", err)
	}
	return msgs, tx.Commit()
}
```

- [ ] **Step 3: 运行全部 store 测试**

```
go test ./internal/store/... -v
```

所有测试通过后继续。

---

## Task 3: 重写 `Feeder.tick()` — semaphore + 非阻塞

**文件：**
- Modify: `internal/bee/feeder.go`

- [ ] **Step 1: 在 `Feeder` struct 中添加 `sem` 字段**

```go
type Feeder struct {
	msgStore        *store.MessageStore
	taskStore       *store.TaskStore
	sessionStore    *store.SessionStore
	execStore       *store.ExecutionStore
	runner          BeeRunner
	workDir         string
	cfg             config.BeeConfig
	failureNotifier FailureNotifier
	sem             chan struct{} // bounds concurrent bee processes
}
```

- [ ] **Step 2: 在 `NewFeeder` 中初始化 semaphore**

在 `NewFeeder` 的 options loop 之前初始化：

```go
func NewFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner BeeRunner, workDir string, cfg config.BeeConfig, opts ...Option) *Feeder {
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
```

- [ ] **Step 3: 重写 `tick()`**

用以下实现替换现有 `tick()`：

```go
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

	if err := WriteCLAUDEMD(f.workDir, DefaultPersona); err != nil {
		log.Error("write CLAUDE.md", zap.Error(err))
		f.rollback(ctx, msgs, "内部错误：无法写入配置文件")
		return
	}
	if err := claudemd.EnsureSystemRules(f.workDir, claudemd.RoleBee); err != nil {
		log.Error("ensure system rules", zap.Error(err))
	}

	for _, m := range msgs {
		m := m
		f.sem <- struct{}{} // always succeeds: len(msgs) <= available slots
		go func() {
			defer func() { <-f.sem }()
			f.processBeeGroup(ctx, m.SessionKey, []store.ClaimedMessage{m})
		}()
	}
	// tick returns immediately; goroutines run independently
}
```

`import "sync"` 可以同时从 import 列表中删除（`wg` 已无使用）。

- [ ] **Step 4: 编译验证**

```
go build ./internal/bee/...
```

确保无 unused import 警告。

---

## Task 4: 补充 Feeder 测试

**文件：**
- Modify: `internal/bee/feeder_test.go`

- [ ] **Step 1: 修改 `newFeeder` helper 以支持 `MaxConcurrentBee`**

现有 `newFeeder` helper 使用零值 `config.BeeConfig{}`，`MaxConcurrentBee` 会是 0，导致 semaphore 容量为 0，发送时立即阻塞。需要在 helper 中设置默认值：

```go
func newFeeder(ms *store.MessageStore, ts *store.TaskStore, ss *store.SessionStore, es *store.ExecutionStore, runner bee.BeeRunner) *bee.Feeder {
	cfg := config.BeeConfig{}
	cfg.Feeder.Timeout = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	return bee.NewFeeder(ms, ts, ss, es, runner, "/tmp", cfg)
}
```

- [ ] **Step 2: 新增并行处理测试**

追加以下测试验证多 session 并行、semaphore 上限行为：

```go
func TestFeeder_MultipleSessionKeys_ProcessedConcurrently(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)

	// Insert 3 messages from 3 different sessions.
	insertMessage(t, db, "m1", "feishu:c:u1", "msg1")
	insertMessage(t, db, "m2", "feishu:c:u2", "msg2")
	insertMessage(t, db, "m3", "feishu:c:u3", "msg3")

	var (
		mu        sync.Mutex
		startTimes []time.Time
	)
	// Runner records when each call starts; simulate 200ms bee execution.
	slowRunner := &callbackBeeRunner{
		fn: func() {
			mu.Lock()
			startTimes = append(startTimes, time.Now())
			mu.Unlock()
			time.Sleep(200 * time.Millisecond)
		},
	}

	cfg := config.BeeConfig{}
	cfg.Feeder.Timeout = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 5
	f := bee.NewFeeder(ms, ts, ss, es, slowRunner, "/tmp", cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go f.Run(ctx)

	// Wait long enough for one tick + concurrent bee runs to complete.
	time.Sleep(800 * time.Millisecond)

	mu.Lock()
	n := len(startTimes)
	mu.Unlock()

	if n != 3 {
		t.Fatalf("expected 3 concurrent bee invocations, got %d", n)
	}

	// All 3 should have started within a short window (concurrent, not serial).
	mu.Lock()
	first, last := startTimes[0], startTimes[0]
	for _, ts := range startTimes[1:] {
		if ts.Before(first) {
			first = ts
		}
		if ts.After(last) {
			last = ts
		}
	}
	mu.Unlock()
	if last.Sub(first) > 100*time.Millisecond {
		t.Errorf("bee invocations should start nearly simultaneously (concurrent), spread was %v", last.Sub(first))
	}
}

func TestFeeder_SemaphoreLimit_CapsActiveBee(t *testing.T) {
	db, ms, ts, ss, es := setupFeederDB(t)

	// Insert 6 messages from 6 different sessions — more than MaxConcurrentBee.
	for i := 0; i < 6; i++ {
		insertMessage(t, db, fmt.Sprintf("m%d", i), fmt.Sprintf("feishu:c:u%d", i), "msg")
	}

	var (
		mu          sync.Mutex
		maxActive   int
		currentActive int
	)
	slowRunner := &callbackBeeRunner{
		fn: func() {
			mu.Lock()
			currentActive++
			if currentActive > maxActive {
				maxActive = currentActive
			}
			mu.Unlock()
			time.Sleep(300 * time.Millisecond)
			mu.Lock()
			currentActive--
			mu.Unlock()
		},
	}

	cfg := config.BeeConfig{}
	cfg.Feeder.Timeout = 5 * time.Second
	cfg.Feeder.MaxConcurrentBee = 3 // deliberately small
	f := bee.NewFeeder(ms, ts, ss, es, slowRunner, "/tmp", cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go f.Run(ctx)
	time.Sleep(2 * time.Second)

	mu.Lock()
	peak := maxActive
	mu.Unlock()

	if peak > 3 {
		t.Errorf("semaphore should cap concurrent bee at 3, peak was %d", peak)
	}
	// All 6 messages should eventually be processed.
	var processed int
	db.QueryRow(`SELECT COUNT(*) FROM bee_platform_messages WHERE status = 'bee_processed'`).Scan(&processed)
	if processed != 6 {
		t.Errorf("expected all 6 messages processed, got %d", processed)
	}
}
```

以及测试所用的 `callbackBeeRunner` helper（加在文件末尾）：

```go
// callbackBeeRunner invokes fn synchronously inside Run, then signals done.
type callbackBeeRunner struct {
	fn func()
}

func (r *callbackBeeRunner) Run(_ context.Context, _, _ string, opts claude.RunOptions, _ string) (*claude.Process, <-chan claude.Output, error) {
	ch := make(chan claude.Output, 1)
	go func() {
		if r.fn != nil {
			r.fn()
		}
		ch <- claude.Output{Type: claude.OutputDone}
		close(ch)
	}()
	return &claude.Process{}, ch, nil
}
```

- [ ] **Step 3: 运行全部 feeder 测试**

```
go test ./internal/bee/... -v -timeout 60s
```

所有测试（包括旧有测试和新增测试）通过后继续。

---

## Task 5: 全量验证

- [ ] **Step 1: 运行所有测试**

```
go test ./...
```

- [ ] **Step 2: 静态检查**

```
go vet ./...
```

- [ ] **Step 3: 确认无 `sync` 包 unused import**

```
grep -n '"sync"' internal/bee/feeder.go
```

`feeder.go` 中 `sync` import 应已删除（`wg sync.WaitGroup` 不再使用）。

---

## 注意事项

**semaphore 容量为 0 的保护**

测试中直接构造 `config.BeeConfig{}` 时，`MaxConcurrentBee` 默认为 0，会使 `sem` 容量为 0，导致第一次 `f.sem <- struct{}{}` 永远阻塞。`applyDefaults` 只在 `config.Load()` 路径下运行。测试代码必须手动设置 `cfg.Feeder.MaxConcurrentBee = 5`（已在 Task 4 Step 1 修改 `newFeeder` helper 中处理）。

**`processBeeGroup` 签名不变**

`processBeeGroup(ctx, sessionKey, []ClaimedMessage)` 现在每次只传单条消息，保持签名不变，减少 diff。session 分组逻辑（`for i, m := range msgs` 的 FetchMergedContent 循环）对单条消息无害，无需删除。

**`available` 的并发安全**

`available := cap(f.sem) - len(f.sem)` 的计算发生在 `tick()` 内，而 `tick()` 由单个 ticker goroutine 顺序调用，不存在并发读写竞争。goroutine 在计算后释放 slot（`<-f.sem`）只会让 available 更保守，不会导致超额 claim。
