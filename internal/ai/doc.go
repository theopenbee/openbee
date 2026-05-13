// Package ai is the low-level AI engine subsystem. It is internal to the
// AI module and is not intended to be imported by business code.
//
// Business code (worker, bee, task, tokenstat, store, config, cmd) must
// depend on internal/ai/bridge instead. Only the bridge package itself,
// the engine implementations under internal/ai/engine/*, and the
// composition root in cmd/openbee/internal/app may import internal/ai.
package ai
