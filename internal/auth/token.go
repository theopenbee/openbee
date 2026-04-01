package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	TokenTypeBee    = "bee"
	TokenTypeWorker = "worker"
)

// MCPClaims are the JWT claims embedded in every MCP token.
type MCPClaims struct {
	Type     string `json:"type"`
	WorkerID string `json:"worker_id,omitempty"`
	jwt.RegisteredClaims
}

// GenerateBeeToken creates a signed JWT for the Bee process.
func GenerateBeeToken(secret string, ttl time.Duration) (string, error) {
	return signToken(MCPClaims{
		Type: TokenTypeBee,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}, secret)
}

// GenerateWorkerToken creates a signed JWT for a specific Worker.
func GenerateWorkerToken(secret, workerID string, ttl time.Duration) (string, error) {
	return signToken(MCPClaims{
		Type:     TokenTypeWorker,
		WorkerID: workerID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}, secret)
}

// ValidateToken parses and validates a JWT, returning its claims.
func ValidateToken(tokenStr, secret string) (*MCPClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &MCPClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(*MCPClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}
	return claims, nil
}

func signToken(claims MCPClaims, secret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}
