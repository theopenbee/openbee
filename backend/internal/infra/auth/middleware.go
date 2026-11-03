package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type PasswordChangeLoader interface {
	PasswordChangedAt(userID string) (int64, error)
}

// AuthMiddleware validates the access token, loads the user id, and rejects
// missing accounts as well as tokens minted before the user's last
// password change (which forces a re-login after any password change).
func AuthMiddleware(jwtSvc *JWTService, users PasswordChangeLoader) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c)
		if token == "" {
			token = c.Query("token")
		}
		uid, issuedAt, err := jwtSvc.ParseAccessToken(token)
		if err != nil || uid == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		passwordChangedAt, err := users.PasswordChangedAt(uid)
		if err != nil || TokenPredatesPasswordChange(issuedAt, passwordChangedAt) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		SetUserID(c, uid)
		c.Next()
	}
}

func PluginsAuth(jwtSvc *JWTService, users PasswordChangeLoader, rpcSecret string) gin.HandlerFunc {
	userAuth := AuthMiddleware(jwtSvc, users)
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			userAuth(c)
			return
		}
		claims, err := ValidateToken(key, rpcSecret)
		if err != nil || claims.Type != TokenTypeAdmin {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func extractBearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return authHeader[7:]
	}
	return ""
}
