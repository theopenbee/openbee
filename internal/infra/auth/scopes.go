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

var AllScopes = []string{
	ScopeReadWorkers,
	ScopeReadDepartments,
	ScopeReadTasks,
	ScopeReadMessages,
	ScopeReadExecutions,
}

var validScopeSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(AllScopes))
	for _, s := range AllScopes {
		m[s] = struct{}{}
	}
	return m
}()

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

// ToolScopeMap maps tool names to their required worker-token scope.
// Tools absent from this map follow existing access rules. Must not be mutated after init.
var ToolScopeMap = map[string]string{
	utils.ListWorkers:          ScopeReadWorkers,
	utils.GetWorker:            ScopeReadWorkers,
	utils.GetWorkerStatus:      ScopeReadWorkers,
	utils.ListDepartments:      ScopeReadDepartments,
	utils.GetDepartment:        ScopeReadDepartments,
	utils.ListTasks:            ScopeReadTasks,
	utils.ListMessages:         ScopeReadMessages,
	utils.ListOutboundMessages: ScopeReadMessages,
	utils.ListExecutions:       ScopeReadExecutions,
}
