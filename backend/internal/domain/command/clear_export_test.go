package command

import "time"

func SetClearClockForTest(h *ClearCommandHandler, now func() time.Time) {
	h.now = now
}
