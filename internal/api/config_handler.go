package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type configResponse struct {
	Language string `json:"language"`
}

func (s *Server) getConfig(c *gin.Context) {
	lang := s.Language
	if lang == "" {
		lang = "en"
	}
	c.JSON(http.StatusOK, configResponse{Language: lang})
}
