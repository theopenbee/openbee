# Bee Notification Spec Redesign Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite bee notification rules to use coordinator/dispatcher semantics instead of task-executor semantics, and simplify dispatch rules by removing redundant `send_message` references.

**Architecture:** Two functions in `internal/claudemd/bee.go` are modified: `beeNotificationRules()` is rewritten with 4 coordinator-oriented notification timing points; `beeContextAndDispatchRules()` has 6 of its 8 `SendMessage` format parameters replaced with inline text "按通知规范告知用户". Existing tests are updated to match.

**Tech Stack:** Go

**Spec:** `docs/superpowers/specs/2026-03-19-bee-notification-spec-design.md`

---

## Chunk 1: Implementation

### Task 1: Rewrite `beeNotificationRules()`

**Files:**
- Modify: `internal/claudemd/bee.go:23-36`

- [ ] **Step 1: Rewrite `beeNotificationRules()` function body**

Replace the current function (lines 23-36) with coordinator-oriented notification rules. The function keeps `fmt.Sprintf` with 1 `toolnames.SendMessage` parameter (for the preamble tool name reference).

```go
func beeNotificationRules() string {
	return fmt.Sprintf(`
## 任务通知规范

你在协调和调度过程中，必须通过 `+"`%s`"+` 工具与用户保持同步；发送通知的消息内容以姓名作为前缀，格式为 "姓名: 消息内容"。这是强制要求，不可省略。

### 何时通知

1. **收到用户请求时** — 确认已收到请求，告知正在分析需求并匹配合适的员工
2. **任务已派发时** — 告知用户任务已分配给哪个员工，简要说明分配理由
3. **派发遇到问题时** — 无匹配员工、需要用户从候选人中选择、或需要用户提供更多信息时，立即告知并说明情况
4. **元操作完成时** — bee 自行处理的操作（会话管理、配置更新、状态查询等）完成后，告知用户结果
`, toolnames.SendMessage)
}
```

- [ ] **Step 2: Run tests to verify compilation**

Run: `cd /Users/tengteng/work/theopenbee/openbee && go test ./internal/claudemd/ -run TestEnsureSystemRules_WritesBeeRules -v`
Expected: PASS (test checks for "任务通知规范" and "send_message" which are both present in new text)

- [ ] **Step 3: Commit**

```bash
git add internal/claudemd/bee.go
git commit -m "refactor(claudemd): rewrite bee notification rules with coordinator semantics"
```

### Task 2: Simplify `beeContextAndDispatchRules()` — remove 6 redundant `SendMessage` refs

**Files:**
- Modify: `internal/claudemd/bee.go:38-146`

The current `fmt.Sprintf` has 16 format parameters. After this change it will have 10. Six `%s` placeholders that resolved to `toolnames.SendMessage` are replaced with hardcoded text "按通知规范告知用户".

Here is the mapping of all 16 current parameters and which 6 are removed:

| # | Current param | Line context | Action |
|---|---------------|-------------|--------|
| 1 | ListSessionContexts | line 44: `调用 %s 列出当前 session` | keep |
| 2 | ListTasks | line 50: `调用 %s，检查是否有活跃任务` | keep |
| 3 | SendMessage | line 50: `调用 %s 告知用户："当前有 N 个任务..."` | **keep** (multi-turn) |
| 4 | ClearSession | line 52: `调用 %s（传入 session_key` | keep |
| 5 | SendMessage | line 53: `调用 %s 向用户展示受影响的员工列表` | **keep** (multi-turn) |
| 6 | ClearSession | line 53: `以 force=true 重新调用 %s` | keep |
| 7 | SendMessage | line 54: `调用 %s 确认："已清除会话上下文。"` | **REMOVE** |
| 8 | ClearWorkerSession | line 60: `调用 %s，传入 session_key` | keep |
| 9 | SendMessage | line 61: `调用 %s 确认："已重置 [员工名]..."` | **REMOVE** |
| 10 | ListWorkers | line 67: `先调用 %s 获取所有可用 worker` | keep |
| 11 | CreateTask | line 72: `调用 %s 创建任务` | keep |
| 12 | SendMessage | line 73: `调用 %s 告知用户任务已分配` | **REMOVE** |
| 13 | SendMessage | line 84: `通过 %s 列出候选 worker` | **REMOVE** |
| 14 | SendMessage | line 101: `通过 %s 告知用户当前无合适的员工` | **REMOVE** |
| 15 | ListTasks | line 105: `使用 %s 的 type 参数` | keep |
| 16 | SendMessage | line 136: `用 %s 告知用户：配置已更新` | **REMOVE** |

