package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJWT_UserTokenRoundTrip(t *testing.T) {
	svc := NewJWTService("test-secret", time.Hour, 24*time.Hour)
	pair, err := svc.GenerateUserTokenPair("user-123")
	require.NoError(t, err)
	uid, issuedAt, err := svc.ParseAccessToken(pair.AccessToken)
	require.NoError(t, err)
	require.Equal(t, "user-123", uid)
	require.Positive(t, issuedAt)

	ruid, _, err := svc.ParseRefreshToken(pair.RefreshToken)
	require.NoError(t, err)
	require.Equal(t, "user-123", ruid)
}
