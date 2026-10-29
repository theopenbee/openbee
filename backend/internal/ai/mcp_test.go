package ai

import (
	"runtime"
	"strings"
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
	env := BuildRunEnv([]string{"NO_PROXY=example.com, 127.0.0.1", "no_proxy=corp.internal"}, nil, "tok")
	want := "example.com,127.0.0.1,corp.internal"
	assert.Equal(t, want, lastValue(env, "NO_PROXY"))
	if runtime.GOOS != "windows" {
		assert.Equal(t, want, lastValue(env, "no_proxy"))
	}
	assert.Equal(t, "tok", lastValue(env, "OPENBEE_API_KEY"))

	env = BuildRunEnv(nil, nil, "tok")
	assert.Equal(t, "127.0.0.1", lastValue(env, "NO_PROXY"))

	t.Setenv("OPENBEE_URL", "")
	assert.Empty(t, lastValue(BuildRunEnv(nil, nil, "tok"), "NO_PROXY"))
}

func lastValue(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], key+"="); ok {
			return v
		}
	}
	return ""
}