- [ ] **Step 1: Replace 6 `%s` placeholders in the format string with inline text**

For each of the 6 REMOVE entries, change the template text from using `%s` to hardcoded "按通知规范告知用户":

1. Line 54: `调用 %s 确认："已清除会话上下文。"` → `按通知规范告知用户会话已清除。`
2. Line 61: `调用 %s 确认："已重置 [员工名] 的对话上下文，下次任务将以全新会话开始。"` → `按通知规范告知用户该员工上下文已重置，下次任务将以全新会话开始。`
3. Line 73: `调用 `+"`%s`"+` 告知用户任务已分配` → `按通知规范告知用户任务已分配`
4. Line 84: `通过 `+"`%s`"+` 列出候选 worker 让用户选择` → `按通知规范列出候选员工让用户选择`
5. Line 101: `通过 `+"`%s`"+` 告知用户当前无合适的员工处理该需求` → `按通知规范告知用户当前无合适的员工处理该需求`
6. Line 136: `用 `+"`%s`"+` 告知用户：配置已更新，下次对话起将使用新的名字/描述` → `按通知规范告知用户：配置已更新，下次对话起将使用新的名字/描述`

- [ ] **Step 2: Update the `fmt.Sprintf` parameter list**

Replace the current parameter list (lines 139-145):

```go
// Current (16 params):
toolnames.ListSessionContexts, toolnames.ListTasks, toolnames.SendMessage,
toolnames.ClearSession, toolnames.SendMessage, toolnames.ClearSession, toolnames.SendMessage,
toolnames.ClearWorkerSession, toolnames.SendMessage,
toolnames.ListWorkers, toolnames.CreateTask, toolnames.SendMessage,
toolnames.SendMessage, toolnames.SendMessage,
toolnames.ListTasks, toolnames.SendMessage
```

With the new list (10 params):

```go
// New (10 params):
toolnames.ListSessionContexts, toolnames.ListTasks, toolnames.SendMessage,
toolnames.ClearSession, toolnames.SendMessage, toolnames.ClearSession,
toolnames.ClearWorkerSession,
toolnames.ListWorkers, toolnames.CreateTask,
toolnames.ListTasks
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/tengteng/work/theopenbee/openbee && go test ./internal/claudemd/ -v`
Expected: ALL PASS. Key checks:
- `TestEnsureSystemRules_WritesBeeRules`: checks "任务通知规范", "send_message", "会话上下文管理", "create_task", "list_workers", "任务分发流程" — all still present
- No bee test checks for the specific notification text that was changed

- [ ] **Step 4: Commit**

```bash
git add internal/claudemd/bee.go
git commit -m "refactor(claudemd): simplify bee dispatch rules by referencing notification spec"
```

### Task 3: Verify full test suite

- [ ] **Step 1: Run all claudemd tests**

Run: `cd /Users/tengteng/work/theopenbee/openbee && go test ./internal/claudemd/ -v`
Expected: ALL PASS

- [ ] **Step 2: Run full project tests to check for regressions**

Run: `cd /Users/tengteng/work/theopenbee/openbee && go test ./...`
Expected: ALL PASS (no other packages depend on the bee rules string content)
