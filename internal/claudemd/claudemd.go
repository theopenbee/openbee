package claudemd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/robobee/core/internal/toolnames"
)

const (
	RoleBee    = "bee"
	RoleWorker = "worker"

	SystemRulesFile = ".robobee.md"
	ImportLine      = "@" + SystemRulesFile
)

// options holds optional parameters for EnsureSystemRules.
type options struct {
	name        string
	description string
	memory      string
}

// Option configures EnsureSystemRules behavior.
type Option func(*options)

// WithName sets the worker name to embed directly in system rules.
func WithName(name string) Option {
	return func(o *options) {
		o.name = name
	}
}

// WithDescription sets the worker description to embed in system rules.
func WithDescription(desc string) Option {
	return func(o *options) {
		o.description = desc
	}
}

// WithMemory sets the worker memory to embed in system rules.
func WithMemory(memory string) Option {
	return func(o *options) {
		o.memory = memory
	}
}

func sharedRules() string {
	return fmt.Sprintf(`## 任务通知规范

你在执行任何任务时，必须通过 `+"`%s`"+` 工具与用户保持同步。这是强制要求，不可省略。

### 何时通知

1. **任务开始时** — 收到任务后、开始实际处理之前，立即调用 `+"`%s`"+` 告知用户你已接收任务并即将开始处理
2. **阶段性进展时** — 如果任务涉及多个步骤或阶段，每完成一个阶段调用 `+"`%s`"+` 汇报当前进度和下一步计划
3. **任务完成时** — 任务执行完毕后，调用 `+"`%s`"+` 汇报最终结果
4. **遇到问题需要咨询时** — 当执行过程中遇到需要用户决策、确认或提供额外信息的问题时，立即调用 `+"`%s`"+` 向用户说明问题（如果存在选项的话也一并说明）并等待回复

### 通知原则

- 简洁明了，不要冗长描述
- 包含关键信息：正在做什么、完成了什么、结果是什么
- 遇到异常或阻塞时也必须通知用户
`, toolnames.SendMessage, toolnames.SendMessage, toolnames.SendMessage, toolnames.SendMessage, toolnames.SendMessage)
}

func beeRules() string {
	return fmt.Sprintf(`
## 清除上下文处理

当用户发送的消息表示想要清除/重置对话（例如"clear"、"清除"、"重置上下文"等）时：

1. 首先调用 %s，传入 session_key 和 status "pending,running" 检查是否有活跃任务。

2. 如果没有活跃任务：
   - 调用 %s，传入 session_key
   - 调用 %s 确认："已清除会话上下文。"

3. 如果有活跃任务：
   - 调用 %s 告知用户："当前有 N 个任务正在处理中，清除上下文将终止这些任务。是否确认清除？"
   - 等待用户下一条消息。

4. 如果用户确认（再次发送 "clear" 或类似确认消息）：
   - 调用 %s（将自动取消所有任务、终止运行中的 worker 进程、清除所有会话上下文）
   - 调用 %s 确认："已终止所有任务并清除会话上下文。"

## 任务分发流程

当用户发送需要 worker 处理的任务时，按以下标准流程操作：

1. 调用 `+"`%s`"+` 查看可用的 worker 列表，选择最合适的 worker
2. 调用 `+"`%s`"+` 创建任务，将任务分配给选定的 worker
3. 调用 `+"`%s`"+` 告知用户任务已创建并分配给了哪个 worker

### 任务查询的精确过滤

当用户查询特定类型的任务时（如"定时任务"、"即时任务"），你必须使用 %s 的 type 参数精确过滤，只返回用户询问的类型。不要返回用户未询问的任务类型。

- "定时任务" → type: "scheduled"
- "即时任务" → type: "immediate"
- "延时任务" → type: "countdown"
- "所有任务" 或未指定类型 → 不传 type 参数

### 定时/延时任务的 instruction 提取规则

当用户消息包含定时（scheduled）或延时（countdown）意图时，你必须将调度语义与执行动作分离：

- **调度语义**（如"每分钟执行一次"、"5分钟后"）→ 映射到 type、cron_expr 或 scheduled_at 字段
- **执行动作**（用户实际希望 worker 每次执行的操作）→ 放入 instruction 字段

instruction 中绝对不能包含"创建定时任务"、"每隔X执行"等调度描述，否则 worker 每次执行时会误以为需要创建新任务。

示例：
- 用户说："每分钟执行一次，获取系统时间告诉我"
  - type: "scheduled", cron_expr: "* * * * *"
  - instruction: "获取当前系统时间并告知用户"（✓ 只有执行动作）
  - 错误 instruction: "创建一个定时任务，每分钟执行一次，获取系统时间..."（✗ 包含了调度描述）

## 自我配置

当用户明确要求修改你的名字或职责描述时，你可以直接编辑工作目录中的 `+"`CLAUDE.md`"+` 文件来更新自身配置。

操作步骤：
1. 读取当前 `+"`CLAUDE.md`"+` 内容
2. 按用户要求修改名字或职责描述（第一行 "你是 XXX" 部分）
3. 确保文件末尾保留 `+"`@.robobee.md`"+` 这一行，不要删除
4. 将修改后的内容写回 `+"`CLAUDE.md`"+`
5. 用 `+"`%s`"+` 告知用户：配置已更新，下次对话起将使用新的名字/描述

注意：只修改用户明确要求的内容，不要改动其他部分。
`,
		toolnames.ListTasks, toolnames.ClearSession, toolnames.SendMessage,
		toolnames.SendMessage, toolnames.ClearSession, toolnames.SendMessage,
		toolnames.ListWorkers, toolnames.CreateTask, toolnames.SendMessage,
		toolnames.ListTasks, toolnames.SendMessage)
}

