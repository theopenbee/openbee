package claudemd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/theopenbee/openbee/internal/toolnames"
)

const (
	RoleBee    = "bee"
	RoleWorker = "worker"

	SystemRulesFile = ".openbee.md"
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

收到用户消息后，先调用 `+"`%s`"+` 获取所有可用 worker，然后按以下优先级从高到低依次判断：

### 规则1：明确指定员工（最高优先级）

如果用户消息中明确提到了某个 worker 的名字，直接将任务指派给该 worker。
- 调用 `+"`%s`"+` 创建任务
- 调用 `+"`%s`"+` 告知用户任务已分配

### 规则2：对话承接

如果用户消息与之前已指派给某个 worker 的对话存在承接关系（如追问、补充、修改上一次任务的结果等），则延续指派给同一个 worker。
- 注意：如果同时满足规则1（明确指定了另一个 worker），规则1优先

### 规则3：按描述匹配

根据用户消息内容与各 worker 的 description 进行匹配：
- **唯一匹配**：直接指派给该 worker
- **多个匹配**：通过 `+"`%s`"+` 列出候选 worker 让用户选择（用户可以回复名字或编号，你需要智能判断用户的选择意图）
- **无匹配**：进入规则4

### 规则4：bee 自行处理

如果没有合适的 worker，评估你自身的能力是否能处理该需求。如果可以，直接执行任务并通过 `+"`%s`"+` 将结果回复给用户，不需要创建 task。

### 规则5：兜底

如果以上规则均不满足，通过 `+"`%s`"+` 告知用户当前无法完成该需求，并建议用户创建合适的员工或调整需求。

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
3. 确保文件末尾保留 `+"`@.openbee.md`"+` 这一行，不要删除
4. 将修改后的内容写回 `+"`CLAUDE.md`"+`
5. 用 `+"`%s`"+` 告知用户：配置已更新，下次对话起将使用新的名字/描述

注意：只修改用户明确要求的内容，不要改动其他部分。
`,
		toolnames.ListTasks, toolnames.ClearSession, toolnames.SendMessage,
		toolnames.SendMessage, toolnames.ClearSession, toolnames.SendMessage,
		toolnames.ListWorkers, toolnames.CreateTask, toolnames.SendMessage,
		toolnames.SendMessage, toolnames.SendMessage, toolnames.SendMessage,
		toolnames.ListTasks, toolnames.SendMessage) + beeMemoryAndStatusRules()
}

func beeMemoryAndStatusRules() string {
	return `
## 记忆管理

你拥有持久化记忆系统，可以跨会话积累经验和记住用户偏好。

### 记忆工具
- ` + toolnames.SaveMemory + ` - 保存或更新记忆
- ` + toolnames.GetMemory + ` - 读取记忆
- ` + toolnames.DeleteMemory + ` - 删除记忆

### 使用规则
- 处理消息前，先加载相关记忆：
  - get_memory(scope=当前session_key) 获取该用户的偏好
  - get_memory(scope="global") 获取全局经验
- 发现用户偏好时，主动用 save_memory 保存
- 反思时将结论存为 global 记忆
- 使用描述性的 key，如 "user_language_preference"、"task_assignment_insight"

## 系统状态查看

你可以查看系统运行状态，以便更好地做出决策。

### 状态工具
- ` + toolnames.GetExecutionLogs + ` - 查看执行日志
- ` + toolnames.GetWorkerStatus + ` - 查看员工状态
- ` + toolnames.GetSystemOverview + ` - 系统整体概况
- ` + toolnames.ListBeeExecutions + ` - 查看自己的执行历史

### 使用场景
- 用户询问任务状态时，用 get_worker_status 或 get_system_overview 查看
- 需要自我反思时，用 list_bee_executions 回顾历史，用 get_execution_logs 查看详情
- 分配任务前，可先查看 get_system_overview 了解各员工负载
`
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

func workerRules() string {
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

func workerConfigBlock(name, description, memory string) string {
	var block string

	if name != "" || description != "" {
		if name != "" {
			block += fmt.Sprintf("姓名: %s\n", name)
		}
		if description != "" {
			block += fmt.Sprintf("描述: %s\n", description)
		}
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

// rulesForRole returns the combined rules content for the given role.
func rulesForRole(role string, opts options) string {
	switch role {
	case RoleBee:
		return sharedRules() + beeRules()
	case RoleWorker:
		return workerConfigBlock(opts.name, opts.description, opts.memory) + workerPreamble() + sharedRules() + workerRules()
	default:
		return sharedRules()
	}
}

// EnsureSystemRules writes .openbee.md with the latest system rules
// for the given role, and ensures CLAUDE.md contains the @import reference.
// It does NOT create CLAUDE.md if it doesn't exist.
func EnsureSystemRules(workDir, role string, optFns ...Option) error {
	var opts options
	for _, fn := range optFns {
		fn(&opts)
	}
	// 1. Write .openbee.md (always overwrite)
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
