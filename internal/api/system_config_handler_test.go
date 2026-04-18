package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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

func newSysConfigRouter(store sysConfigStore, validator engineValidatorForSys) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewSystemConfigHandler(store, validator)
	r := gin.New()
	api := r.Group("/api")
	api.GET("/system-configs", h.Get)
	api.PUT("/system-configs/:key", h.Set)
	return r
}

// --- tests ---

func TestSystemConfigHandler_Get_Empty(t *testing.T) {
	router := newSysConfigRouter(&fakeSysConfigStore{vals: map[string]string{}}, &fakeEngineValidatorForSys{})

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
	if resp["default_engine"] != "" {
		t.Errorf("expected empty default_engine, got %q", resp["default_engine"])
	}
}

func TestSystemConfigHandler_Get_WithValue(t *testing.T) {
	store := &fakeSysConfigStore{vals: map[string]string{"default_engine": "claude"}}
	router := newSysConfigRouter(store, &fakeEngineValidatorForSys{})

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
	if resp["default_engine"] != "claude" {
		t.Errorf("expected claude, got %q", resp["default_engine"])
	}
}

func TestSystemConfigHandler_Set_ValidEngine(t *testing.T) {
	store := &fakeSysConfigStore{vals: map[string]string{}}
	validator := &fakeEngineValidatorForSys{valid: map[string]bool{"claude": true}}
	router := newSysConfigRouter(store, validator)

	body, _ := json.Marshal(map[string]string{"value": "claude"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/default_engine", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if store.vals["default_engine"] != "claude" {
		t.Errorf("expected store to have claude, got %q", store.vals["default_engine"])
	}
}

func TestSystemConfigHandler_Set_InvalidEngine(t *testing.T) {
	store := &fakeSysConfigStore{vals: map[string]string{}}
	validator := &fakeEngineValidatorForSys{valid: map[string]bool{"claude": true}}
	router := newSysConfigRouter(store, validator)

	body, _ := json.Marshal(map[string]string{"value": "unknown-engine"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/default_engine", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSystemConfigHandler_Set_UnknownKey(t *testing.T) {
	router := newSysConfigRouter(&fakeSysConfigStore{vals: map[string]string{}}, &fakeEngineValidatorForSys{})

	body, _ := json.Marshal(map[string]string{"value": "something"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/system-configs/unknown_key", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
