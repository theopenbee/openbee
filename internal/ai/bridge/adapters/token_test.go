package adapters

import (
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/auth"
)

func TestTokenIssuerMintsWorkerAndBeeTokens(t *testing.T) {
	secret := "test-secret"
	iss := NewTokenIssuer(secret, time.Hour)

	wt, err := iss.WorkerToken("worker-id-1", []string{"scope-a"})
	if err != nil {
		t.Fatalf("WorkerToken: %v", err)
	}
	if wt == "" {
		t.Fatal("expected non-empty worker token")
	}
	if _, err := auth.ValidateToken(wt, secret); err != nil {
		t.Fatalf("ValidateToken worker: %v", err)
	}

	bt, err := iss.BeeToken()
	if err != nil {
		t.Fatalf("BeeToken: %v", err)
	}
	if _, err := auth.ValidateToken(bt, secret); err != nil {
		t.Fatalf("ValidateToken bee: %v", err)
	}
}
