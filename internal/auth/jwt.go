package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims is the JWT payload used for both access and refresh tokens.
type Claims struct {
	Type string `json:"type"` // "access" or "refresh"
	jwt.RegisteredClaims
}

// TokenPair holds the tokens returned after a successful login.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds until access_token expires
}

// JWTService signs and validates JWT tokens using HMAC-SHA256.
type JWTService struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// NewJWTService creates a new JWTService.
func NewJWTService(secret string, accessTTL, refreshTTL time.Duration) *JWTService {
	return &JWTService{
		secret:          []byte(secret),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	}
}

// GenerateTokenPair creates both an access token and a refresh token.
func (s *JWTService) GenerateTokenPair() (*TokenPair, error) {
	now := time.Now()

	accessToken, err := s.signToken("access", now, s.accessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	refreshToken, err := s.signToken("refresh", now, s.refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.accessTokenTTL.Seconds()),
	}, nil
}

// GenerateAccessToken creates only an access token (used during refresh).
func (s *JWTService) GenerateAccessToken() (string, int64, error) {
	token, err := s.signToken("access", time.Now(), s.accessTokenTTL)
	if err != nil {
		return "", 0, fmt.Errorf("sign access token: %w", err)
	}
	return token, int64(s.accessTokenTTL.Seconds()), nil
}

// ValidateAccessToken parses and validates an access token string.
func (s *JWTService) ValidateAccessToken(tokenStr string) error {
	return s.validateToken(tokenStr, "access")
}

// ValidateRefreshToken parses and validates a refresh token string.
func (s *JWTService) ValidateRefreshToken(tokenStr string) error {
	return s.validateToken(tokenStr, "refresh")
}

func (s *JWTService) signToken(tokenType string, now time.Time, ttl time.Duration) (string, error) {
	claims := Claims{
		Type: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *JWTService) validateToken(tokenStr, expectedType string) error {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || claims.Type != expectedType {
		return fmt.Errorf("invalid token type: expected %s", expectedType)
	}
	return nil
}
