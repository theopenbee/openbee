package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type configResponse struct {
	Language       string   `json:"language"`
	EnabledEngines []string `json:"enabled_engines"`
	DefaultEngine  string   `json:"default_engine"`
}

type ConfigHandler struct {
	language       string
	enabledEngines []string
	defaultEngine  string
}

func NewConfigHandler(lang string, enabledEngines []string, defaultEngine string) *ConfigHandler {
	return &ConfigHandler{language: lang, enabledEngines: enabledEngines, defaultEngine: defaultEngine}
}

func (h *ConfigHandler) Get(c *gin.Context) {
	lang := h.language
	if lang == "" {
		lang = "en"
	}
	c.JSON(http.StatusOK, configResponse{
		Language:       lang,
		EnabledEngines: h.enabledEngines,
		DefaultEngine:  h.defaultEngine,
	})
}
