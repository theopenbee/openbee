package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoginRateLimiter_BlocksAfterBurst(t *testing.T) {
	l := NewLoginRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		require.True(t, l.Allow("1.2.3.4"), "attempt %d should be allowed", i+1)
	}
	require.False(t, l.Allow("1.2.3.4"), "attempt beyond burst should be blocked")
}

func TestLoginRateLimiter_ResetClears(t *testing.T) {
	l := NewLoginRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		l.Allow("1.2.3.4")
	}
	require.False(t, l.Allow("1.2.3.4"), "precondition: should be blocked before reset")
	l.Reset("1.2.3.4")
	require.True(t, l.Allow("1.2.3.4"), "after reset the IP should be allowed again")
}

func TestLoginRateLimiter_PerIPIsolation(t *testing.T) {
	l := NewLoginRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		l.Allow("1.1.1.1")
	}
	require.True(t, l.Allow("2.2.2.2"), "a different IP must not be affected by another IP's attempts")
}
