package mcp

import (
	"time"

	"github.com/theopenbee/openbee/internal/auth"
)

// TokenTypeBee and TokenTypeWorker are re-exported for backwards compatibility.
const (
	TokenTypeBee    = auth.TokenTypeBee
	TokenTypeWorker = auth.TokenTypeWorker
)

// MCPClaims is re-exported from auth.
type MCPClaims = auth.MCPClaims

// GenerateBeeToken creates a signed JWT for the Bee process.
func GenerateBeeToken(secret string, ttl time.Duration) (string, error) {
	return auth.GenerateBeeToken(secret, ttl)
}

// GenerateWorkerToken creates a signed JWT for a specific Worker.
func GenerateWorkerToken(secret, workerID string, ttl time.Duration) (string, error) {
	return auth.GenerateWorkerToken(secret, workerID, ttl)
}

// ValidateToken parses and validates a JWT, returning its claims.
func ValidateToken(tokenStr, secret string) (*MCPClaims, error) {
	return auth.ValidateToken(tokenStr, secret)
}
