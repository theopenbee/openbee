# Feeder Session-Parallel Scheduling Design

**Date:** 2026-03-26
**Status:** Draft

## Background

Feeder 负责从 `bee_platform_messages` 表中取出未处理的消息，驱动 bee（Claude CLI）执行，并维护 session 上下文的连续性。当前实现每次只取一条消息，且 tick 会阻塞直到 bee 执行完成，导致不同用户的消息只能串行处理。

## 现有调度机制分析

### 执行流程

```
Run() goroutine
  └─ 每 500ms ticker 触发一次 tick()
       └─ ClaimBatch(ctx, 1)          // 只取 1 条消息
            └─ 按 sessionKey 分组
                 └─ 并行启动 goroutines (实际上只有 1 个 group)
                      └─ processBeeGroup()  // 调用 bee，阻塞等待完成
       └─ wg.Wait()                   // tick 阻塞，等所有 bee 执行完
  └─ 下一次 ticker 才能被处理
```

### 现有问题

**问题 1：tick 阻塞导致跨 session 完全串行**

`tick()` 调用 `wg.Wait()` 等待 bee 完成，而 `Run()` 的 select 循环是单 goroutine。bee 执行时间通常为 10–60 秒，实际消息处理间隔为 `max(500ms, bee执行时长)`，而非预期的 500ms。

在此期间，session B 的消息即使已在队列中等待，也无法被处理。

**问题 2：并行分组逻辑是死代码**

`ClaimBatch` 固定传入 `batchSize=1`，tick 内的 session 分组和并行 goroutine 逻辑永远只有一个 group，从未真正并行执行过。

**问题 3：吞吐瓶颈**

当队列中有 N 个不同用户的消息时，实际处理时间为 `N × bee执行时长`，而非理想的单次 bee 执行时长。

## 优化方案：DB 驱动的 Session 并行调度

### 核心约束

- **同一 session 内的消息必须串行处理**：保证对话连贯性，维护 `--resume` 语义
- **不同 session 的消息可以完全并行**：互不影响

### 设计原则

将"同一 session 不双跑"的保证从应用层下沉到 **DB 层**，通过 SQL 查询本身排除已有 in-flight 消息的 session，无需内存锁。

### 新的 ClaimBatch SQL

修改查询逻辑：每次取最多 N 条消息，每个 session_key 最多取一条，且跳过已有 `feeding` 状态消息的 session：

```sql
SELECT id, session_key, platform, content, retry_count
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
LIMIT ?
```

**语义**：取每个"空闲 session"中最早收到的那条消息，最多取 N 条（N = MaxConcurrentBee）。

### 新的 tick() 逻辑

```
tick():
  msgs = ClaimBatch(ctx, MaxConcurrentBee)
  for each msg:
    semaphore.acquire()            // 限制最大并发
    go func():
      defer semaphore.release()
      processBeeGroup(ctx, msg)    // 独立 goroutine，不等待
  return                           // tick 立即返回，不 wg.Wait()
```

### Semaphore 并发控制

引入 channel-based semaphore 控制最大并发 bee 进程数：

```go
sem := make(chan struct{}, MaxConcurrentBee)
```

防止队列积压时（如 1000 个不同 session 同时有消息）一次性启动过多 bee 进程。`MaxConcurrentBee` 作为新配置项，建议默认值为 **5**。

### 新的执行流程

```
Run() goroutine
  └─ 每 500ms ticker 触发一次 tick()
       └─ ClaimBatch(ctx, MaxConcurrentBee)
            // 每 session 最多 1 条，跳过已 feeding 的 session
       └─ for each msg: sem.acquire() + go processBeeGroup()
       └─ tick 立即返回
  └─ 下一次 ticker 正常触发（不受 bee 执行时长影响）

processBeeGroup goroutine (独立运行)
  └─ 执行 bee（10–60s）
  └─ 完成后 sem.release()
  └─ 下一次 tick 即可为该 session 取下一条消息
```

## 配置变更

在 `config.BeeConfig.Feeder` 中新增：

| 配置项 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `MaxConcurrentBee` | `int` | `5` | 最大并发 bee 进程数，同时也是每次 ClaimBatch 的 batchSize |

常量 `MaxRetries`、`PollInterval`、`QueueWarnThreshold` 保持不变。

## 潜在风险与应对

### 风险 1：SQLite 并发写压力

多个 goroutine 并发完成时，会同时写 DB（`MarkBeeProcessed`、`UpsertSessionContext`）。SQLite 默认 WAL 模式支持并发读 + 单写，写操作会自动串行化，不会有数据损坏，但高并发下可能有短暂锁等待。

**应对**：MaxConcurrentBee 默认 5，并发写压力可控；已有 WAL 模式无需额外处理。

### 风险 2：NOT IN 子查询性能

`session_key NOT IN (SELECT session_key ... WHERE status = 'feeding')` 在 `feeding` 消息较多时，子查询结果集可能较大。

**应对**：`feeding` 状态消息数量即为当前并发 bee 数（上限 MaxConcurrentBee=5），子查询结果集极小，不存在性能问题。`status` 字段已有索引。

### 风险 3：goroutine 泄漏

processBeeGroup goroutine 持有 semaphore slot，若 bee 执行异常且 goroutine panic，slot 不会释放，最终导致所有 slot 耗尽。

**应对**：semaphore release 通过 `defer` 调用，panic 也会正确释放。

### 风险 4：Context 传播

tick 非阻塞后，processBeeGroup goroutine 持有的是根 context（来自 `Run()`），服务关闭时根 ctx 取消，`beeCtx` 超时或服务关闭都能正确传播给 in-flight bee 进程。此行为与现有实现一致。

### 风险 5：崩溃恢复

进程崩溃时，in-flight 消息停留在 `feeding` 状态。已有 `RecoverFeeding()` 在启动时将所有 `feeding` 消息重置为 `received`，此机制无需修改，完全可复用。

## 改动范围

| 文件 | 改动 |
|---|---|
| `internal/store/message_store.go` | 修改 `ClaimBatch` SQL，支持 per-session 去重 + 跳过 feeding session |
| `internal/bee/feeder.go` | 去掉 `wg.Wait()`；新增 semaphore；`tick()` 非阻塞 |
| `internal/bee/constants.go` | 新增 `MaxConcurrentBee` 常量（或迁入 config） |
| `internal/config/` | `BeeConfig.Feeder` 增加 `MaxConcurrentBee` 字段 |
| `internal/bee/feeder_test.go` | 补充多 session 并行测试；验证 semaphore 上限 |
| `internal/store/message_store_test.go` | 补充新 `ClaimBatch` 行为的单元测试 |

已有的 `RecoverFeeding`、retry 机制、`FailureNotifier` 逻辑均无需修改。

## 预期效果

- **延迟**：不同用户的消息处理延迟从 `N × bee时长` 降至接近 `1 × bee时长`
- **吞吐**：最多 MaxConcurrentBee（默认 5）个用户的消息并行处理
- **正确性**：同一 session 内消息依然严格串行，session 上下文连续性不受影响
- **可维护性**：session 分组逻辑简化（每条 claimed 消息即一个独立 group），死代码消除
