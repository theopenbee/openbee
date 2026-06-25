package api

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/buildinfo"
)

// VersionHandler serves build and runtime metadata for the dashboard's
// system-info panel.
type VersionHandler struct {
	info buildinfo.Info
}

// NewVersionHandler constructs a VersionHandler from the build metadata.
func NewVersionHandler(info buildinfo.Info) *VersionHandler {
	return &VersionHandler{info: info}
}

type versionResponse struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Get returns the running build's version, commit, build date and Go runtime
// platform.
func (h *VersionHandler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, versionResponse{
		Version:   h.info.Version,
		Commit:    h.info.Commit,
		Date:      h.info.Date,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	})
}