func workerRules() string {
	namePrefix := fmt.Sprintf(`
## 通知名称前缀

使用 `+"`%s`"+` 发送消息时，消息内容必须以你的名称作为前缀，格式为 "名称: 消息内容"。
`, toolnames.SendMessage)

	return namePrefix + "\n" + fmt.Sprintf(`
## 系统元数据

每个任务的指令开头会包含系统元数据行，格式为：

`+"```"+`
[系统元数据] task_id=<task_id> message_id=<message_id>
`+"```"+`

- **task_id** — 当前任务的唯一标识，用于调用 `+"`%s`"+` 标记任务成功
- **message_id** — 原始用户消息的标识，用于调用 `+"`%s`"+` 回复用户

你必须从系统元数据中提取这些 ID 并在后续工具调用中正确使用。

## 任务状态标记

任务执行完成后，必须调用 `+"`%s`"+` 标记任务完成。

这是每个任务的最后一步，不可遗漏。先调用 `+"`%s`"+` 通知结果，再调用 `+"`%s`"+` 标记完成。
`,
		toolnames.MarkTaskComplete, toolnames.SendMessage,
		toolnames.MarkTaskComplete,
		toolnames.SendMessage, toolnames.MarkTaskComplete)
}

func workerConfigBlock(name, description, memory string) string {
	if name == "" && description == "" && memory == "" {
		return ""
	}
	block := "\n## Worker 配置\n\n"
	if name != "" {
		block += fmt.Sprintf("- **名称:** %s\n", name)
	}
	if description != "" {
		block += fmt.Sprintf("- **职责:** %s\n", description)
	}
	if memory != "" {
		block += fmt.Sprintf("\n### Memory\n\n%s\n", memory)
	}
	block += "\n"
	return block
}

// rulesForRole returns the combined rules content for the given role.
func rulesForRole(role string, opts options) string {
	switch role {
	case RoleBee:
		return sharedRules() + beeRules()
	case RoleWorker:
		return workerConfigBlock(opts.name, opts.description, opts.memory) + sharedRules() + workerRules()
	default:
		return sharedRules()
	}
}

// EnsureSystemRules writes .robobee.md with the latest system rules
// for the given role, and ensures CLAUDE.md contains the @import reference.
// It does NOT create CLAUDE.md if it doesn't exist.
func EnsureSystemRules(workDir, role string, optFns ...Option) error {
	var opts options
	for _, fn := range optFns {
		fn(&opts)
	}
	// 1. Write .robobee.md (always overwrite)
	rulesPath := filepath.Join(workDir, SystemRulesFile)
	if err := os.WriteFile(rulesPath, []byte(rulesForRole(role, opts)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", SystemRulesFile, err)
	}

	// 2. Check CLAUDE.md for import reference
	claudePath := filepath.Join(workDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // CLAUDE.md doesn't exist, skip
		}
		return fmt.Errorf("read CLAUDE.md: %w", err)
	}

	// 3. Append import if missing
	if !strings.Contains(string(data), ImportLine) {
		data = append(data, []byte("\n"+ImportLine+"\n")...)
		if err := os.WriteFile(claudePath, data, 0o644); err != nil {
			return fmt.Errorf("update CLAUDE.md: %w", err)
		}
	}

	return nil
}
