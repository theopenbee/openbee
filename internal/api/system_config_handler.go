package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	ai "github.com/theopenbee/openbee/internal/ai"
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
	engineCfg *enginecfg.Store
}

func NewSystemConfigHandler(store sysConfigStore, validator engineValidatorForSys, engineCfg *enginecfg.Store) *SystemConfigHandler {
	return &SystemConfigHandler{store: store, validator: validator, engineCfg: engineCfg}
}

// Get returns all known system config keys as a JSON object.
// Missing DB rows are returned as empty strings.
func (h *SystemConfigHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	keys := []string{
		model.SystemConfigKeyDefaultEngine,
		model.SystemConfigKeyEngineExtraArgsGlobal,
		model.SystemConfigKeyEngineExtraArgsBee,
	}
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		cfg, found, err := h.store.Get(ctx, key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if found {
			result[key] = cfg.Value
		} else {
			result[key] = ""
		}
	}
	c.JSON(http.StatusOK, result)
}

type setSystemConfigRequest struct {
	Value string `json:"value"`
}

// Set updates a single system config key.
func (h *SystemConfigHandler) Set(c *gin.Context) {
	key := c.Param("key")
	var req setSystemConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	switch key {
	case model.SystemConfigKeyDefaultEngine:
		if err := h.validator.ValidateEngine(req.Value); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := h.store.Set(c.Request.Context(), key, req.Value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		h.engineCfg.Set(req.Value)

	case model.SystemConfigKeyEngineExtraArgsGlobal, model.SystemConfigKeyEngineExtraArgsBee:
		if req.Value != "" && req.Value != "{}" {
			var raw map[string]string
			if err := json.Unmarshal([]byte(req.Value), &raw); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "value must be a JSON object mapping engine to CLI args string"})
				return
			}
			for engine := range raw {
				if engine == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "engine_extra_args contains an empty engine name"})
					return
				}
				if err := h.validator.ValidateEngine(engine); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
			}
			if _, err := ai.ParseEngineExtraArgs(raw); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		if err := h.store.Set(c.Request.Context(), key, req.Value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown config key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
