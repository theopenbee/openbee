package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/domain/group"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type GroupHandler struct {
	manager    *group.Manager
	groupStore *store.GroupStore
}

func NewGroupHandler(m *group.Manager, gs *store.GroupStore) *GroupHandler {
	return &GroupHandler{manager: m, groupStore: gs}
}

func respondGroupError(c *gin.Context, err error) {
	respondDomainError(c, err, group.ErrNotFound, group.ErrValidation)
}

func (h *GroupHandler) Create(c *gin.Context) {
	var req struct {
		Name             string `json:"name"`
		Description      string `json:"description"`
		Constraints      string `json:"constraints"`
		Engine           string `json:"engine"`
		EngineArgs       string `json:"engine_args"`
		PermissionScopes string `json:"permission_scopes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	g, err := h.manager.CreateGroup(group.CreateGroupParams{
		Name:             req.Name,
		Description:      req.Description,
		Constraints:      req.Constraints,
		Engine:           req.Engine,
		EngineArgs:       req.EngineArgs,
		PermissionScopes: req.PermissionScopes,
	})
	if err != nil {
		respondGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, g)
}

func (h *GroupHandler) List(c *gin.Context) {
	list, err := h.groupStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.Group{}
	}
	c.JSON(http.StatusOK, list)
}

func (h *GroupHandler) Get(c *gin.Context) {
	g, err := h.groupStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	members, err := h.groupStore.ListMembers(g.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if members == nil {
		members = []model.MemberBrief{}
	}
	c.JSON(http.StatusOK, model.GroupWithMembers{Group: g, Members: members})
}

func (h *GroupHandler) Update(c *gin.Context) {
	g, err := h.groupStore.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	var req struct {
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		Constraints      *string `json:"constraints"`
		Engine           *string `json:"engine"`
		EngineArgs       *string `json:"engine_args"`
		PermissionScopes *string `json:"permission_scopes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name != nil {
		g.Name = *req.Name
	}
	if req.Description != nil {
		g.Description = *req.Description
	}
	if req.Constraints != nil {
		g.Constraints = *req.Constraints
	}
	if req.Engine != nil {
		g.Engine = *req.Engine
	}
	if req.EngineArgs != nil {
		g.EngineArgs = *req.EngineArgs
	}
	if req.PermissionScopes != nil {
		g.PermissionScopes = *req.PermissionScopes
	}
	out, err := h.manager.UpdateGroup(g)
	if err != nil {
		respondGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *GroupHandler) Delete(c *gin.Context) {
	deleteWorkDir := c.Query("delete_work_dir") == "true"
	if err := h.manager.DeleteGroup(c.Param("id"), deleteWorkDir); err != nil {
		respondGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *GroupHandler) AddMember(c *gin.Context) {
	var req struct {
		WorkerID string `json:"worker_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.AddMember(c.Param("id"), req.WorkerID); err != nil {
		respondGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "added"})
}

func (h *GroupHandler) RemoveMember(c *gin.Context) {
	if err := h.manager.RemoveMember(c.Param("id"), c.Param("worker_id")); err != nil {
		respondGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

func (h *GroupHandler) ListMembers(c *gin.Context) {
	members, err := h.groupStore.ListMembers(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if members == nil {
		members = []model.MemberBrief{}
	}
	c.JSON(http.StatusOK, members)
}
