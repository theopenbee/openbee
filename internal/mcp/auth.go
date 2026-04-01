package mcp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	CtxKeyTokenType = "mcp.token.type"
	CtxKeyWorkerID  = "mcp.token.worker_id"
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
		claims, err := ValidateToken(tokenStr, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(CtxKeyTokenType, claims.Type)
		c.Set(CtxKeyWorkerID, claims.WorkerID)
		c.Next()
	}
}

// RequireBee aborts with 403 if the token type is not "bee".
func RequireBee() gin.HandlerFunc {
	return requireType(TokenTypeBee)
}

// RequireWorker aborts with 403 if the token type is not "worker".
func RequireWorker() gin.HandlerFunc {
	return requireType(TokenTypeWorker)
}

// RequireBeeOrWorker accepts tokens of either type.
func RequireBeeOrWorker() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenType, _ := c.Get(CtxKeyTokenType)
		if tokenType != TokenTypeBee && tokenType != TokenTypeWorker {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

func requireType(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenType, _ := c.Get(CtxKeyTokenType)
		if tokenType != expected {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}
