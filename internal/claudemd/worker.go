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
	return workerConfigBlock(name, description, memory)
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

