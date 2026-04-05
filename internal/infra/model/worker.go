package model

type WorkerStatus string

const (
	WorkerStatusIdle    WorkerStatus = "idle"
	WorkerStatusWorking WorkerStatus = "working"
	WorkerStatusError   WorkerStatus = "error"
)

type Worker struct {
	ID                  string       `json:"id" db:"id"`
	Name                string       `json:"name" db:"name"`
	Description         string       `json:"description" db:"description"`
	Memory              string       `json:"memory" db:"memory"`
	WorkDir             string       `json:"work_dir" db:"work_dir"`
	Status              WorkerStatus `json:"status" db:"status"`
	CreatedAt           int64        `json:"created_at" db:"created_at"`
	UpdatedAt           int64        `json:"updated_at" db:"updated_at"`
}
