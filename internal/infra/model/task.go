package model

const (
	TaskTypeImmediate = "immediate"
	TaskTypeCountdown = "countdown"
	TaskTypeScheduled = "scheduled"

	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"
)

// Task represents a unit of work created by bee and dispatched to a worker.
type Task struct {
	ID              string
	MessageID       string
	WorkerID        string
	Instruction     string
	Type            string // TaskTypeImmediate | TaskTypeCountdown | TaskTypeScheduled
	Status          string // TaskStatus*
	ScheduledAt     *int64 // ms; countdown: absolute trigger time
	CronExpr        string
	NextRunAt       *int64 // ms; scheduled tasks only
	ExecutionID string
	CreatedAt       int64
	UpdatedAt       int64
}

// ClaimedTask is a Task joined with data from its originating platform_messages row,
// needed by the TaskScheduler to build a DispatchTask.
type ClaimedTask struct {
	Task
	MessageSessionKey string
	MessagePlatform   string
}
