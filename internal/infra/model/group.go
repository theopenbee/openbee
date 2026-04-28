package model

type GroupStatus = WorkerStatus // reuse idle/working/error vocabulary

// Group is an Agent that coordinates Worker members on a root task.
type Group struct {
	ID               string      `json:"id" db:"id"`
	Name             string      `json:"name" db:"name"`
	Description      string      `json:"description" db:"description"`
	Constraints      string      `json:"constraints" db:"constraints"`
	WorkDir          string      `json:"work_dir" db:"work_dir"`
	Engine           string      `json:"engine" db:"engine"`
	EngineArgs       string      `json:"engine_args" db:"engine_args"`
	Status           GroupStatus `json:"status" db:"status"`
	PermissionScopes string      `json:"permission_scopes" db:"permission_scopes"`
	CreatedAt        int64       `json:"created_at" db:"created_at"`
	UpdatedAt        int64       `json:"updated_at" db:"updated_at"`
}

// WorkerGroup is the many-to-many membership row.
type WorkerGroup struct {
	WorkerID  string `json:"worker_id" db:"worker_id"`
	GroupID   string `json:"group_id" db:"group_id"`
	Role      string `json:"role" db:"role"` // reserved for future use; default "member"
	CreatedAt int64  `json:"created_at" db:"created_at"`
}

// GroupBrief is a lightweight Group summary used in list responses or membership reports.
type GroupBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// MemberBrief is a lightweight Worker summary used inside a group response.
type MemberBrief struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GroupWithMembers extends Group with its current member roster.
type GroupWithMembers struct {
	Group
	Members []MemberBrief `json:"members"`
}
