package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/domain/pluginmgr"
	"github.com/theopenbee/openbee/internal/infra/model"
)

type fakePluginLister struct {
	plugins []pluginmgr.Status
	err     error
}

func (f *fakePluginLister) List(context.Context) ([]pluginmgr.Status, error) {
	return f.plugins, f.err
}

func onboardingRouter(h *OnboardingHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/onboarding", h.Get)
	r.POST("/onboarding/skip", h.Skip)
	return r
}

func getOnboarding(t *testing.T, h *OnboardingHandler) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	onboardingRouter(h).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/onboarding", nil))
	var body map[string]any
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	}
	return w.Code, body
}

func TestOnboardingPendingWithoutEngines(t *testing.T) {
	h := NewOnboardingHandler(&fakeSysConfigStore{}, &fakePluginLister{plugins: []pluginmgr.Status{{ID: "telegram", Type: "channel"}}})
	code, body := getOnboarding(t, h)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, body["pending"])
}

func TestOnboardingNotPendingWithInstalledEngine(t *testing.T) {
	h := NewOnboardingHandler(&fakeSysConfigStore{}, &fakePluginLister{plugins: []pluginmgr.Status{{ID: "claude", Type: "engine", Enabled: false}}})
	code, body := getOnboarding(t, h)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, body["pending"])
}

func TestOnboardingNotPendingAfterSkip(t *testing.T) {
	store := &fakeSysConfigStore{}
	h := NewOnboardingHandler(store, &fakePluginLister{})

	w := httptest.NewRecorder()
	onboardingRouter(h).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/onboarding/skip", nil))
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "true", store.vals[model.SystemConfigKeyOnboardingSkipped])

	code, body := getOnboarding(t, h)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, body["pending"])
}

func TestOnboardingListError(t *testing.T) {
	h := NewOnboardingHandler(&fakeSysConfigStore{}, &fakePluginLister{err: errors.New("boom")})
	code, _ := getOnboarding(t, h)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func TestOnboardingStoreError(t *testing.T) {
	h := NewOnboardingHandler(&fakeSysConfigStore{err: errors.New("boom")}, &fakePluginLister{})
	code, _ := getOnboarding(t, h)
	assert.Equal(t, http.StatusInternalServerError, code)

	w := httptest.NewRecorder()
	onboardingRouter(h).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/onboarding/skip", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
