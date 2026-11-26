package api

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/domain/pluginmgr"
	"github.com/theopenbee/openbee/internal/plugin"
)

const avatarCacheControl = "private, max-age=86400"

type avatarService interface {
	AvatarPacks() []pluginmgr.AvatarPack
	AvatarFile(ref string) (plugin.Avatar, bool)
}

type AvatarHandler struct {
	svc avatarService
}

func NewAvatarHandler(svc avatarService) *AvatarHandler {
	return &AvatarHandler{svc: svc}
}

type avatarPackView struct {
	ID      string               `json:"id"`
	Name    plugin.LocalizedText `json:"name"`
	Avatars []string             `json:"avatars"`
}

type avatarListView struct {
	Packs []avatarPackView `json:"packs"`
}

func (h *AvatarHandler) List(c *gin.Context) {
	packs := h.svc.AvatarPacks()
	out := avatarListView{Packs: make([]avatarPackView, 0, len(packs))}
	for _, p := range packs {
		out.Packs = append(out.Packs, avatarPackView{ID: p.ID, Name: p.Name, Avatars: p.Avatars})
	}
	c.JSON(http.StatusOK, out)
}

func (h *AvatarHandler) Image(c *gin.Context) {
	a, ok := h.svc.AvatarFile(plugin.AvatarRef(c.Param("plugin"), c.Param("name")))
	if ok {
		info, err := os.Lstat(a.Path)
		ok = err == nil && info.Mode().IsRegular()
	}
	if !ok {
		respondError(c, http.StatusNotFound, apperr.New("avatar_not_found", "avatar not found"))
		return
	}
	c.Header("Cache-Control", avatarCacheControl)
	c.Header("Content-Type", a.ContentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(a.Path)
}
