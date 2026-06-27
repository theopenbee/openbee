package auth

import "github.com/gin-gonic/gin"

const ctxUserIDKey = "auth_user_id"

// SetUserID stores the authenticated user id on the request context.
func SetUserID(c *gin.Context, uid string) { c.Set(ctxUserIDKey, uid) }

// UserID returns the authenticated user id, or "" if unauthenticated.
func UserID(c *gin.Context) string {
	v, ok := c.Get(ctxUserIDKey)
	if !ok {
		return ""
	}
	uid, _ := v.(string)
	return uid
}
