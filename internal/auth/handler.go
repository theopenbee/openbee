package auth

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	username    string
	password    string
	jwtSvc      *JWTService
	rateLimiter *LoginRateLimiter
}

func NewAuthHandler(username, password string, jwtSvc *JWTService, rateLimiter *LoginRateLimiter) *AuthHandler {
	return &AuthHandler{
		username:    username,
		password:    password,
		jwtSvc:      jwtSvc,
		rateLimiter: rateLimiter,
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	if !h.rateLimiter.Allow(c.ClientIP()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many attempts, please try again later"})
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	usernameMatch := subtle.ConstantTimeCompare([]byte(req.Username), []byte(h.username)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(req.Password), []byte(h.password)) == 1
	if !usernameMatch || !passwordMatch {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	pair, err := h.jwtSvc.GenerateTokenPair()
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

	if err := h.jwtSvc.ValidateRefreshToken(req.RefreshToken); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}

	accessToken, expiresIn, err := h.jwtSvc.GenerateAccessToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, refreshResponse{
		AccessToken: accessToken,
		ExpiresIn:   expiresIn,
	})
}

func (h *AuthHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"auth_required": true})
}

func StatusDisabled() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"auth_required": false})
	}
}
