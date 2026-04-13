package auth

import "github.com/theopenbee/openbee/internal/infra/utils"

const (
	ScopeReadWorkers     = "read:workers"
	ScopeReadDepartments = "read:departments"
	ScopeReadTasks       = "read:tasks"
	ScopeReadMessages    = "read:messages"
	ScopeReadExecutions  = "read:executions"
)

// ToolScopeMap maps tool names to the scope required for worker-token callers.
// Tools in this map require the listed scope when called with a worker token.
// Tools absent from this map follow existing access rules (unchanged behavior).
// Must not be mutated after package initialization.
var ToolScopeMap = map[string]string{
	utils.ListWorkers:     ScopeReadWorkers,
	utils.GetWorker:       ScopeReadWorkers,
	utils.GetWorkerStatus: ScopeReadWorkers,
	utils.ListDepartments: ScopeReadDepartments,
	utils.GetDepartment:   ScopeReadDepartments,
	utils.ListTasks:       ScopeReadTasks,
	utils.ListMessages:    ScopeReadMessages,
	utils.ListExecutions:  ScopeReadExecutions,
}
