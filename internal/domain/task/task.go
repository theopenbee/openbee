package task

import "github.com/theopenbee/openbee/internal/platform"

// DispatchTask is the unit of work sent to the TaskDispatcher by the TaskScheduler.
type DispatchTask struct {
	TaskID          string                  // task record ID
	WorkerID        string
	SessionKey      string                  // original message session_key
	Instruction     string
	ReplyTo         platform.InboundMessage // platform info for result delivery
	TaskType        string                  // "immediate"|"countdown"|"scheduled"
	MessageID string // originating platform_messages.id (for session lookup)
}
