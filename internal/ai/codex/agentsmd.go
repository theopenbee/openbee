package codex

import ai "github.com/theopenbee/openbee/internal/ai"

func setupWorkspace(workDir string, role ai.Role, opts ai.WorkspaceOptions) error {
	return ai.SetupWorkspace(workDir, role, opts)
}
