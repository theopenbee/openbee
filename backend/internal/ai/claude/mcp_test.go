package claude

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMCPConfig(t *testing.T) {
	assert.JSONEq(t, `{"mcpServers":{"openbee":{"type":"http","url":"http://127.0.0.1:9000/mcp","headers":{"Authorization":"Bearer ${OPENBEE_API_KEY}"},"alwaysLoad":true}}}`,
		mcpConfig("http://127.0.0.1:9000/mcp"))
}
