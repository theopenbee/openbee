package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type versionResponse struct {
	Version string `json:"version"`
	Bundled bool   `json:"bundled"`
}

type VersionHandler struct {
	version string
	bundled bool
}

func NewVersionHandler(version string, bundled bool) *VersionHandler {
	return &VersionHandler{version: version, bundled: bundled}
}

func (h *VersionHandler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, versionResponse{Version: h.version, Bundled: h.bundled})
}
