package config

import (
	"testing"
	"time"
)

// Web login uses DB users now, so Auth.Password is normally empty. Token TTL
// defaults must still apply, otherwise access tokens are minted already-expired.
func TestApplyDefaults_TokenTTLDefaultsWithEmptyPassword(t *testing.T) {
	cfg := &Config{}
	if err := applyDefaults(cfg); err != nil {
		t.Fatalf("applyDefaults: %v", err)
	}
	if cfg.Server.Auth.AccessTokenTTL != 2*time.Hour {
		t.Errorf("AccessTokenTTL = %v, want 2h", cfg.Server.Auth.AccessTokenTTL)
	}
	if cfg.Server.Auth.RefreshTokenTTL != 7*24*time.Hour {
		t.Errorf("RefreshTokenTTL = %v, want 168h", cfg.Server.Auth.RefreshTokenTTL)
	}
}

func TestApplyDefaults_TokenTTLRespectsExplicitValues(t *testing.T) {
	cfg := &Config{}
	cfg.Server.Auth.AccessTokenTTL = 30 * time.Minute
	cfg.Server.Auth.RefreshTokenTTL = 48 * time.Hour
	if err := applyDefaults(cfg); err != nil {
		t.Fatalf("applyDefaults: %v", err)
	}
	if cfg.Server.Auth.AccessTokenTTL != 30*time.Minute {
		t.Errorf("AccessTokenTTL overwritten: %v", cfg.Server.Auth.AccessTokenTTL)
	}
	if cfg.Server.Auth.RefreshTokenTTL != 48*time.Hour {
		t.Errorf("RefreshTokenTTL overwritten: %v", cfg.Server.Auth.RefreshTokenTTL)
	}
}
