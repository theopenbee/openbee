package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/domain/pluginmgr"
	"github.com/theopenbee/openbee/internal/plugin"
	"github.com/theopenbee/openbee/internal/plugin/catalog"
)

type pluginService interface {
	List(ctx context.Context) ([]pluginmgr.Status, error)
	Get(ctx context.Context, id string) (pluginmgr.Detail, error)
	Catalog(ctx context.Context, refresh bool) (pluginmgr.CatalogView, error)
	Install(spec string) pluginmgr.Operation
	Uninstall(ctx context.Context, id string) (pluginmgr.Operation, error)
	Operation(id string) (pluginmgr.Operation, bool)
	Operations() []pluginmgr.Operation
	UpdateSettings(ctx context.Context, id string, botName *string, config map[string]any) error
	Enable(ctx context.Context, id string) error
	Disable(ctx context.Context, id string) error
	Restart(ctx context.Context, id string) error
	StartSetup(ctx context.Context, id string) (string, error)
	SubscribeSetup(sid string) ([]pluginmgr.SetupEvent, <-chan pluginmgr.SetupEvent, func(), error)
	SetupInput(sid, text string) error
	CancelSetup(sid string) error
}

const setupKeepAlive = 15 * time.Second

type PluginHandler struct {
	svc pluginService
}

func NewPluginHandler(svc pluginService) *PluginHandler {
	return &PluginHandler{svc: svc}
}

type pluginView struct {
	ID          string               `json:"id"`
	Package     string               `json:"package,omitempty"`
	Source      string               `json:"source"`
	Version     string               `json:"version,omitempty"`
	Path        string               `json:"path"`
	Name        plugin.LocalizedText `json:"name"`
	Description plugin.LocalizedText `json:"description,omitempty"`
	Enabled     bool                 `json:"enabled"`
	BotName     string               `json:"bot_name"`
	State       pluginmgr.State      `json:"state"`
	Error       string               `json:"error,omitempty"`
	HasSetup    bool                 `json:"has_setup"`
}

type pluginDetailView struct {
	pluginView
	Config     map[string]any       `json:"config"`
	Fields     []plugin.ConfigField `json:"fields"`
	SetupLabel plugin.LocalizedText `json:"setup_label,omitempty"`
}

type operationView struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Spec       string     `json:"spec,omitempty"`
	PluginID   string     `json:"plugin_id,omitempty"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type catalogPluginView struct {
	catalog.Entry
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version,omitempty"`
	UpdateAvailable  bool   `json:"update_available"`
	Installable      bool   `json:"installable"`
}

type catalogView struct {
	SchemaVersion int                 `json:"schema_version"`
	Categories    []catalog.Vocab     `json:"categories"`
	Tags          []catalog.Vocab     `json:"tags"`
	Plugins       []catalogPluginView `json:"plugins"`
}

func toPluginView(s pluginmgr.Status) pluginView {
	return pluginView{
		ID: s.ID, Package: s.Package, Source: s.Source, Version: s.Version, Path: s.Path,
		Name: s.Name, Description: s.Description, Enabled: s.Enabled, BotName: s.BotName,
		State: s.State, Error: s.Error, HasSetup: s.HasSetup,
	}
}

func toOperationView(op pluginmgr.Operation) operationView {
	v := operationView{ID: op.ID, Kind: op.Kind, Spec: op.Spec, PluginID: op.PluginID, Status: op.Status, Error: op.Error, CreatedAt: op.CreatedAt}
	if !op.FinishedAt.IsZero() {
		t := op.FinishedAt
		v.FinishedAt = &t
	}
	return v
}

func pluginErrorStatus(err error) int {
	switch apperr.Code(err) {
	case pluginmgr.CodeNotFound, pluginmgr.CodeOperationNotFound, pluginmgr.CodeSetupNotFound:
		return http.StatusNotFound
	case pluginmgr.CodeConfigInvalid, pluginmgr.CodeUnknownField, pluginmgr.CodeBotNameConflict,
		pluginmgr.CodeEnvSecretMissing, pluginmgr.CodeNotConfigured, pluginmgr.CodeNoSetup:
		return http.StatusBadRequest
	case pluginmgr.CodeSetupFinished:
		return http.StatusConflict
	case pluginmgr.CodeCatalogUnavailable:
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

func respondPluginError(c *gin.Context, err error) {
	respondError(c, pluginErrorStatus(err), err)
}

func (h *PluginHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.Request.Context())
	if err != nil {
		respondPluginError(c, err)
		return
	}
	out := make([]pluginView, 0, len(list))
	for _, s := range list {
		out = append(out, toPluginView(s))
	}
	c.JSON(http.StatusOK, gin.H{"plugins": out})
}

func (h *PluginHandler) Get(c *gin.Context) {
	d, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondPluginError(c, err)
		return
	}
	fields := d.Fields
	if fields == nil {
		fields = []plugin.ConfigField{}
	}
	c.JSON(http.StatusOK, pluginDetailView{pluginView: toPluginView(d.Status), Config: d.Config, Fields: fields, SetupLabel: d.SetupLabel})
}

