package auth_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/auth"
)

func TestValidatePermissionScopes_Empty_OK(t *testing.T) {
	require.NoError(t, auth.ValidatePermissionScopes(""))
}

func TestValidatePermissionScopes_ValidSingle_OK(t *testing.T) {
	require.NoError(t, auth.ValidatePermissionScopes("read:workers"))
}

func TestValidatePermissionScopes_ValidMultiple_OK(t *testing.T) {
	require.NoError(t, auth.ValidatePermissionScopes("read:workers,read:tasks,read:messages"))
}

func TestValidatePermissionScopes_ValidWithSpaces_OK(t *testing.T) {
	require.NoError(t, auth.ValidatePermissionScopes(" read:workers , read:departments "))
}

func TestValidatePermissionScopes_InvalidScope_Error(t *testing.T) {
	require.Error(t, auth.ValidatePermissionScopes("write:workers"))
}

func TestValidatePermissionScopes_MixedValidInvalid_Error(t *testing.T) {
	require.Error(t, auth.ValidatePermissionScopes("read:workers,bogus:scope"))
}
