package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// UserAuthStateLoader returns the account status and last password-change time
// for a user id, in a single query.
type UserAuthStateLoader interface {
	UserAuthState(userID string) (status string, passwordChangedAt int64, err error)
}

// AuthMiddleware validates the access token, loads the user id, and rejects
// missing/disabled accounts as well as tokens minted before the user's last
// password change (which forces a re-login after any password change/reset).
func AuthMiddleware(jwtSvc *JWTService, users UserAuthStateLoader) gin.HandlerFunc {
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
		status, passwordChangedAt, err := users.UserAuthState(uid)
		if err != nil || status != model.UserStatusActive {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		if TokenPredatesPasswordChange(issuedAt, passwordChangedAt) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		SetUserID(c, uid)
		c.Next()
	}
}

const ctxKeyAdminToken = "auth.admin_token"

func PluginsAuth(jwtSvc *JWTService, users UserAuthStateLoader, rpcSecret string) gin.HandlerFunc {
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
		c.Set(ctxKeyAdminToken, true)
		c.Next()
	}
}

// RequirePermission aborts with 403 unless the current user holds at least one
// of perms. Multiple perms are OR-ed (any one grants access); pass a single
// perm for the common case.
func RequirePermission(resolver *PermissionResolver, perms ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetBool(ctxKeyAdminToken) {
			c.Next()
			return
		}
		uid := UserID(c)
		if uid == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		ok, err := resolver.HasAnyPermission(uid, perms...)
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
