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

`buildInstruction` 函数的格式字符串：

```go
// 改前
return fmt.Sprintf("[系统元数据] task_id=%s message_id=%s\n\n%s",
    task.TaskID, task.MessageID, task.Instruction)

// 改后
return fmt.Sprintf("---\ntask_id: %s\nmessage_id: %s\n---\n\n%s",
    task.TaskID, task.MessageID, task.Instruction)
```

### 2. `internal/claudemd/claudemd.go`

`workerRules` 函数中：

- 删除 `## 系统元数据` 整个章节（标题 + 格式示例 + 字段说明）
- 在 `## 任务状态标记` 节开头补充字段来源说明，告知 worker 从 frontmatter 中读取 `task_id` 和 `message_id`

### 3. `internal/task_dispatcher/dispatcher_test.go`

断言改为匹配新格式：

```go
// 改前
strings.Contains(instr, "task_id=task-abc")
strings.Contains(instr, "message_id=msg-xyz")

// 改后
strings.Contains(instr, "task_id: task-abc")
strings.Contains(instr, "message_id: msg-xyz")
```

### 4. `internal/claudemd/claudemd_test.go`

断言改为检查新内容中的关键词（不再检查 `"系统元数据"`）：

```go
// 改前
strings.Contains(content, "系统元数据")

// 改后
strings.Contains(content, "task_id")
```

## 不在范围内

- 不改变 `task_id` / `message_id` 的实际值或语义
- 不改变 MCP 工具调用逻辑
- 不引入 frontmatter 解析库（worker 为 LLM，直接阅读文本）
