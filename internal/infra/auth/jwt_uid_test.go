package auth

import (
	"testing"
	"time"
)

func TestJWT_UserTokenRoundTrip(t *testing.T) {
	svc := NewJWTService("test-secret", time.Hour, 24*time.Hour)
	pair, err := svc.GenerateUserTokenPair("user-123")
	if err != nil {
		t.Fatalf("GenerateUserTokenPair: %v", err)
	}
	uid, err := svc.ParseAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if uid != "user-123" {
		t.Fatalf("expected uid user-123, got %s", uid)
	}

	ruid, err := svc.ParseRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ParseRefreshToken: %v", err)
	}
	if ruid != "user-123" {
		t.Fatalf("expected refresh uid user-123, got %s", ruid)
	}
}
