package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"go.uber.org/zap"
)

type SetupHandler struct {
	users           *store.UserStore
	jwtSvc          *auth.JWTService
	claimLegacyChat func(ctx context.Context, userID string) error
}

func NewSetupHandler(users *store.UserStore, jwtSvc *auth.JWTService, claimLegacyChat func(ctx context.Context, userID string) error) *SetupHandler {
	return &SetupHandler{users: users, jwtSvc: jwtSvc, claimLegacyChat: claimLegacyChat}
}

// Status reports whether the system already has at least one user.
func (h *SetupHandler) Status(c *gin.Context) {
	n, err := h.users.Count()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"initialized": n > 0})
}

type setupRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required,min=6"`
	DisplayName string `json:"display_name"`
}

// Create provisions the first super-admin. Only works while no users exist.
func (h *SetupHandler) Create(c *gin.Context) {
	n, err := h.users.Count()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	if n > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "system already initialized"})
		return
	}
	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	user, err := h.users.CreateFirst(req.Username, req.Password, req.DisplayName, []string{model.RoleIDSuperAdmin})
	if errors.Is(err, store.ErrAlreadyInitialized) {
		c.JSON(http.StatusConflict, gin.H{"error": "system already initialized"})
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	if err := h.claimLegacyChat(c.Request.Context(), user.ID); err != nil {
		logger.Error("claim legacy local chat", zap.String("user_id", user.ID), zap.Error(err))
	}
	pair, err := h.jwtSvc.GenerateUserTokenPair(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	c.JSON(http.StatusOK, pair)
}
