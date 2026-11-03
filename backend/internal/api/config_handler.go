package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type configResponse struct {
	Language       string          `json:"language"`
	EnabledEngines []string        `json:"enabled_engines"`
	Engines        []ai.EngineInfo `json:"engines"`
}

type ConfigHandler struct {
	language string
	engines  ai.EngineSource
}

func NewConfigHandler(lang string, src ai.EngineSource) *ConfigHandler {
	return &ConfigHandler{language: lang, engines: src}
}

func (h *ConfigHandler) Get(c *gin.Context) {
	lang := h.language
	if lang == "" {
		lang = "en"
	}
	engines := append([]ai.EngineInfo{}, h.engines.Engines()...)
	ids := make([]string, len(engines))
	for i, e := range engines {
		ids[i] = e.ID
	}
	c.JSON(http.StatusOK, configResponse{
		Language:       lang,
		EnabledEngines: ids,
		Engines:        engines,
	})
}