func (h *PluginHandler) Catalog(c *gin.Context) {
	refresh := c.Query("refresh") == "1" || c.Query("refresh") == "true"
	view, err := h.svc.Catalog(c.Request.Context(), refresh)
	if err != nil {
		respondPluginError(c, err)
		return
	}
	out := catalogView{SchemaVersion: view.SchemaVersion, Categories: view.Categories, Tags: view.Tags, Plugins: make([]catalogPluginView, 0, len(view.Plugins))}
	if out.Categories == nil {
		out.Categories = []catalog.Vocab{}
	}
	if out.Tags == nil {
		out.Tags = []catalog.Vocab{}
	}
	for _, p := range view.Plugins {
		out.Plugins = append(out.Plugins, catalogPluginView{
			Entry: p.Entry, Installed: p.Installed, InstalledVersion: p.InstalledVersion,
			UpdateAvailable: p.UpdateAvailable, Installable: p.Installable,
		})
	}
	c.JSON(http.StatusOK, out)
}

func (h *PluginHandler) Install(c *gin.Context) {
	var req struct {
		Spec string `json:"spec"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	spec := strings.TrimSpace(req.Spec)
	if spec == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "spec is required"})
		return
	}
	c.JSON(http.StatusAccepted, toOperationView(h.svc.Install(spec)))
}

func (h *PluginHandler) Operations(c *gin.Context) {
	ops := h.svc.Operations()
	out := make([]operationView, 0, len(ops))
	for _, op := range ops {
		out = append(out, toOperationView(op))
	}
	c.JSON(http.StatusOK, gin.H{"operations": out})
}

func (h *PluginHandler) Operation(c *gin.Context) {
	op, ok := h.svc.Operation(c.Param("id"))
	if !ok {
		err := apperr.New(pluginmgr.CodeOperationNotFound, "operation not found").WithParams(map[string]any{"id": c.Param("id")})
		respondPluginError(c, err)
		return
	}
	c.JSON(http.StatusOK, toOperationView(op))
}

func (h *PluginHandler) UpdateSettings(c *gin.Context) {
	var req struct {
		BotName *string        `json:"bot_name"`
		Config  map[string]any `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.UpdateSettings(c.Request.Context(), c.Param("id"), req.BotName, req.Config); err != nil {
		respondPluginError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *PluginHandler) lifecycle(c *gin.Context, fn func(context.Context, string) error) {
	if err := fn(c.Request.Context(), c.Param("id")); err != nil {
		respondPluginError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *PluginHandler) Enable(c *gin.Context)  { h.lifecycle(c, h.svc.Enable) }
func (h *PluginHandler) Disable(c *gin.Context) { h.lifecycle(c, h.svc.Disable) }
func (h *PluginHandler) Restart(c *gin.Context) { h.lifecycle(c, h.svc.Restart) }

func (h *PluginHandler) Uninstall(c *gin.Context) {
	op, err := h.svc.Uninstall(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondPluginError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, toOperationView(op))
}

func (h *PluginHandler) StartSetup(c *gin.Context) {
	sid, err := h.svc.StartSetup(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondPluginError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": sid})
}

func (h *PluginHandler) SetupEvents(c *gin.Context) {
	history, ch, unsubscribe, err := h.svc.SubscribeSetup(c.Param("sid"))
	if err != nil {
		respondPluginError(c, err)
		return
	}
	defer unsubscribe()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	write := func(ev pluginmgr.SetupEvent) {
		data, _ := json.Marshal(ev)
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	}
	for _, ev := range history {
		write(ev)
	}
	c.Writer.Flush()

	ping := time.NewTicker(setupKeepAlive)
	defer ping.Stop()
	ctx := c.Request.Context()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			write(ev)
			c.Writer.Flush()
		case <-ping.C:
			fmt.Fprint(c.Writer, ": ping\n\n")
			c.Writer.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func (h *PluginHandler) SetupInput(c *gin.Context) {
	var req struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.SetupInput(c.Param("sid"), req.Text); err != nil {
		respondPluginError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *PluginHandler) CancelSetup(c *gin.Context) {
	if err := h.svc.CancelSetup(c.Param("sid")); err != nil {
		respondPluginError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
