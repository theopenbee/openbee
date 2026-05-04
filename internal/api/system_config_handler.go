package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/domain/linearcfg"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type sysConfigStore interface {
	Get(ctx context.Context, key string) (model.SystemConfig, bool, error)
	Set(ctx context.Context, key, value string) error
}

type engineValidatorForSys interface {
	ValidateEngine(name string) error
	ValidateEngineArgs(raw map[string]string) error
}

// SystemConfigHandler serves GET /system-configs and PUT /system-configs/:key.
type SystemConfigHandler struct {
	store     sysConfigStore
	validator engineValidatorForSys
	engineCfg *enginecfg.Store
	linearCfg *linearcfg.Store
}

func NewSystemConfigHandler(store sysConfigStore, validator engineValidatorForSys, engineCfg *enginecfg.Store, linearCfg *linearcfg.Store) *SystemConfigHandler {
	return &SystemConfigHandler{store: store, validator: validator, engineCfg: engineCfg, linearCfg: linearCfg}
}

// Get returns all known system config keys as a JSON object.
// Missing DB rows are returned as empty strings.
func (h *SystemConfigHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	keys := []string{
		model.SystemConfigKeyDefaultEngine,
		model.SystemConfigKeyEngineArgsGlobal,
		model.SystemConfigKeyEngineArgsBee,
		model.SystemConfigKeyLinearProjects,
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

// parseLinearProjects validates that value is either empty or a JSON array of
// strings, and returns the trimmed non-empty entries.
func parseLinearProjects(value string) ([]string, error) {
	if value == "" || value == "[]" {
		return nil, nil
	}
	var raw []string
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, errInvalidLinearProjects
	}
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

var errInvalidLinearProjects = errLinearProjects("value must be a JSON array of project name strings")

type errLinearProjects string

func (e errLinearProjects) Error() string { return string(e) }

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

	case model.SystemConfigKeyEngineArgsGlobal, model.SystemConfigKeyEngineArgsBee:
		if req.Value != "" && req.Value != "{}" {
			var raw map[string]string
			if err := json.Unmarshal([]byte(req.Value), &raw); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "value must be a JSON object mapping engine to CLI args string"})
				return
			}
			if err := h.validator.ValidateEngineArgs(raw); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		if err := h.store.Set(c.Request.Context(), key, req.Value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

	case model.SystemConfigKeyLinearProjects:
		projects, err := parseLinearProjects(req.Value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := h.store.Set(c.Request.Context(), key, req.Value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if h.linearCfg != nil {
			h.linearCfg.Set(projects)
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown config key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
