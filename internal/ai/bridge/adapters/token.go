package adapters

import (
	"time"

	bridge "github.com/theopenbee/openbee/internal/ai/bridge"
	"github.com/theopenbee/openbee/internal/infra/auth"
)

type tokenIssuer struct {
	secret string
	ttl    time.Duration
}

// NewTokenIssuer wraps auth.GenerateWorkerToken / GenerateBeeToken.
func NewTokenIssuer(secret string, ttl time.Duration) bridge.TokenIssuer {
	return tokenIssuer{secret: secret, ttl: ttl}
}

func (t tokenIssuer) WorkerToken(workerID string, scopes []string) (string, error) {
	return auth.GenerateWorkerToken(t.secret, workerID, scopes, t.ttl)
}
func (t tokenIssuer) BeeToken() (string, error) {
	return auth.GenerateBeeToken(t.secret, t.ttl)
}
