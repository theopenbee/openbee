# 失败通知改善设计

**日期**：2026-03-26
**状态**：待实现

## 背景

当前失败通知格式为：
```
[系统通知] 任务执行失败：<reason>
```

其中 `reason` 来源：
- Worker 失败路径：`exec.Result`，通常是 Go 原始报错（如 `exit status 1`）
- Bee 失败路径（重试耗尽后）：硬编码的中文短语（如"AI 处理失败，请稍后重试"）

用户反馈通知体验差，具体痛点：
- A. 原始报错技术性太强，用户不知道发生了什么
- B. 不知道系统是否已经自动重试过
- C. 有多个 Worker 时不知道是哪个出了问题

## 目标

帮助用户在收到失败通知后，能够**自行判断是否值得重试**，不需要找开发者。

## 设计方案

### 新通知格式

```
❌ 任务执行失败
Worker：<WorkerName>
已重试：<RetryCount>/<MaxRetries> 次
错误：<raw error details>
```

示例：
```
❌ 任务执行失败
Worker：数据分析助手
已重试：3/3 次
错误：exit status 1: error connecting to claude API
```

说明：
- 不包含"建议"字段——用户为技术型，原始报错本身即为决策依据
- 保留现有 500 rune 截断逻辑

### 架构变更

#### 新增 `FailureInfo` 结构体

在 `internal/task_dispatcher/failure_notifier.go` 中新增：

```go
type FailureInfo struct {
    Reason     string // 原始错误信息（来自 exec.Result 或 err.Error()）
    WorkerName string // Worker 或 Bee 名称（用于定位）
    RetryCount int    // 已重试次数；-1 表示无重试机制，格式化时省略该行
    MaxRetries int    // 最大重试上限；RetryCount=-1 时忽略此字段
}
```

#### `NotifyTaskFailure` 签名变更

```go
// 变更前
func (n *PlatformFailureNotifier) NotifyTaskFailure(ctx context.Context, messageID string, reason string) error

// 变更后
func (n *PlatformFailureNotifier) NotifyTaskFailure(ctx context.Context, messageID string, info FailureInfo) error
```

#### 消息格式化逻辑

```go
content := fmt.Sprintf(
    "❌ 任务执行失败\nWorker：%s\n已重试：%d/%d 次\n错误：%s",
    info.WorkerName,
    info.RetryCount,
    info.MaxRetries,
    info.Reason,
)
```

### 调用点变更

| 位置 | 当前传参 | 变更后传参 |
|------|---------|-----------|
| `feeder.go:262`（Bee 重试耗尽） | 硬编码字符串 | `WorkerName=msg.SessionKey`, `RetryCount=msg.RetryCount`, `MaxRetries=MaxRetries`, `Reason=原始 drainErr` |
| `dispatcher.go:204`（Worker 启动失败） | `err.Error()` | `WorkerName=exec.WorkerName`, `RetryCount=-1`（无重试）, `Reason=err.Error()` |
| `dispatcher.go:270`（Worker 执行失败） | `exec.Result` | `WorkerName=exec.WorkerName`, `RetryCount=-1`（无重试）, `Reason=exec.Result` |

> Worker 任务当前无自动重试机制。`RetryCount=-1` 为哨兵值，格式化时当 `RetryCount < 0` 时**省略"已重试"行**，避免显示无意义的"已重试：0/0 次"。

## 涉及文件

- `internal/task_dispatcher/failure_notifier.go` — 新增 `FailureInfo` 结构体，更新格式化逻辑
- `internal/task_dispatcher/dispatcher.go` — 更新两处 `notifyFailure()` 调用传参
- `internal/bee/feeder.go` — 更新 `rollback()` 及其调用点，传入重试上下文

## 不在范围内

- 错误分类/翻译层（可作为后续迭代）
- 分层通知（摘要 + 详情分离）
- Worker 任务的自动重试机制
