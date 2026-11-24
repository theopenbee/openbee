package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/theopenbee/openbee/internal/domain/pluginmgr"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/plugin"
)

type pluginLister interface {
	List(ctx context.Context) ([]pluginmgr.Status, error)
}

type OnboardingHandler struct {
	store   sysConfigStore
	plugins pluginLister
}

func NewOnboardingHandler(store sysConfigStore, plugins pluginLister) *OnboardingHandler {
	return &OnboardingHandler{store: store, plugins: plugins}
}

func (h *OnboardingHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	skipped, _, err := h.store.Get(ctx, model.SystemConfigKeyOnboardingSkipped)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if skipped.Value == "true" {
		c.JSON(http.StatusOK, gin.H{"pending": false})
		return
	}
	installed, err := h.plugins.List(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, p := range installed {
		if p.Type == plugin.TypeEngine {
			c.JSON(http.StatusOK, gin.H{"pending": false})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"pending": true})
}

func (h *OnboardingHandler) Skip(c *gin.Context) {
	if err := h.store.Set(c.Request.Context(), model.SystemConfigKeyOnboardingSkipped, "true"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
