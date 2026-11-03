package bee

import "time"

const (
	// PollInterval is the fixed tick rate for both the Feeder and TaskScheduler.
	PollInterval = 500 * time.Millisecond

	// QueueWarnThreshold is the number of unprocessed messages that triggers a warning.
	QueueWarnThreshold = 20

)
