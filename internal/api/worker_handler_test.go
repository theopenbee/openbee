package api

import (
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func TestToWorkerResponse_ParsesEngineArgs(t *testing.T) {
	resp, err := toWorkerResponse(model.Worker{
		ID:         "w1",
		Name:       "worker",
		EngineArgs: `{"claude":"--model claude-sonnet-4-5"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.EngineArgs["claude"]; got != "--model claude-sonnet-4-5" {
		t.Fatalf("got %q, want %q", got, "--model claude-sonnet-4-5")
	}
}

func TestToWorkerResponse_EmptyEngineArgsUsesEmptyMap(t *testing.T) {
	resp, err := toWorkerResponse(model.Worker{ID: "w1", Name: "worker"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.EngineArgs == nil {
		t.Fatal("expected empty map, got nil")
	}
	if len(resp.EngineArgs) != 0 {
		t.Fatalf("expected empty map, got %v", resp.EngineArgs)
	}
}

func TestToWorkerResponse_InvalidEngineArgsJSON(t *testing.T) {
	_, err := toWorkerResponse(model.Worker{
		ID:         "w1",
		Name:       "worker",
		EngineArgs: `{"claude":`,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
