package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/domain/pluginmgr"
	"github.com/theopenbee/openbee/internal/plugin"
)

type fakeAvatarService struct {
	packs []pluginmgr.AvatarPack
	files map[string]plugin.Avatar
}

func (f *fakeAvatarService) AvatarPacks() []pluginmgr.AvatarPack { return f.packs }

func (f *fakeAvatarService) AvatarFile(ref string) (plugin.Avatar, bool) {
	a, ok := f.files[ref]
	return a, ok
}

func newAvatarRouter(svc avatarService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAvatarHandler(svc)
	r.GET("/avatars", h.List)
	r.GET("/avatars/:plugin/:name", h.Image)
	return r
}

func TestAvatarHandler_List(t *testing.T) {
	r := newAvatarRouter(&fakeAvatarService{packs: []pluginmgr.AvatarPack{
		{ID: "mfers", Name: plugin.LocalizedText{"en": "mfers"}, Avatars: []string{"mfers/00001", "mfers/00002"}},
	}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/avatars", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"packs":[{"id":"mfers","name":{"en":"mfers"},"avatars":["mfers/00001","mfers/00002"]}]}`, rec.Body.String())

	r = newAvatarRouter(&fakeAvatarService{})
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/avatars", nil))
	assert.JSONEq(t, `{"packs":[]}`, rec.Body.String())
}

func TestAvatarHandler_Image(t *testing.T) {
	path := filepath.Join(t.TempDir(), "00001.webp")
	require.NoError(t, os.WriteFile(path, []byte("RIFF-webp"), 0o644))
	r := newAvatarRouter(&fakeAvatarService{files: map[string]plugin.Avatar{
		"mfers/00001": {Name: "00001", Path: path, ContentType: "image/webp"},
	}})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/avatars/mfers/00001", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "RIFF-webp", rec.Body.String())
	assert.Equal(t, "image/webp", rec.Header().Get("Content-Type"))
	assert.Equal(t, avatarCacheControl, rec.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/avatars/mfers/00002", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "avatar_not_found", body["code"])
}
