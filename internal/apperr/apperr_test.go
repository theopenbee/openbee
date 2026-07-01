package apperr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/theopenbee/openbee/internal/apperr"
)

func TestCodeAndMessage(t *testing.T) {
	err := apperr.New("last_super_admin", "cannot remove last super-admin")
	if got := apperr.Code(err); got != "last_super_admin" {
		t.Fatalf("Code = %q, want %q", got, "last_super_admin")
	}
	if got := err.Error(); got != "cannot remove last super-admin" {
		t.Fatalf("Error = %q", got)
	}
}

func TestParams(t *testing.T) {
	err := apperr.New("env_invalid_scope", "invalid scope").
		WithParams(map[string]any{"scope": "worker"})
	params := apperr.Params(err)
	if params["scope"] != "worker" {
		t.Fatalf("Params = %v", params)
	}
}

func TestWrappingPreservesErrorsIs(t *testing.T) {
	sentinel := errors.New("validation error")
	err := apperr.New("env_invalid_scope", "invalid scope").Wrapping(sentinel)
	if !errors.Is(err, sentinel) {
		t.Fatal("expected errors.Is to match the wrapped sentinel")
	}
	if got := apperr.Code(err); got != "env_invalid_scope" {
		t.Fatalf("Code = %q", got)
	}
}

func TestCodeThroughWrappedChain(t *testing.T) {
	coded := apperr.New("worker_not_found", "worker not found")
	wrapped := fmt.Errorf("get worker: %w", coded)
	if got := apperr.Code(wrapped); got != "worker_not_found" {
		t.Fatalf("Code through chain = %q", got)
	}
}

func TestCodeAndParamsForPlainError(t *testing.T) {
	err := errors.New("boom")
	if got := apperr.Code(err); got != "" {
		t.Fatalf("Code for plain error = %q, want empty", got)
	}
	if got := apperr.Params(err); got != nil {
		t.Fatalf("Params for plain error = %v, want nil", got)
	}
}
