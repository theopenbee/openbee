package auth_test

import (
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/auth"
)

func TestGenerateBeeToken_ValidAndParseable(t *testing.T) {
	tok, err := auth.GenerateBeeToken("test-secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateBeeToken: %v", err)
	}
	claims, err := auth.ValidateToken(tok, "test-secret")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Type != auth.TokenTypeBee {
		t.Errorf("Type: want %q got %q", auth.TokenTypeBee, claims.Type)
	}
	if claims.WorkerID != "" {
		t.Errorf("WorkerID: want empty got %q", claims.WorkerID)
	}
}

func TestGenerateWorkerToken_ValidAndParseable(t *testing.T) {
	tok, err := auth.GenerateWorkerToken("test-secret", "worker-abc", time.Hour)
	if err != nil {
		t.Fatalf("GenerateWorkerToken: %v", err)
	}
	claims, err := auth.ValidateToken(tok, "test-secret")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Type != auth.TokenTypeWorker {
		t.Errorf("Type: want %q got %q", auth.TokenTypeWorker, claims.Type)
	}
	if claims.WorkerID != "worker-abc" {
		t.Errorf("WorkerID: want worker-abc got %q", claims.WorkerID)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	tok, _ := auth.GenerateBeeToken("secret-a", time.Hour)
	_, err := auth.ValidateToken(tok, "secret-b")
	if err == nil {
		t.Error("expected error for wrong secret, got nil")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	tok, _ := auth.GenerateBeeToken("secret", -time.Second)
	_, err := auth.ValidateToken(tok, "secret")
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

func TestValidateToken_MalformedString(t *testing.T) {
	_, err := auth.ValidateToken("not-a-jwt", "secret")
	if err == nil {
		t.Error("expected error for malformed token, got nil")
	}
}
