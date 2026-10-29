package codex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMCPArgs(t *testing.T) {
	assert.Nil(t, mcpArgs(""))
	assert.Equal(t, []string{
		"-c", `mcp_servers.openbee.url="http://127.0.0.1:9000/mcp"`,
		"-c", `mcp_servers.openbee.bearer_token_env_var="OPENBEE_API_KEY"`,
	}, mcpArgs("http://127.0.0.1:9000/mcp"))
}
