package claude

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
	return "You are a Worker in an AI team, responsible for executing tasks assigned to you. You must invoke the Skill tool to load the openbee-worker skill and strictly follow all rules defined in that skill.\n"
}

func workerConfigBlock(name, description, memory string) string {
	var block string

	if name != "" {
		block += fmt.Sprintf("Name: %s\n", name)
	}
	if description != "" {
		block += fmt.Sprintf("Description: %s\n", description)
	}

	if memory != "" {
		if block != "" {
			block += "\n"
		}
		block += fmt.Sprintf("## Memory Constraints\n%s\n", memory)
	}

	if block == "" {
		return ""
	}

	return block + "\n"
}
