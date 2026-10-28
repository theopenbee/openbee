package command

import "time"

func SetStatusClockForTest(h *StatusCommandHandler, now func() time.Time) {
	h.now = now
}
