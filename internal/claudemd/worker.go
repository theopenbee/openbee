package claudemd

import (
	"fmt"
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
	return workerRoleRules() + workerConfigBlock(name, description, memory)
}

func workerRoleRules() string {
	return "你是一个 AI 团队的 Worker，负责执行分配给你的任务。你必须调用 Skill 工具加载 openbee-worker skill，并严格按照该 skill 中的规定执行所有操作。\n"
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

