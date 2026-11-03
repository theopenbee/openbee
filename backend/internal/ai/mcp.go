package ai

import (
	"os"
	"strings"
)

const (
	MCPPath       = "/mcp"
	MCPServerName = "openbee"
	APIKeyEnv     = "OPENBEE_API_KEY"
)

func MCPURL() string {
	base := strings.TrimSuffix(os.Getenv("OPENBEE_URL"), "/")
	if base == "" {
		return ""
	}
	return base + MCPPath
}
