package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// JWTMiddleware returns a Gin middleware that validates JWT access tokens.
// Tokens are read from the Authorization header (Bearer scheme) or the "token" query parameter.
func JWTMiddleware(jwtSvc *JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c)
		if token == "" {
			token = c.Query("token")
		}
		if err := jwtSvc.ValidateAccessToken(token); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func extractBearerToken(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	return ""
}
