package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/infra/model"
)

func TestToWorkerResponse_ParsesEngineArgs(t *testing.T) {
	resp, err := toWorkerResponse(model.Worker{
		ID:         "w1",
		Name:       "worker",
		EngineArgs: `{"claude":"--model claude-sonnet-4-5"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "--model claude-sonnet-4-5", resp.EngineArgs["claude"])
}

func TestToWorkerResponse_EmptyEngineArgsUsesEmptyMap(t *testing.T) {
	resp, err := toWorkerResponse(model.Worker{ID: "w1", Name: "worker"})
	require.NoError(t, err)
	require.NotNil(t, resp.EngineArgs)
	require.Empty(t, resp.EngineArgs)
}

func TestToWorkerResponse_InvalidEngineArgsJSON(t *testing.T) {
	_, err := toWorkerResponse(model.Worker{
		ID:         "w1",
		Name:       "worker",
		EngineArgs: `{"claude":`,
	})
	require.Error(t, err)
}
