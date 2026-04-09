# @姓名 直接调度 Worker 设计文档

**日期：** 2026-04-08  
**状态：** 已确认，待实现

## 背景

目前所有用户消息都经由 Bee（Claude）调度后再分配给 Worker。现在需要新增一个快捷机制：当消息以 `@姓名 `（@加名字加空格）开头时，程序直接将任务分配给指定的 Worker，跳过 Bee 调度环节。

## 决策摘要

| 问题 | 决策 |
|---|---|
| Worker 不存在时 | 回退给 Bee 处理 |
| @姓名 前缀是否剥离 | 是，Worker 只收到正文内容 |
| 名字匹配规则 | 大小写不敏感；同名多个 Worker 取最早创建的 |
| Session 连续性 | 维持，与 immediate 任务行为一致 |
| 实现方案 | 方案一：在 Feeder 内部路由 |

## 架构

### 现有消息流

```
Platform → msgingest.Gateway → platform_messages(received)
                                        ↓
                               bee.Feeder (轮询)
                                        ↓
                                  Bee (Claude)
                                        ↓
                            MCP assign_task → tasks 表
                                        ↓
                           task.Scheduler → task.Dispatcher
                                        ↓
                              worker.Manager.ExecuteWorker
```

### 新增直接调度路径

```
platform_messages(received)
        ↓
  Feeder.claim()
        ↓
  合并消息内容（已有逻辑）
        ↓
  tryDirectDispatch()        ← 新增检测点
   ├── 未匹配 @姓名           → 原有 Bee 流程
   ├── 匹配但 Worker 不存在   → 原有 Bee 流程（回退）
   └── 匹配且 Worker 存在
         ├── 剥离 @姓名 前缀
         ├── 构建 DispatchTask (TaskType=immediate)
         ├── 投入 directDispatchCh → TaskDispatcher → Worker
         └── 标记 bee_processed（跳过 Bee）
```

## 组件变更

### 1. `internal/infra/store/worker_store.go`

新增按名字查询方法，大小写不敏感，同名取最早创建：

```go
func (s *WorkerStore) GetByName(name string) (model.Worker, error) {
    row := s.db.QueryRow(
        "SELECT "+workerColumns+" FROM bee_workers WHERE LOWER(name)=LOWER(?) ORDER BY created_at ASC LIMIT 1",
        name,
    )
    return scanWorker(row)
}
```

### 2. `internal/domain/bee/feeder.go`

新增接口、字段和 Option：

```go
// WorkerNameLookup 按名字查找 Worker。
type WorkerNameLookup interface {
    GetByName(name string) (model.Worker, error)
}

type Feeder struct {
    // ... 现有字段 ...
    workerLookup     WorkerNameLookup
    directDispatchCh chan<- task.DispatchTask
}

func WithDirectDispatch(ch chan<- task.DispatchTask, lookup WorkerNameLookup) Option {
    return func(f *Feeder) {
        f.directDispatchCh = ch
        f.workerLookup = lookup
    }
}
```

在 `processBeeGroup()` 中，合并内容后插入检测：

```go
if f.tryDirectDispatch(ctx, sessionKey, msgs, mergedContent, primaryMsgID) {
    return // 已直接分配，跳过 Bee
}
// 原有 Bee 流程继续 ...
```

`tryDirectDispatch()` 逻辑：
- 用正则 `^@(\S+)\s+` 检测合并内容前缀，提取 Worker 名字
- `workerLookup.GetByName(name)` 查找 Worker（大小写不敏感）
- 未匹配或查询失败或 Worker 不存在：return false，走 Bee 回退
- 匹配成功：
  - 剥离 `@姓名 ` 前缀得到正文
  - 构建 `DispatchTask{WorkerID, SessionKey, Instruction: 正文, MessageID, TaskType: immediate}`
  - 非阻塞投入 `directDispatchCh`（满则 warn 日志并回退 Bee）
  - 调用 `msgStore.MarkBeeProcessed(ctx, msgIDs)`
  - return true

### 3. `internal/domain/task/dispatcher.go`

修改 `buildInstruction()`，使 `message_id` 在没有 `task_id` 时也能注入，确保直接调度的 Worker 能调用 `send_message`：

```go
func buildInstruction(t DispatchTask) string {
    if t.TaskID == "" && t.MessageID == "" {
        return t.Instruction
    }
    header := fmt.Sprintf("---\nmessage_id: %s\n", t.MessageID)
    if t.TaskID != "" {
        header += fmt.Sprintf("task_id: %s\n", t.TaskID)
    }
    return header + "---\n\n" + t.Instruction
}
```

### 4. `internal/app/app.go`

在组装 Feeder 时注入新依赖（`dispatchCh` 和 `workerStore` 均为已有变量）：

```go
feeder := bee.NewFeeder(...,
    bee.WithDirectDispatch(dispatchCh, s.workerStore),
)
```

## Session 连续性

直接调度复用 `TaskDispatcher` 现有的 session 恢复机制：

- `DispatchTask.SessionKey` = 原始消息的 `session_key`
- `DispatchTask.WorkerID` = 匹配到的 Worker ID
- `TaskDispatcher.resolveExecution()` 调用 `GetSessionContext(ctx, sessionKey, workerID)`
- 首次对话开启新 session；后续同一用户再次 `@同名Worker` 时自动 resume

## 边界情况

| 情况 | 处理方式 |
|---|---|
| @name 在消息中间或结尾 | 不触发，正则只检测开头 |
| Worker 名含空格 | 无法被精准匹配，回退 Bee |
| 防抖合并后开头仍是 @name | 触发直接调度 |
| directDispatchCh 已满 | warn 日志，回退 Bee，不丢消息 |
| DB 查询失败 | error 日志，回退 Bee |
| Worker 状态为 working/error | 正常投入队列，TaskDispatcher 串行处理 |
| 无 task record | 直接调度不创建 tasks 表记录；Worker 指令头只含 message_id，可调用 send_message 但不能调用 mark_task_success（MVP 已知限制） |

## 改动范围

- 无 DB schema 变更
- 无新组件
- 改动文件：4 个（worker_store.go、feeder.go、dispatcher.go、app.go）
- 新增接口：`WorkerNameLookup`（便于单元测试 mock）
