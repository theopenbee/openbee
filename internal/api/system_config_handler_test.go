package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/domain/enginecfg"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// --- fakes ---

type fakeSysConfigStore struct {
	vals map[string]string
	err  error
}

func (f *fakeSysConfigStore) Get(_ context.Context, key string) (model.SystemConfig, bool, error) {
	if f.err != nil {
		return model.SystemConfig{}, false, f.err
	}
	v, ok := f.vals[key]
	if !ok {
		return model.SystemConfig{}, false, nil
	}
	return model.SystemConfig{Key: key, Value: v}, true, nil
}

func (f *fakeSysConfigStore) Set(_ context.Context, key, value string) error {
	if f.err != nil {
		return f.err
	}
	if f.vals == nil {
		f.vals = make(map[string]string)
	}
	f.vals[key] = value
	return nil
}

type fakeEngineValidatorForSys struct {
	valid map[string]bool
}

func (f *fakeEngineValidatorForSys) ValidateEngine(name string) error {
	if name == "" || f.valid[name] {
		return nil
	}
	return fmt.Errorf("engine %q not enabled", name)
}

func (f *fakeEngineValidatorForSys) ValidateEngineExtraArgs(raw map[string]string) error {
	for engine := range raw {
		if err := f.ValidateEngine(engine); err != nil {
			return err
		}
	}
	return nil
}

func newSysConfigRouter(store sysConfigStore, validator engineValidatorForSys) (*gin.Engine, *enginecfg.Store) {
	gin.SetMode(gin.TestMode)
	cfg := enginecfg.NewStore("")
	h := NewSystemConfigHandler(store, validator, cfg)
	r := gin.New()
	api := r.Group("/api")
	api.GET("/system-configs", h.Get)
	api.PUT("/system-configs/:key", h.Set)
	return r, cfg
}

// --- tests ---

func TestSystemConfigHandler_Get_Empty(t *testing.T) {
	router, _ := newSysConfigRouter(&fakeSysConfigStore{vals: map[string]string{}}, &fakeEngineValidatorForSys{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/system-configs", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp[model.SystemConfigKeyDefaultEngine] != "" {
		t.Errorf("expected empty default_engine, got %q", resp[model.SystemConfigKeyDefaultEngine])
	}
}

func TestSystemConfigHandler_Get_WithValue(t *testing.T) {
	store := &fakeSysConfigStore{vals: map[string]string{model.SystemConfigKeyDefaultEngine: "claude"}}
	router, _ := newSysConfigRouter(store, &fakeEngineValidatorForSys{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/system-configs", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp[model.SystemConfigKeyDefaultEngine] != "claude" {
		t.Errorf("expected claude, got %q", resp[model.SystemConfigKeyDefaultEngine])
	}
}

func TestSystemConfigHandler_Set_ValidEngine(t *testing.T) {
	store := &fakeSysConfigStore{vals: map[string]string{}}
	validator := &fakeEngineValidatorForSys{valid: map[string]bool{"claude": true}}
	router, cfg := newSysConfigRouter(store, validator)

	body, _ := json.Marshal(map[string]string{"value": "claude"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/"+model.SystemConfigKeyDefaultEngine, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if store.vals[model.SystemConfigKeyDefaultEngine] != "claude" {
		t.Errorf("expected store to have claude, got %q", store.vals[model.SystemConfigKeyDefaultEngine])
	}
	if got := cfg.Get(); got != "claude" {
		t.Errorf("engineCfg not updated: got %q", got)
	}
}

func TestSystemConfigHandler_Set_ClearToDefault(t *testing.T) {
	store := &fakeSysConfigStore{vals: map[string]string{model.SystemConfigKeyDefaultEngine: "claude"}}
	router, _ := newSysConfigRouter(store, &fakeEngineValidatorForSys{})

	body, _ := json.Marshal(map[string]string{"value": ""})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/"+model.SystemConfigKeyDefaultEngine, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when clearing to system default, got %d: %s", w.Code, w.Body.String())
	}
	if store.vals[model.SystemConfigKeyDefaultEngine] != "" {
		t.Errorf("expected store to have empty value, got %q", store.vals[model.SystemConfigKeyDefaultEngine])
	}
}

func TestSystemConfigHandler_Set_InvalidEngine(t *testing.T) {
	store := &fakeSysConfigStore{vals: map[string]string{}}
	validator := &fakeEngineValidatorForSys{valid: map[string]bool{"claude": true}}
	router, _ := newSysConfigRouter(store, validator)

	body, _ := json.Marshal(map[string]string{"value": "unknown-engine"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/"+model.SystemConfigKeyDefaultEngine, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSystemConfigHandler_Set_UnknownKey(t *testing.T) {
	router, _ := newSysConfigRouter(&fakeSysConfigStore{vals: map[string]string{}}, &fakeEngineValidatorForSys{})

	body, _ := json.Marshal(map[string]string{"value": "something"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/unknown_key", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSystemConfigHandler_Get_StoreError(t *testing.T) {
	store := &fakeSysConfigStore{err: errors.New("db down")}
	router, _ := newSysConfigRouter(store, &fakeEngineValidatorForSys{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/system-configs", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestSystemConfigHandler_Set_StoreError(t *testing.T) {
	store := &fakeSysConfigStore{err: errors.New("db down")}
	validator := &fakeEngineValidatorForSys{valid: map[string]bool{"claude": true}}
	router, _ := newSysConfigRouter(store, validator)
	body, _ := json.Marshal(map[string]string{"value": "claude"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/"+model.SystemConfigKeyDefaultEngine, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
