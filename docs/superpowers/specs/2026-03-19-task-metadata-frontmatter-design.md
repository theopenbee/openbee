# 任务元数据格式改为 YAML Frontmatter

**日期：** 2026-03-19

## 背景

当前系统在向 worker 分发任务时，在指令开头注入元数据，格式为：

```
[系统元数据] task_id=<uuid> message_id=<uuid>

指令内容
```

该格式可读性较差，且标记风格与项目其他部分（使用 YAML frontmatter）不一致。

## 目标

将格式改为标准 YAML frontmatter：

```
---
task_id: <uuid>
message_id: <uuid>
---

指令内容
```

同时移除独立的 `## 系统元数据` 说明章节，将字段用途说明合并到 `## 任务状态标记` 节。

## 改动范围

### 1. `internal/task_dispatcher/dispatcher.go`

`buildInstruction` 函数完整替换（保留 `task_id` 为空时的早返回守卫）：

```go
// 改前
func buildInstruction(task DispatchTask) string {
    if task.TaskID == "" {
        return task.Instruction
    }
    return fmt.Sprintf("[系统元数据] task_id=%s message_id=%s\n\n%s",
        task.TaskID, task.MessageID, task.Instruction)
}

// 改后
func buildInstruction(task DispatchTask) string {
    if task.TaskID == "" {
        return task.Instruction
    }
    return fmt.Sprintf("---\ntask_id: %s\nmessage_id: %s\n---\n\n%s",
        task.TaskID, task.MessageID, task.Instruction)
}
```

### 2. `internal/claudemd/claudemd.go`

`workerRules` 函数中：

- 删除 `## 系统元数据` 整个章节（标题 + 格式示例 + 字段说明）
- 将字段来源说明合并到 `## 任务状态标记` 节开头

原有的 `"你必须从系统元数据中提取这些 ID 并在后续工具调用中正确使用"` 提示句**有意删除**，因为新的合并段落已通过字段说明（"用于调用 X"）传达了相同意图，无需重复。

改后的 `## 任务状态标记` 节应为：

```
## 任务状态标记（强制 — 不可省略）

每个任务的指令以 YAML frontmatter 开头，其中包含 task_id 和 message_id：

- **task_id** — 当前任务的唯一标识，用于调用 `mark_task_complete` 标记任务成功
- **message_id** — 原始用户消息的标识，用于调用 `send_message` 回复用户（可能为空）

无论任务执行成功还是失败，无论过程中发生了什么，你都必须调用 `mark_task_complete` 标记任务完成。

这是每个任务的最后一步，绝对不可遗漏。先调用 `send_message` 通知结果，再调用 `mark_task_complete` 标记完成。如果你没有调用 `mark_task_complete`，任务将永远处于运行状态，这是严重错误。
```

`workerPreamble()` 函数不受影响，无需修改。

### 3. `internal/task_dispatcher/dispatcher_test.go`

将现有两条断言**替换**为以下三条（不是新增）：

```go
// 改前（替换这两条）
strings.Contains(instr, "task_id=task-abc")
strings.Contains(instr, "message_id=msg-xyz")

// 改后（用这三条替换）
strings.HasPrefix(instr, "---\n")           // 验证 frontmatter 分隔符
strings.Contains(instr, "task_id: task-abc")
strings.Contains(instr, "message_id: msg-xyz")
```

### 4. `internal/claudemd/claudemd_test.go`

将现有断言**替换**，同时增加负断言确认旧章节已被删除：

```go
// 改前（替换这条）
strings.Contains(content, "系统元数据")

// 改后（用这两条替换）
strings.Contains(content, "task_id")
!strings.Contains(content, "系统元数据")   // 验证旧章节已删除
```

## 不在范围内

- 不改变 `task_id` / `message_id` 的实际值或语义
- 不改变 MCP 工具调用逻辑
- 不引入 frontmatter 解析库（worker 为 LLM，直接阅读文本）
- `message_id` 可能为空字符串（fire-and-forget 任务），frontmatter 中将显示 `message_id: `，此行为与改前一致，不在本次处理范围
