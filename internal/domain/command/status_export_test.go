package command

import "time"

// SetStatusClockForTest overrides the time source used by /status. Available
// to external _test packages so production callers cannot mutate the clock.
func SetStatusClockForTest(h *StatusCommandHandler, now func() time.Time) {
	h.now = now
}
