# Bee 通知规范重构设计

## 背景

`beeNotificationRules()`（bee.go:23-36）当前内容与 `workerNotificationRules()`（worker.go:82-95）几乎相同，都围绕"任务执行"的通知规范（任务开始、阶段性进展、任务完成、遇到问题）。但 bee 的角色是**协调者与调度员**，不执行业务任务。通知规范应当反映其协调/调度职责。

## 目标

1. 重写 `beeNotificationRules()`，将通知时机从"任务执行者"语义改为"协调者/调度员"语义
2. 精简 `beeContextAndDispatchRules()` 中与通知规范重叠的显式 `send_message` 描述，改为引用通知规范

## 设计

### 新的 `beeNotificationRules()` 内容

通知规范的前导句中，`send_message` 通过 `fmt.Sprintf` 的 `%s` 占位符引用 `toolnames.SendMessage`，与 codebase 中其他规则保持一致。

```
## 任务通知规范

你在协调和调度过程中，必须通过 `send_message` 工具与用户保持同步；发送通知的消息内容以姓名作为前缀，格式为 "姓名: 消息内容"。这是强制要求，不可省略。

### 何时通知

1. **收到用户请求时** — 确认已收到请求，告知正在分析需求并匹配合适的员工
2. **任务已派发时** — 告知用户任务已分配给哪个员工，简要说明分配理由
3. **派发遇到问题时** — 无匹配员工、需要用户从候选人中选择、或需要用户提供更多信息时，立即告知并说明情况
4. **元操作完成时** — bee 自行处理的操作（会话管理、配置更新、状态查询等）完成后，告知用户结果
```

> **关于 worker 结果转达**：worker 完成任务后直接通过 `send_message` 通知用户，bee 不参与结果转达。因此不需要"worker 任务完成时"的通知时机。

### 精简 `beeContextAndDispatchRules()` 中的冗余描述

`beeContextAndDispatchRules()` 中共有 8 个 `toolnames.SendMessage` 引用，按处理方式分为三类：

**替换为"按通知规范告知用户"（6 处）：**

| 位置 | 当前写法 | 改为 |
|------|----------|------|
| 清除会话完成确认（line 54） | `调用 %s 确认："已清除会话上下文。"` | `按通知规范告知用户会话已清除` |
| 重置员工上下文完成确认（line 61） | `调用 %s 确认："已重置 [员工名]..."` | `按通知规范告知用户该员工上下文已重置` |
| 规则1（明确指定员工，line 73） | `调用 send_message 告知用户任务已分配` | `按通知规范告知用户任务已分配` |
| 规则3（多个匹配，line 84） | `通过 send_message 列出候选 worker` | `按通知规范列出候选员工让用户选择` |
| 规则5（兜底，line 101） | `通过 send_message 告知用户当前无合适的员工` | `按通知规范告知用户当前无合适的员工` |
| 自我配置完成（line 136） | `用 send_message 告知用户：配置已更新` | `按通知规范告知用户配置已更新` |

**保留显式 `send_message` 引用（2 处）：**

| 位置 | 原因 |
|------|------|
| 清除会话前警告用户有活跃任务（line 50） | 多轮交互确认流程的一部分，需要等待用户确认后继续 |
| 清除会话时展示受影响员工列表（line 53） | 同上，需要用户确认后以 force=true 重新调用 |

### 代码影响

- **bee.go** `beeNotificationRules()`：重写函数体，`fmt.Sprintf` 保留 1 个 `toolnames.SendMessage` 参数（前导句中的工具名引用）
- **bee.go** `beeContextAndDispatchRules()`：移除 6 处 `toolnames.SendMessage` 格式化参数，对应文本改为"按通知规范告知用户"；保留 2 处多轮交互确认中的引用

## 不变的部分

- `workerNotificationRules()` 不受影响
- `beePreamble()` 和 `beeMemoryAndStatusRules()` 不受影响
