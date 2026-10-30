package configcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/config"
)

func TestRenderConfig_OmitsWebCredentials(t *testing.T) {
	vals := defaultValues()
	vals.Language = "en"
	vals.AuthJWTSecret = "jwt-secret"
	vals.RPCTokenSecret = "rpc-secret"
	vals.ServerEnvSecret = config.GenerateRandomSecret()

	rendered, err := renderConfig(vals)
	require.NoError(t, err)
	assert.NotContains(t, string(rendered), "username:")
	assert.NotContains(t, string(rendered), "password:")

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, rendered, 0o600))
	loaded := loadExistingConfig(path)
	require.NotNil(t, loaded)
	assert.Equal(t, "jwt-secret", loaded.AuthJWTSecret)
	assert.Equal(t, "2h0m0s", loaded.AuthAccessTTL)
}
