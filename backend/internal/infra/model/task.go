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

	TaskStatusActive = TaskStatusPending + "," + TaskStatusRunning
)

var TaskStatuses = []string{
	TaskStatusPending,
	TaskStatusRunning,
	TaskStatusCompleted,
	TaskStatusFailed,
	TaskStatusCancelled,
}

// Task represents a unit of work created by bee and dispatched to a worker.
type Task struct {
	ID          string
	MessageID   string
	SessionKey  string
	Platform    string
	WorkerID    string
	Instruction string
	Type        string // TaskTypeImmediate | TaskTypeCountdown | TaskTypeScheduled
	Status      string // TaskStatus*
	ScheduledAt *int64 // ms; countdown: absolute trigger time
	CronExpr    string
	NextRunAt   *int64 // ms; scheduled tasks only
	CreatedAt   int64
	UpdatedAt   int64
}
