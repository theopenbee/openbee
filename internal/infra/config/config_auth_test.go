package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyDefaults_TokenTTLDefaults(t *testing.T) {
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
