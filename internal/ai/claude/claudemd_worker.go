package claude

import ai "github.com/theopenbee/openbee/internal/ai"

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
	return ai.WorkerRules(name, description, memory)
}
