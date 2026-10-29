package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMCPURL(t *testing.T) {
	t.Setenv("OPENBEE_URL", "")
	assert.Empty(t, MCPURL())
	t.Setenv("OPENBEE_URL", "http://localhost:8080/")
	assert.Equal(t, "http://localhost:8080/mcp", MCPURL())
}

func TestBuildRunEnvBypassesProxyForMCP(t *testing.T) {
	t.Setenv("OPENBEE_URL", "http://127.0.0.1:8080")
	env := BuildRunEnv([]string{"NO_PROXY=example.com"}, nil, "tok")
	assert.Equal(t, "example.com,127.0.0.1", lastEnvValue(env, "NO_PROXY"))
	assert.Equal(t, "127.0.0.1", lastEnvValue(env, "no_proxy"))
	assert.Equal(t, "tok", lastEnvValue(env, "OPENBEE_API_KEY"))

	t.Setenv("OPENBEE_URL", "")
	assert.Empty(t, lastEnvValue(BuildRunEnv(nil, nil, "tok"), "NO_PROXY"))
}
