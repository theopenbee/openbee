package bridge

import (
	"errors"
	"fmt"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// ErrEngineNotEnabled is returned by ValidateEngine when a non-empty
// engine name is not present in the enabled set.
var ErrEngineNotEnabled = errors.New("bridge: engine not enabled")

// ValidateEngineArgs reports whether s tokenises cleanly under the shared
// CLI lexer (single/double quotes, backslash escape).
func ValidateEngineArgs(s string) error { return ai.ValidateExtraArgs(s) }

// ValidateEngineArgs implements Bridge.ValidateEngineArgs.
func (b *bridgeImpl) ValidateEngineArgs(line string) error { return ai.ValidateExtraArgs(line) }

// ValidateEngine accepts the empty string (caller will fall back to the
// default engine) and otherwise requires the name to be enabled.
func (b *bridgeImpl) ValidateEngine(name string) error {
	if name == "" {
		return nil
	}
	if _, ok := b.engines[name]; !ok {
		return fmt.Errorf("%w: %q", ErrEngineNotEnabled, name)
	}
	return nil
}
