package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type configResponse struct {
	Language       string   `json:"language"`
	EnabledEngines []string `json:"enabled_engines"`
}

type ConfigHandler struct {
	language       string
	enabledEngines []string
}

func NewConfigHandler(lang string, enabledEngines []string) *ConfigHandler {
	return &ConfigHandler{language: lang, enabledEngines: enabledEngines}
}

func (h *ConfigHandler) Get(c *gin.Context) {
	lang := h.language
	if lang == "" {
		lang = "en"
	}
	c.JSON(http.StatusOK, configResponse{
		Language:       lang,
		EnabledEngines: h.enabledEngines,
	})
}
