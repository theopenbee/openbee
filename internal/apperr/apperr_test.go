package apperr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/apperr"
)

func TestCodeAndMessage(t *testing.T) {
	err := apperr.New("last_super_admin", "cannot remove last super-admin")
	require.Equal(t, "last_super_admin", apperr.Code(err))
	require.Equal(t, "cannot remove last super-admin", err.Error())
}

func TestParams(t *testing.T) {
	err := apperr.New("env_invalid_scope", "invalid scope").
		WithParams(map[string]any{"scope": "worker"})
	params := apperr.Params(err)
	require.Equal(t, "worker", params["scope"])
}

func TestWrappingPreservesErrorsIs(t *testing.T) {
	sentinel := errors.New("validation error")
	err := apperr.New("env_invalid_scope", "invalid scope").Wrapping(sentinel)
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, "env_invalid_scope", apperr.Code(err))
}

func TestCodeThroughWrappedChain(t *testing.T) {
	coded := apperr.New("worker_not_found", "worker not found")
	wrapped := fmt.Errorf("get worker: %w", coded)
	require.Equal(t, "worker_not_found", apperr.Code(wrapped))
}

func TestCodeAndParamsForPlainError(t *testing.T) {
	err := errors.New("boom")
	require.Empty(t, apperr.Code(err))
	require.Nil(t, apperr.Params(err))
}
