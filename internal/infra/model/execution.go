package model

type ExecutionStatus string

const (
	ExecStatusPending   ExecutionStatus = "pending"
	ExecStatusRunning   ExecutionStatus = "running"
	ExecStatusCompleted ExecutionStatus = "completed"
	ExecStatusFailed    ExecutionStatus = "failed"
)

type WorkerExecution struct {
	ID           string          `json:"id" db:"id"`
	TaskID       string          `json:"task_id,omitempty" db:"task_id"`
	WorkerID     *string         `json:"worker_id,omitempty" db:"worker_id"`
	WorkerName   string          `json:"worker_name,omitempty" db:"-"`
	SessionID    string          `json:"session_id" db:"session_id"`
	Engine       string          `json:"engine,omitempty" db:"engine"`
	TriggerInput string          `json:"trigger_input,omitempty" db:"trigger_input"`
	Status       ExecutionStatus `json:"status" db:"status"`
	Result       string          `json:"result,omitempty" db:"result"`
	LogPath      string          `json:"log_path,omitempty" db:"log_path"`
	AIProcessPID int             `json:"ai_process_pid,omitempty" db:"ai_process_pid"`
	StartedAt    *int64          `json:"started_at,omitempty" db:"started_at"`
	CompletedAt  *int64          `json:"completed_at,omitempty" db:"completed_at"`
}

// FailureInfo carries context for a task failure notification sent to the user.
type FailureInfo struct {
	Reason     string // raw error (exec.Result or err.Error())
	WorkerName string // worker or bee name for identification
}
