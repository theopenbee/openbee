package auth_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/auth"
)

func TestGenerateToken_ValidAndParseable(t *testing.T) {
	cases := []struct {
		name         string
		gen          func() (string, error)
		wantType     string
		wantWorkerID string
		wantScopes   []string
	}{
		{
			name:     "Bee",
			gen:      func() (string, error) { return auth.GenerateBeeToken("test-secret", time.Hour) },
			wantType: auth.TokenTypeBee,
		},
		{
			name:         "Worker",
			gen:          func() (string, error) { return auth.GenerateWorkerToken("test-secret", "worker-abc", nil, time.Hour) },
			wantType:     auth.TokenTypeWorker,
			wantWorkerID: "worker-abc",
		},
		{
			name: "WorkerWithScopes",
			gen: func() (string, error) {
				return auth.GenerateWorkerToken("test-secret", "worker-abc", []string{"read:workers", "read:tasks"}, time.Hour)
			},
			wantType:     auth.TokenTypeWorker,
			wantWorkerID: "worker-abc",
			wantScopes:   []string{"read:workers", "read:tasks"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok, err := tc.gen()
			require.NoError(t, err)
			claims, err := auth.ValidateToken(tok, "test-secret")
			require.NoError(t, err)
			assert.Equal(t, tc.wantType, claims.Type)
			assert.Equal(t, tc.wantWorkerID, claims.WorkerID)
			if tc.wantScopes != nil {
				assert.Equal(t, tc.wantScopes, claims.Scopes)
			}
		})
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	tok, _ := auth.GenerateBeeToken("secret-a", time.Hour)
	_, err := auth.ValidateToken(tok, "secret-b")
	assert.Error(t, err)
}

func TestValidateToken_Expired(t *testing.T) {
	tok, _ := auth.GenerateBeeToken("secret", -time.Second)
	_, err := auth.ValidateToken(tok, "secret")
	assert.Error(t, err)
}

func TestValidateToken_MalformedString(t *testing.T) {
	_, err := auth.ValidateToken("not-a-jwt", "secret")
	assert.Error(t, err)
}

func TestGenerateWorkerToken_NoScopes(t *testing.T) {
	tok, err := auth.GenerateWorkerToken("test-secret", "worker-abc", nil, time.Hour)
	require.NoError(t, err)
	claims, err := auth.ValidateToken(tok, "test-secret")
	require.NoError(t, err)
	assert.Empty(t, claims.Scopes)
}
