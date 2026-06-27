package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

var errLastSuperAdmin = errors.New("cannot remove or disable the last active super-admin")

type UserHandler struct {
	users    *store.UserStore
	resolver *auth.PermissionResolver
}

func NewUserHandler(users *store.UserStore, resolver *auth.PermissionResolver) *UserHandler {
	return &UserHandler{users: users, resolver: resolver}
}

func (h *UserHandler) List(c *gin.Context) {
	users, err := h.users.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if users == nil {
		users = []model.UserWithRoles{}
	}
	c.JSON(http.StatusOK, users)
}

type createUserRequest struct {
	Username    string   `json:"username" binding:"required"`
	Password    string   `json:"password" binding:"required,min=6"`
	DisplayName string   `json:"display_name"`
	RoleIDs     []string `json:"role_ids"`
}

func (h *UserHandler) Create(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.users.Create(req.Username, req.Password, req.DisplayName, auth.UserID(c), req.RoleIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, user)
}

type setRolesRequest struct {
	RoleIDs []string `json:"role_ids"`
}

func (h *UserHandler) SetRoles(c *gin.Context) {
	id := c.Param("id")
	var req setRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.guardLastSuperAdmin(c, id, req.RoleIDs); err != nil {
		return
	}
	if err := h.users.SetRoles(id, req.RoleIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.resolver.Invalidate(id)
	c.Status(http.StatusNoContent)
}

type setStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active disabled"`
}

func (h *UserHandler) SetStatus(c *gin.Context) {
	id := c.Param("id")
	var req setStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status == model.UserStatusDisabled {
		if err := h.guardLastSuperAdmin(c, id, nil); err != nil {
			return
		}
	}
	if err := h.users.SetStatus(id, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.resolver.Invalidate(id)
	c.Status(http.StatusNoContent)
}

type resetPasswordRequest struct {
	Password string `json:"password" binding:"required,min=6"`
}

func (h *UserHandler) ResetPassword(c *gin.Context) {
	id := c.Param("id")
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.users.SetPassword(id, req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *UserHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.guardLastSuperAdmin(c, id, nil); err != nil {
		return
	}
	if err := h.users.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.resolver.Invalidate(id)
	c.Status(http.StatusNoContent)
}

// guardLastSuperAdmin blocks removing the super-admin role from / disabling /
// deleting the last remaining active super-admin. newRoleIDs is the prospective
// role set for a SetRoles call, or nil for disable/delete.
func (h *UserHandler) guardLastSuperAdmin(c *gin.Context, userID string, newRoleIDs []string) error {
	user, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return err
	}
	isSuper := false
	for _, r := range user.Roles {
		if r.ID == model.RoleIDSuperAdmin {
			isSuper = true
		}
	}
	if !isSuper {
		return nil
	}
	// If newRoleIDs still grants super-admin, the change is safe.
	for _, rid := range newRoleIDs {
		if rid == model.RoleIDSuperAdmin {
			return nil
		}
	}
	count, err := h.users.CountActiveSuperAdmins()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return err
	}
	if count <= 1 {
		err := errLastSuperAdmin
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}
	return nil
}
