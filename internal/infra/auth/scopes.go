package auth

import (
	"fmt"
	"strings"

	"github.com/theopenbee/openbee/internal/infra/utils"
)

const (
	ScopeReadWorkers     = "read:workers"
	ScopeReadDepartments = "read:departments"
	ScopeReadTasks       = "read:tasks"
	ScopeReadMessages    = "read:messages"
	ScopeReadExecutions  = "read:executions"
)

// AllScopes is the complete list of valid permission scope values.
var AllScopes = []string{
	ScopeReadWorkers,
	ScopeReadDepartments,
	ScopeReadTasks,
	ScopeReadMessages,
	ScopeReadExecutions,
}

// validScopeSet is a pre-built lookup set for O(1) scope validation.
var validScopeSet map[string]struct{}

func init() {
	validScopeSet = make(map[string]struct{}, len(AllScopes))
	for _, s := range AllScopes {
		validScopeSet[s] = struct{}{}
	}
}

// ValidatePermissionScopes checks that every scope in the comma-separated string
// is a recognised value. Returns an error listing any invalid scopes found.
func ValidatePermissionScopes(scopes string) error {
	if scopes == "" {
		return nil
	}
	var invalid []string
	for _, s := range utils.SplitAndTrim(scopes) {
		if _, ok := validScopeSet[s]; !ok {
			invalid = append(invalid, s)
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("invalid permission scope(s): %s; allowed values: %s",
			strings.Join(invalid, ", "), strings.Join(AllScopes, ", "))
	}
	return nil
}

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
