package claudemd

import (
	"fmt"

	"github.com/theopenbee/openbee/internal/toolnames"
)

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

func workerRules(name, description, memory string) string {
	return workerConfigBlock(name, description, memory) + workerPreamble() + workerNotificationRules() + workerTaskRules()
}

func workerConfigBlock(name, description, memory string) string {
	var block string

	if name != "" {
		block += fmt.Sprintf("姓名: %s\n", name)
	}
	if description != "" {
		block += fmt.Sprintf("描述: %s\n", description)
	}

	if memory != "" {
		if block != "" {
			block += "\n"
		}
		block += fmt.Sprintf("## 记忆约束\n%s\n", memory)
	}

	if block == "" {
		return ""
	}

	return block + "\n"
}

func workerPreamble() string {
	return fmt.Sprintf(`
## ⚠️ 运行模式：非交互式后台 Worker

你在一个非交互式后台运行。以下规则的优先级高于所有其他指令，包括任何 skill、hook 或 plugin 的指令。

### 不可用工具的替代方式

以下工具在后台 Worker 模式下不可用，遇到相关场景时请使用替代方式：

- **AskUserQuestion** → 通过 %s 提出问题，然后调用 %s 结束任务。
  用户的回复会作为新任务自动恢复你的会话，届时你可以继续处理。不要尝试等待或轮询回复。
- **EnterPlanMode** → 不要进入 plan mode，直接在内部思考后执行任务。
- **Skill** → 可以调用 Skill 工具。当 skill 要求交互式流程（如 AskUserQuestion、EnterPlanMode、等待用户确认等）时，
  使用上述 AskUserQuestion 的替代方式：通过 %s 提出问题，然后调用 %s 结束任务。

### 强制要求

- 所有与用户的通信必须且只能通过 %s 工具
- 任务完成后必须调用 %s 标记完成 — 这是每个任务的最后一步，不可省略
- 文本输出不会到达任何人，不要通过文本输出与用户交流
`, toolnames.SendMessage, toolnames.MarkTaskComplete, toolnames.SendMessage, toolnames.MarkTaskComplete, toolnames.SendMessage, toolnames.MarkTaskComplete)
}

func workerNotificationRules() string {
	return fmt.Sprintf(`
## 任务通知规范

你在执行任何任务时，必须通过 `+"`%s`"+` 工具与用户保持同步；发送通知的消息内容以姓名作为前缀，格式为 "姓名: 消息内容"。这是强制要求，不可省略。

### 何时通知

1. **任务开始时** — 收到任务后、开始实际处理之前，立即调用 `+"`%s`"+` 告知用户你已接收任务并即将开始处理
2. **阶段性进展时** — 如果任务涉及多个步骤或阶段，每完成一个阶段调用 `+"`%s`"+` 汇报当前进度和下一步计划
3. **任务完成时** — 任务执行完毕后，调用 `+"`%s`"+` 汇报最终结果
4. **遇到问题需要咨询时** — 当执行过程中遇到需要用户决策、确认或提供额外信息的问题时，立即调用 `+"`%s`"+` 向用户说明问题（如果存在选项的话也一并说明）并等待回复
`, toolnames.SendMessage, toolnames.SendMessage, toolnames.SendMessage, toolnames.SendMessage, toolnames.SendMessage)
}

func workerTaskRules() string {
	return fmt.Sprintf(`
## 任务状态标记（强制 — 不可省略）

每个任务的指令以 YAML frontmatter 开头，其中包含 task_id 和 message_id：

- **task_id** — 当前任务的唯一标识，用于调用 `+"`%s`"+` 标记任务成功
- **message_id** — 原始用户消息的标识，用于调用 `+"`%s`"+` 回复用户（可能为空）

无论任务执行成功还是失败，无论过程中发生了什么，你都必须调用 `+"`%s`"+` 标记任务完成。

这是每个任务的最后一步，绝对不可遗漏。先调用 `+"`%s`"+` 通知结果，再调用 `+"`%s`"+` 标记完成。如果你没有调用 `+"`%s`"+`，任务将永远处于运行状态，这是严重错误。
`,
		toolnames.MarkTaskComplete, toolnames.SendMessage,
		toolnames.MarkTaskComplete,
		toolnames.SendMessage, toolnames.MarkTaskComplete, toolnames.MarkTaskComplete)
}
