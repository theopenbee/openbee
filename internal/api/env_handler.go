package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/domain/env"
)

type EnvHandler struct {
	svc *env.Service
}

func NewEnvHandler(svc *env.Service) *EnvHandler {
	return &EnvHandler{svc: svc}
}

type createEnvRequest struct {
	Scope   string `json:"scope"    binding:"required,oneof=global bee department worker"`
	ScopeID string `json:"scope_id"`
	Key     string `json:"key"      binding:"required"`
	Value   string `json:"value"    binding:"required"`
}

type updateEnvRequest struct {
	Value string `json:"value" binding:"required"`
}

func (h *EnvHandler) List(c *gin.Context) {
	scope := c.Query("scope")
	if scope == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope query parameter is required"})
		return
	}
	scopeID := c.Query("scope_id")

	var scopeIDPtr *string
	if scopeID != "" {
		scopeIDPtr = &scopeID
	}

	configs, err := h.svc.List(scope, scopeIDPtr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, configs)
}

func (h *EnvHandler) Create(c *gin.Context) {
	var req createEnvRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg, err := h.svc.Create(req.Scope, req.ScopeID, req.Key, req.Value)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, cfg)
}

func (h *EnvHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req updateEnvRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.UpdateValue(id, req.Value); err != nil {
		if errors.Is(err, env.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *EnvHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.svc.Delete(id); err != nil {
		if errors.Is(err, env.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
