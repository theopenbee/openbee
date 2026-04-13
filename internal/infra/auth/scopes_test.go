package auth_test

import (
	"testing"

	"github.com/theopenbee/openbee/internal/infra/auth"
)

func TestValidatePermissionScopes_Empty_OK(t *testing.T) {
	if err := auth.ValidatePermissionScopes(""); err != nil {
		t.Fatalf("expected no error for empty string, got: %v", err)
	}
}

func TestValidatePermissionScopes_ValidSingle_OK(t *testing.T) {
	if err := auth.ValidatePermissionScopes("read:workers"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePermissionScopes_ValidMultiple_OK(t *testing.T) {
	if err := auth.ValidatePermissionScopes("read:workers,read:tasks,read:messages"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePermissionScopes_ValidWithSpaces_OK(t *testing.T) {
	if err := auth.ValidatePermissionScopes(" read:workers , read:departments "); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePermissionScopes_InvalidScope_Error(t *testing.T) {
	if err := auth.ValidatePermissionScopes("write:workers"); err == nil {
		t.Fatal("expected error for invalid scope, got nil")
	}
}

func TestValidatePermissionScopes_MixedValidInvalid_Error(t *testing.T) {
	if err := auth.ValidatePermissionScopes("read:workers,bogus:scope"); err == nil {
		t.Fatal("expected error for mixed valid/invalid scopes, got nil")
	}
}
