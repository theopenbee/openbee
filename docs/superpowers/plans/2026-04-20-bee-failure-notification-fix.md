# Bee 执行失败 IM 通知错误信息修复计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 bee 执行失败时 IM 收到的错误信息与 `bee_executions.result` 不一致的问题，使 IM 展示真实的 API 错误而非 `exit status 1`，并消除误导性的"消息解析失败"文字。

**Root Cause（两个 bug）：**

1. `feeder.go:245`：`failMessages` 传入的是 `drainErr.Error()`（OS 进程退出码 `"exit status 1"`），而非已从日志解析出的真实错误 `resultMsg`。DB 写入用的是 `resultMsg` 所以是正确的，但 IM 通知用的是错误的 reason。
2. `feeder.go:284`：`failMessages` 调 `NotifyTaskFailure` 时未设置 `FailureInfo.WorkerName`，触发 `failure_notifier.go:43` 的 `ParseFailed` 分支，IM 出现"消息解析失败"。

**Architecture：** 两处改动均在 `internal/domain/bee/feeder.go`，不涉及其他文件。

---

### Task 1：修复 failMessages 传入错误的 reason（Bug 1）

**Files:**
- Modify: `internal/domain/bee/feeder.go:243-246`

**背景：**

当前代码在 `drainErr != nil` 分支：
```go
// line 243-246（当前）
if drainErr != nil {
    log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(drainErr))
    f.failMessages(ctx, msgs, drainErr.Error())  // ← 错误：传了 drainErr 而非 resultMsg
    return
}
```

`resultMsg` 已在第 232 行通过 `runRes.ExtractResult(logPath)` 正确提取出真实 API 错误，并在第 239 行写入 DB。应将 `resultMsg` 也传给 `failMessages`。

- [x] **Step 1：将 `drainErr.Error()` 改为 `resultMsg`**

```go
// line 243-246（修改后）
if drainErr != nil {
    log.Error("bee run failed", zap.String("sessionKey", sessionKey), zap.Error(drainErr))
    f.failMessages(ctx, msgs, resultMsg)  // ← 使用已提取的真实错误信息
    return
}
```

---

### Task 2：修复 IM 出现"消息解析失败"（Bug 2）

**Files:**
- Modify: `internal/domain/bee/feeder.go:284`

**背景：**

`failure_notifier.go:40-44` 逻辑：
```go
if info.WorkerName != "" {
    workerLine = fmt.Sprintf(m.WorkerLine, info.WorkerName)
} else {
    workerLine = m.ParseFailed  // "消息解析失败" ← bee 场景不应出现此文字
}
```

`failMessages` 构造 `FailureInfo` 时 `WorkerName` 为空，应传入 `"bee"` 作为标识。

- [x] **Step 1：在 `FailureInfo` 中设置 `WorkerName: "bee"`**

```go
// line 284（修改后）
if notifyErr := f.failureNotifier.NotifyTaskFailure(ctx, m.ID, model.FailureInfo{
    Reason:     reason,
    WorkerName: "bee",
}); notifyErr != nil {
```

---

### Task 3：验证

- [x] 确认两处改动编译通过：`go build ./internal/domain/bee/...` ✓
- [x] `go test ./internal/domain/bee/...` 全部通过 ✓
- [ ] 触发一次 bee 执行失败场景，检查 IM 通知显示的是真实错误信息，且不再出现"消息解析失败"
