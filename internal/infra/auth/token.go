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

// RPCClaims are the JWT claims embedded in every RPC token.
type RPCClaims struct {
	Type     string   `json:"type"`
	WorkerID string   `json:"worker_id,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
	jwt.RegisteredClaims
}

func GenerateBeeToken(secret string, ttl time.Duration) (string, error) {
	return signToken(RPCClaims{
		Type: TokenTypeBee,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}, secret)
}

func GenerateWorkerToken(secret, workerID string, scopes []string, ttl time.Duration) (string, error) {
	return signToken(RPCClaims{
		Type:     TokenTypeWorker,
		WorkerID: workerID,
		Scopes:   scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}, secret)
}

func ValidateToken(tokenStr, secret string) (*RPCClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &RPCClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(*RPCClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}
	return claims, nil
}

func signToken(claims RPCClaims, secret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}
