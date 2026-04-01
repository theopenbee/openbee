package mcp_test

import (
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/mcp"
)

func TestGenerateBeeToken_ValidAndParseable(t *testing.T) {
	tok, err := mcp.GenerateBeeToken("test-secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateBeeToken: %v", err)
	}
	claims, err := mcp.ValidateToken(tok, "test-secret")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Type != mcp.TokenTypeBee {
		t.Errorf("Type: want %q got %q", mcp.TokenTypeBee, claims.Type)
	}
	if claims.WorkerID != "" {
		t.Errorf("WorkerID: want empty got %q", claims.WorkerID)
	}
}

func TestGenerateWorkerToken_ValidAndParseable(t *testing.T) {
	tok, err := mcp.GenerateWorkerToken("test-secret", "worker-abc", time.Hour)
	if err != nil {
		t.Fatalf("GenerateWorkerToken: %v", err)
	}
	claims, err := mcp.ValidateToken(tok, "test-secret")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Type != mcp.TokenTypeWorker {
		t.Errorf("Type: want %q got %q", mcp.TokenTypeWorker, claims.Type)
	}
	if claims.WorkerID != "worker-abc" {
		t.Errorf("WorkerID: want worker-abc got %q", claims.WorkerID)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	tok, _ := mcp.GenerateBeeToken("secret-a", time.Hour)
	_, err := mcp.ValidateToken(tok, "secret-b")
	if err == nil {
		t.Error("expected error for wrong secret, got nil")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	tok, _ := mcp.GenerateBeeToken("secret", -time.Second)
	_, err := mcp.ValidateToken(tok, "secret")
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

func TestValidateToken_MalformedString(t *testing.T) {
	_, err := mcp.ValidateToken("not-a-jwt", "secret")
	if err == nil {
		t.Error("expected error for malformed token, got nil")
	}
}
