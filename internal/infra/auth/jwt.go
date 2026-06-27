package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
)

type Claims struct {
	Type string `json:"type"`
	UID  string `json:"uid,omitempty"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds until access_token expires
}

type JWTService struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewJWTService(secret string, accessTTL, refreshTTL time.Duration) *JWTService {
	return &JWTService{
		secret:          []byte(secret),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	}
}

func (s *JWTService) GenerateTokenPair() (*TokenPair, error) {
	now := time.Now()

	accessToken, err := s.signToken(tokenTypeAccess, now, s.accessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	refreshToken, err := s.signToken(tokenTypeRefresh, now, s.refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.accessTokenTTL.Seconds()),
	}, nil
}

func (s *JWTService) GenerateAccessToken() (string, int64, error) {
	token, err := s.signToken(tokenTypeAccess, time.Now(), s.accessTokenTTL)
	if err != nil {
		return "", 0, fmt.Errorf("sign access token: %w", err)
	}
	return token, int64(s.accessTokenTTL.Seconds()), nil
}

func (s *JWTService) ValidateAccessToken(tokenStr string) error {
	return s.validateToken(tokenStr, tokenTypeAccess)
}

func (s *JWTService) ValidateRefreshToken(tokenStr string) error {
	return s.validateToken(tokenStr, tokenTypeRefresh)
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

// GenerateUserTokenPair issues an access+refresh pair bound to a user id.
func (s *JWTService) GenerateUserTokenPair(userID string) (*TokenPair, error) {
	now := time.Now()
	access, err := s.signUserToken(tokenTypeAccess, userID, now, s.accessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}
	refresh, err := s.signUserToken(tokenTypeRefresh, userID, now, s.refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.accessTokenTTL.Seconds()),
	}, nil
}

// GenerateUserAccessToken issues a fresh access token for a user id.
func (s *JWTService) GenerateUserAccessToken(userID string) (string, int64, error) {
	token, err := s.signUserToken(tokenTypeAccess, userID, time.Now(), s.accessTokenTTL)
	if err != nil {
		return "", 0, fmt.Errorf("sign access token: %w", err)
	}
	return token, int64(s.accessTokenTTL.Seconds()), nil
}

// ParseAccessToken validates an access token and returns its uid.
func (s *JWTService) ParseAccessToken(tokenStr string) (string, error) {
	return s.parseUserToken(tokenStr, tokenTypeAccess)
}

// ParseRefreshToken validates a refresh token and returns its uid.
func (s *JWTService) ParseRefreshToken(tokenStr string) (string, error) {
	return s.parseUserToken(tokenStr, tokenTypeRefresh)
}

func (s *JWTService) signUserToken(tokenType, userID string, now time.Time, ttl time.Duration) (string, error) {
	claims := Claims{
		Type: tokenType,
		UID:  userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *JWTService) parseUserToken(tokenStr, expectedType string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || claims.Type != expectedType {
		return "", fmt.Errorf("invalid token type: expected %s", expectedType)
	}
	return claims.UID, nil
}
