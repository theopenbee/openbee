package mcp

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIKeyMiddleware returns a Gin middleware that requires X-API-Key header or api_key query param to match key.
func APIKeyMiddleware(key string) gin.HandlerFunc {
	return AnyKeyMiddleware(key)
}

// AnyKeyMiddleware returns a Gin middleware that accepts any of the provided keys.
// Keys are converted to []byte once at creation time. Per-request comparison always
// exhausts all keys to avoid timing oracles that leak which key matched.
func AnyKeyMiddleware(keys ...string) gin.HandlerFunc {
	keyBytes := make([][]byte, len(keys))
	for i, k := range keys {
		keyBytes[i] = []byte(k)
	}
	return func(c *gin.Context) {
		candidate := c.GetHeader("X-API-Key")
		if candidate == "" {
			candidate = c.Query("api_key")
		}
		cb := []byte(candidate)
		matched := false
		for _, kb := range keyBytes {
			if subtle.ConstantTimeCompare(cb, kb) == 1 {
				matched = true
			}
		}
		if matched {
			c.Next()
		} else {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		}
	}
}
