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

// ParseAccessToken validates an access token and returns its uid and the
// issued-at time in milliseconds (floored to the second, matching the JWT `iat`
// claim). Callers compare issuedAt against the user's password_changed_at to
// reject tokens minted before a password change.
func (s *JWTService) ParseAccessToken(tokenStr string) (uid string, issuedAt int64, err error) {
	return s.parseUserToken(tokenStr, tokenTypeAccess)
}

// ParseRefreshToken validates a refresh token and returns its uid and issued-at
// time in milliseconds. See ParseAccessToken for the issuedAt semantics.
func (s *JWTService) ParseRefreshToken(tokenStr string) (uid string, issuedAt int64, err error) {
	return s.parseUserToken(tokenStr, tokenTypeRefresh)
}

// TokenPredatesPasswordChange reports whether a token issued at issuedAt was
// minted before the user's last password change (passwordChangedAt), in which
// case it must be rejected to force a re-login. Both the access-token middleware
// and the refresh path apply this check, since the refresh endpoint bypasses the
// middleware.
func TokenPredatesPasswordChange(issuedAt, passwordChangedAt int64) bool {
	return issuedAt < passwordChangedAt
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

func (s *JWTService) parseUserToken(tokenStr, expectedType string) (string, int64, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return "", 0, fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || claims.Type != expectedType {
		return "", 0, fmt.Errorf("invalid token type: expected %s", expectedType)
	}
	var issuedAt int64
	if claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.UnixMilli()
	}
	return claims.UID, issuedAt, nil
}
