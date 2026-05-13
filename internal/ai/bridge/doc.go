// Package bridge is the business-facing front for the internal AI engine
// subsystem. Business code (worker, bee, task, tokenstat, store, config,
// cmd) must only depend on this package; it must not import internal/ai
// directly.
//
// Design spec: docs/superpowers/specs/2026-05-12-ai-bridge-design.md
package bridge
