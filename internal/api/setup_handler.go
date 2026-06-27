package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type SetupHandler struct {
	users  *store.UserStore
	jwtSvc *auth.JWTService
}

func NewSetupHandler(users *store.UserStore, jwtSvc *auth.JWTService) *SetupHandler {
	return &SetupHandler{users: users, jwtSvc: jwtSvc}
}

// Status reports whether the system already has at least one user.
func (h *SetupHandler) Status(c *gin.Context) {
	n, err := h.users.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "system already initialized"})
		return
	}
	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.users.Create(req.Username, req.Password, req.DisplayName, "", []string{model.RoleIDSuperAdmin})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pair, err := h.jwtSvc.GenerateUserTokenPair(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	c.JSON(http.StatusOK, pair)
}
