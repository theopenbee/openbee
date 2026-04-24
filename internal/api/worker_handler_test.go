package api

import (
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func TestToWorkerResponse_ParsesEngineExtraArgs(t *testing.T) {
	resp, err := toWorkerResponse(model.Worker{
		ID:              "w1",
		Name:            "worker",
		EngineExtraArgs: `{"claude":"--model claude-sonnet-4-5"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.EngineExtraArgs["claude"]; got != "--model claude-sonnet-4-5" {
		t.Fatalf("got %q, want %q", got, "--model claude-sonnet-4-5")
	}
}

func TestToWorkerResponse_EmptyEngineExtraArgsUsesEmptyMap(t *testing.T) {
	resp, err := toWorkerResponse(model.Worker{ID: "w1", Name: "worker"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.EngineExtraArgs == nil {
		t.Fatal("expected empty map, got nil")
	}
	if len(resp.EngineExtraArgs) != 0 {
		t.Fatalf("expected empty map, got %v", resp.EngineExtraArgs)
	}
}

func TestToWorkerResponse_InvalidEngineExtraArgsJSON(t *testing.T) {
	_, err := toWorkerResponse(model.Worker{
		ID:              "w1",
		Name:            "worker",
		EngineExtraArgs: `{"claude":`,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
