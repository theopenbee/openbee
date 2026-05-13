package bridge

import ai "github.com/theopenbee/openbee/internal/ai"

const (
	EngineClaude = ai.EngineClaude
	EngineCodex  = ai.EngineCodex
	EnginePi     = ai.EnginePi
	EngineKimi   = ai.EngineKimi
)

// AllEngines returns the canonical engine name list in declaration order,
// independent of which engines are enabled in the running process.
func AllEngines() []string { return ai.AllEngines() }

// AllEngines implements Bridge.AllEngines.
func (b *bridgeImpl) AllEngines() []string { return ai.AllEngines() }

// EnabledEngines returns the enabled engines in canonical order.
func (b *bridgeImpl) EnabledEngines() []string {
	out := make([]string, 0, len(b.engines))
	for _, name := range ai.AllEngines() {
		if _, ok := b.engines[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

// IsEnabled reports whether name is one of the enabled engines.
func (b *bridgeImpl) IsEnabled(name string) bool {
	_, ok := b.engines[name]
	return ok
}
