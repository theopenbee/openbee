package command

import "time"

// SetClearClockForTest overrides the time source used by /clear. Available
// to external _test packages so production callers cannot mutate the clock.
func SetClearClockForTest(h *ClearCommandHandler, now func() time.Time) {
	h.now = now
}
