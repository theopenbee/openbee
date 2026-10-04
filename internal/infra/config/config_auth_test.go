package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Web login uses DB users now, so Auth.Password is normally empty. Token TTL
// defaults must still apply, otherwise access tokens are minted already-expired.
func TestApplyDefaults_TokenTTLDefaultsWithEmptyPassword(t *testing.T) {
	cfg := &Config{}
	require.NoError(t, applyDefaults(cfg))
	assert.Equal(t, 2*time.Hour, cfg.Server.Auth.AccessTokenTTL)
	assert.Equal(t, 7*24*time.Hour, cfg.Server.Auth.RefreshTokenTTL)
}

func TestApplyDefaults_TokenTTLRespectsExplicitValues(t *testing.T) {
	cfg := &Config{}
	cfg.Server.Auth.AccessTokenTTL = 30 * time.Minute
	cfg.Server.Auth.RefreshTokenTTL = 48 * time.Hour
	require.NoError(t, applyDefaults(cfg))
	assert.Equal(t, 30*time.Minute, cfg.Server.Auth.AccessTokenTTL, "AccessTokenTTL overwritten")
	assert.Equal(t, 48*time.Hour, cfg.Server.Auth.RefreshTokenTTL, "RefreshTokenTTL overwritten")
}
