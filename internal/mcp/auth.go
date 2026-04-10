package mcp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
)

const (
	CtxKeyTokenType = "mcp.token.type"
	CtxKeyWorkerID  = "mcp.token.worker_id"
	CtxKeyScopes = "mcp.token.scopes"
)

// JWTAuthMiddleware validates the MCP JWT and writes claims to gin.Context.
// Reads the token from X-API-Key header or api_key query param.
// Returns 401 on missing, invalid, or expired token.
func JWTAuthMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := c.GetHeader("X-API-Key")
		if tokenStr == "" {
			tokenStr = c.Query("api_key")
		}
		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		claims, err := auth.ValidateToken(tokenStr, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(CtxKeyTokenType, claims.Type)
		c.Set(CtxKeyWorkerID, claims.WorkerID)
		c.Set(CtxKeyScopes, claims.Scopes)
		c.Next()
	}
}

// RequireBeeOrWorker accepts tokens of either type.
func RequireBeeOrWorker() gin.HandlerFunc {
	return requireTypeIn(auth.TokenTypeBee, auth.TokenTypeWorker)
}

func requireTypeIn(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenType, _ := c.Get(CtxKeyTokenType)
		for _, t := range allowed {
			if tokenType == t {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	}
}
