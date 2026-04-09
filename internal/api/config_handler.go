package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type configResponse struct {
	Language string `json:"language"`
}

// ConfigHandler handles HTTP requests for server configuration.
type ConfigHandler struct {
	language string
}

func NewConfigHandler(lang string) *ConfigHandler {
	return &ConfigHandler{language: lang}
}

func (h *ConfigHandler) Get(c *gin.Context) {
	lang := h.language
	if lang == "" {
		lang = "en"
	}
	c.JSON(http.StatusOK, configResponse{Language: lang})
}
