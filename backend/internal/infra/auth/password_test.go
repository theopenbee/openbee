package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("s3cret")
	require.NoError(t, err)
	require.NotEqual(t, "s3cret", hash, "hash must not equal plaintext")
	require.NotEmpty(t, hash, "hash must not be empty")
	require.True(t, CheckPassword(hash, "s3cret"), "expected correct password to match")
	require.False(t, CheckPassword(hash, "wrong"), "expected wrong password to fail")
}
