package model

// Department represents a department node in a tree hierarchy.
type Department struct {
	ID        string  `json:"id" db:"id"`
	Name      string  `json:"name" db:"name"`
	ParentID  *string `json:"parent_id" db:"parent_id"`
	SortOrder int     `json:"sort_order" db:"sort_order"`
	CreatedAt int64   `json:"created_at" db:"created_at"`
	UpdatedAt int64   `json:"updated_at" db:"updated_at"`
}

// DepartmentTree is a Department with its nested children for tree responses.
type DepartmentTree struct {
	Department
	Children []DepartmentTree `json:"children"`
}

// DepartmentBrief is a lightweight department summary used in list and detail responses.
type DepartmentBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// WorkerDepartment represents the many-to-many link between a Worker and a Department.
type WorkerDepartment struct {
	WorkerID     string `json:"worker_id" db:"worker_id"`
	DepartmentID string `json:"department_id" db:"department_id"`
	CreatedAt    int64  `json:"created_at" db:"created_at"`
}

// ToDepartmentBriefs converts a slice of Department to a slice of DepartmentBrief.
func ToDepartmentBriefs(depts []Department) []DepartmentBrief {
	briefs := make([]DepartmentBrief, 0, len(depts))
	for _, d := range depts {
		briefs = append(briefs, DepartmentBrief{ID: d.ID, Name: d.Name})
	}
	return briefs
}
