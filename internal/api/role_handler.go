package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type RoleHandler struct {
	roles    *store.RoleStore
	resolver *auth.PermissionResolver
}

func NewRoleHandler(roles *store.RoleStore, resolver *auth.PermissionResolver) *RoleHandler {
	return &RoleHandler{roles: roles, resolver: resolver}
}

// Catalog returns the grouped permission catalog.
func (h *RoleHandler) Catalog(c *gin.Context) {
	c.JSON(http.StatusOK, auth.PermissionCatalog())
}

func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.roles.List()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	if roles == nil {
		roles = []model.RoleWithPermissions{}
	}
	c.JSON(http.StatusOK, roles)
}

type roleRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (h *RoleHandler) Create(c *gin.Context) {
	var req roleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if err := validatePermissions(req.Permissions); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	role, err := h.roles.Create(model.Role{Name: req.Name, Description: req.Description}, req.Permissions)
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusCreated, role)
}

func (h *RoleHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req roleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if err := validatePermissions(req.Permissions); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if err := h.roles.Update(
		model.Role{ID: id, Name: req.Name, Description: req.Description}, req.Permissions,
	); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	h.resolver.InvalidateAll() // role permission change affects every member
	c.Status(http.StatusNoContent)
}

func (h *RoleHandler) Delete(c *gin.Context) {
	if err := h.roles.Delete(c.Param("id")); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	h.resolver.InvalidateAll()
	c.Status(http.StatusNoContent)
}

// validatePermissions rejects unknown permission keys (wildcard not assignable here).
func validatePermissions(perms []string) error {
	for _, p := range perms {
		if !auth.IsAssignablePermission(p) {
			return fmt.Errorf("unknown permission: %s", p)
		}
	}
	return nil
}
