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
	Constraints         string       `json:"constraints" db:"constraints"`
	WorkDir             string       `json:"work_dir" db:"work_dir"`
	Engine              string       `json:"engine" db:"engine"`
	EngineArgs          string       `json:"engine_args" db:"engine_args"`
	Status              WorkerStatus `json:"status" db:"status"`
	PermissionScopes    string       `json:"permission_scopes" db:"permission_scopes"`
	CreatedAt           int64        `json:"created_at" db:"created_at"`
	UpdatedAt           int64        `json:"updated_at" db:"updated_at"`
}

// WorkerWithDepartments is a Worker with its associated department summaries.
type WorkerWithDepartments struct {
	Worker
	Departments []DepartmentBrief `json:"departments"`
}
