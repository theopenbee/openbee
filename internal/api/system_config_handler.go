package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type sysConfigStore interface {
	Get(ctx context.Context, key string) (model.SystemConfig, bool, error)
	Set(ctx context.Context, key, value string) error
}

type engineValidatorForSys interface {
	ValidateEngine(name string) error
}

// SystemConfigHandler serves GET /system-configs and PUT /system-configs/:key.
type SystemConfigHandler struct {
	store     sysConfigStore
	validator engineValidatorForSys
}

func NewSystemConfigHandler(store sysConfigStore, validator engineValidatorForSys) *SystemConfigHandler {
	return &SystemConfigHandler{store: store, validator: validator}
}

// Get returns all known system config keys as a JSON object.
// Missing DB rows are returned as empty strings.
func (h *SystemConfigHandler) Get(c *gin.Context) {
	cfg, found, err := h.store.Get(c.Request.Context(), model.SystemConfigKeyDefaultEngine)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	value := ""
	if found {
		value = cfg.Value
	}
	c.JSON(http.StatusOK, gin.H{model.SystemConfigKeyDefaultEngine: value})
}

type setSystemConfigRequest struct {
	Value string `json:"value"`
}

// Set updates a single system config key.
// Only "default_engine" is accepted; unknown keys return 400.
func (h *SystemConfigHandler) Set(c *gin.Context) {
	key := c.Param("key")
	if key != model.SystemConfigKeyDefaultEngine {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown config key"})
		return
	}
	var req setSystemConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.validator.ValidateEngine(req.Value); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.Set(c.Request.Context(), key, req.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	enginecfg.Set(req.Value)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
