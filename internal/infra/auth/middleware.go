package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// UserStatusLoader returns the account status for a user id.
type UserStatusLoader interface {
	UserStatus(userID string) (string, error)
}

// AuthMiddleware validates the access token, loads the user id, and rejects
// missing/disabled accounts.
func AuthMiddleware(jwtSvc *JWTService, users UserStatusLoader) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c)
		if token == "" {
			token = c.Query("token")
		}
		uid, err := jwtSvc.ParseAccessToken(token)
		if err != nil || uid == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		status, err := users.UserStatus(uid)
		if err != nil || status != model.UserStatusActive {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		SetUserID(c, uid)
		c.Next()
	}
}

// RequirePermission aborts with 403 unless the current user holds perm.
func RequirePermission(resolver *PermissionResolver, perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := UserID(c)
		if uid == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		ok, err := resolver.HasPermission(uid, perm)
		if err != nil || !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
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
