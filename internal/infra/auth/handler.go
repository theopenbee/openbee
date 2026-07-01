package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// UserAuthenticator authenticates credentials and loads user data.
type UserAuthenticator interface {
	Authenticate(username, password string) (model.UserWithRoles, error)
	GetByID(id string) (model.UserWithRoles, error)
	SetPassword(id, plainPassword string) error
	UserAuthState(userID string) (status string, passwordChangedAt int64, err error)
}

type AuthHandler struct {
	users       UserAuthenticator
	jwtSvc      *JWTService
	rateLimiter *LoginRateLimiter
	resolver    *PermissionResolver
}

func NewAuthHandler(users UserAuthenticator, jwtSvc *JWTService, rateLimiter *LoginRateLimiter, resolver *PermissionResolver) *AuthHandler {
	return &AuthHandler{users: users, jwtSvc: jwtSvc, rateLimiter: rateLimiter, resolver: resolver}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	ip := c.ClientIP()
	if !h.rateLimiter.Allow(ip) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many attempts, please try again later"})
		return
	}
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	user, err := h.users.Authenticate(req.Username, req.Password)
	if err != nil {
		// The token consumed above stays spent, so only failed attempts
		// accumulate toward the limit.
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}
	// A successful login clears the budget: legitimate users never lock
	// themselves (or others behind the same IP) out by logging in.
	h.rateLimiter.Reset(ip)
	pair, err := h.jwtSvc.GenerateUserTokenPair(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	c.JSON(http.StatusOK, pair)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	uid, issuedAt, err := h.jwtSvc.ParseRefreshToken(req.RefreshToken)
	if err != nil || uid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}
	// A refresh token minted before the user's last password change must not mint
	// new access tokens — otherwise a stale session could refresh indefinitely
	// after the password was changed or reset. This path bypasses AuthMiddleware,
	// so the same check is repeated here.
	_, passwordChangedAt, err := h.users.UserAuthState(uid)
	if err != nil || TokenPredatesPasswordChange(issuedAt, passwordChangedAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}
	accessToken, expiresIn, err := h.jwtSvc.GenerateUserAccessToken(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	c.JSON(http.StatusOK, refreshResponse{AccessToken: accessToken, ExpiresIn: expiresIn})
}

type meResponse struct {
	model.UserWithRoles
	Permissions []string `json:"permissions"`
}

// Me returns the current user with resolved permissions.
func (h *AuthHandler) Me(c *gin.Context) {
	uid := UserID(c)
	user, err := h.users.GetByID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	perms, err := h.resolver.PermissionsFor(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve permissions"})
		return
	}
	c.JSON(http.StatusOK, meResponse{UserWithRoles: user, Permissions: perms})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// ChangePassword updates the current user's password after verifying the old one.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	uid := UserID(c)
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.users.GetByID(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if _, err := h.users.Authenticate(user.Username, req.OldPassword); err != nil {
		// 400, not 401: a wrong old password is a validation failure, not an
		// expired session. Returning 401 here would trip the frontend's token
		// interceptor and log the user out. The code lets the UI localize it.
		c.JSON(http.StatusBadRequest, gin.H{"error": "old password is incorrect", "code": "old_password_incorrect"})
		return
	}
	if err := h.users.SetPassword(uid, req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
		return
	}
	c.Status(http.StatusNoContent)
}
