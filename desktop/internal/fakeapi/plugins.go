package fakeapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const (
	operationLimit  = 50
	secretMinLength = 12
	secretTail      = 4
	randomHexBytes  = 8
	opInstall       = "install"
	opUninstall     = "uninstall"
	opRunning       = "running"
	opSucceeded     = "succeeded"
	opFailed        = "failed"
	setupDone       = "done"
	typeEngine      = "engine"
	stateReady      = "ready"
	stateRunning    = "running"
)

type setupSession struct {
	plugin  string
	history []api.SetupEvent
	subs    map[*stream]struct{}
	done    bool
}

type setupBook struct {
	sessions map[string]*setupSession
	byPlugin map[string]string
}

func newSetupBook() setupBook {
	return setupBook{sessions: map[string]*setupSession{}, byPlugin: map[string]string{}}
}

func (s *Server) pluginRoutes() {
	s.route("GET /api/plugins", authPlugins, s.listPlugins)
	s.route("GET /api/plugins/catalog", authPlugins, s.getCatalog)
	s.route("POST /api/plugins/install", authPlugins, s.installPlugin)
	s.route("GET /api/plugins/operations", authPlugins, s.listOperations)
	s.route("GET /api/plugins/operations/{id}", authPlugins, s.getOperation)
	s.raw("GET /api/plugins/setup/{sid}/events", authPlugins, s.setupEvents)
	s.route("POST /api/plugins/setup/{sid}/input", authPlugins, s.setupInput)
	s.route("DELETE /api/plugins/setup/{sid}", authPlugins, s.cancelSetup)
	s.route("GET /api/plugins/{id}", authPlugins, s.getPlugin)
	s.route("PUT /api/plugins/{id}/settings", authPlugins, s.updatePluginSettings)
	s.route("POST /api/plugins/{id}/enable", authPlugins, s.enablePlugin)
	s.route("POST /api/plugins/{id}/disable", authPlugins, s.disablePlugin)
	s.route("POST /api/plugins/{id}/restart", authPlugins, s.restartPlugin)
	s.route("DELETE /api/plugins/{id}", authPlugins, s.uninstallPlugin)
	s.route("POST /api/plugins/{id}/setup", authPlugins, s.startSetup)
}

func pluginNotFound(id string) reply {
	return codedReply(http.StatusNotFound, "plugin_not_found", fmt.Sprintf("plugin %q is not installed", id), map[string]any{"id": id})
}

func notConfigured(id, reason string) reply {
	return codedReply(http.StatusBadRequest, "plugin_not_configured",
		fmt.Sprintf("plugin %q is not configured: %s", id, reason), map[string]any{"id": id, "reason": reason})
}

func configInvalid(reason string) reply {
	return codedReply(http.StatusBadRequest, "plugin_config_invalid", reason, map[string]any{"reason": reason})
}

func setupNotFound(sid string) reply {
	return codedReply(http.StatusNotFound, "plugin_setup_not_found", "setup session not found", map[string]any{"id": sid})
}

func (s *Server) pluginIndex(id string) int {
	return slices.IndexFunc(s.st.Plugins, func(p api.PluginDetail) bool { return p.ID == id })
}

func outDetail(p api.PluginDetail) api.PluginDetail {
	if p.Name == nil {
		p.Name = api.LocalizedText{"en": p.ID}
	}
	p.Config = maps.Clone(p.Config)
	if p.Config == nil {
		p.Config = map[string]any{}
	}
	if p.Fields == nil {
		p.Fields = []api.PluginConfigField{}
	}
	return p
}

func (s *Server) listPlugins(*request) reply {
	out := make([]api.PluginStatus, 0, len(s.st.Plugins))
	for _, p := range s.st.Plugins {
		out = append(out, outDetail(p).PluginStatus)
	}
	return ok(map[string]any{"plugins": out})
}

func (s *Server) getPlugin(q *request) reply {
	i := s.pluginIndex(q.param("id"))
	if i < 0 {
		return pluginNotFound(q.param("id"))
	}
	return ok(outDetail(s.st.Plugins[i]))
}

func (s *Server) getCatalog(*request) reply {
	out := api.PluginCatalog{
		SchemaVersion: s.st.Catalog.SchemaVersion,
		Categories:    slices.Clone(s.st.Catalog.Categories),
		Tags:          slices.Clone(s.st.Catalog.Tags),
		Plugins:       make([]api.CatalogPlugin, 0, len(s.st.Catalog.Plugins)),
	}
	if out.Categories == nil {
		out.Categories = []api.CatalogTag{}
	}
	if out.Tags == nil {
		out.Tags = []api.CatalogTag{}
	}
	for _, p := range s.st.Catalog.Plugins {
		if p.Maintainers == nil {
			p.Maintainers = []string{}
		}
		if p.Tags == nil {
			p.Tags = []string{}
		}
		out.Plugins = append(out.Plugins, p)
	}
	return ok(out)
}

func (s *Server) newOperation(kind, spec, pluginID string) api.PluginOperation {
	op := api.PluginOperation{
		ID:        s.nextID("op"),
		Kind:      kind,
		Spec:      spec,
		PluginID:  pluginID,
		Status:    opRunning,
		CreatedAt: time.Now().UTC(),
	}
	s.st.Operations = append(s.st.Operations, op)
	return op
}

func (s *Server) installPlugin(q *request) reply {
	var req struct {
		Spec string `json:"spec"`
	}
	if bad, good := q.decode(&req); !good {
		return bad
	}
	spec := strings.TrimSpace(req.Spec)
	if spec == "" {
		return errReply(http.StatusBadRequest, "spec is required")
	}
	return accepted(s.newOperation(opInstall, spec, ""))
}

func (s *Server) uninstallPlugin(q *request) reply {
	id := q.param("id")
	if s.pluginIndex(id) < 0 {
		return pluginNotFound(id)
	}
	return accepted(s.newOperation(opUninstall, "", id))
}

func (s *Server) listOperations(*request) reply {
	ops := slices.Clone(s.st.Operations)
	slices.Reverse(ops)
	if len(ops) > operationLimit {
		ops = ops[:operationLimit]
	}
	if ops == nil {
		ops = []api.PluginOperation{}
	}
	return ok(map[string]any{"operations": ops})
}

func (s *Server) getOperation(q *request) reply {
	id := q.param("id")
	i := slices.IndexFunc(s.st.Operations, func(op api.PluginOperation) bool { return op.ID == id })
	if i < 0 {
		return codedReply(http.StatusNotFound, "plugin_operation_not_found", "operation not found", map[string]any{"id": id})
	}
	return ok(s.st.Operations[i])
}

func (s *Server) FinishOperation(id string, succeeded bool, errText string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.st.Operations, func(op api.PluginOperation) bool { return op.ID == id })
	if i < 0 {
		s.t.Errorf("fakeapi: unknown plugin operation %q", id)
		return
	}
	op := s.st.Operations[i]
	finished := time.Now().UTC()
	op.FinishedAt = &finished
	op.Status = opFailed
	op.Error = errText
	if succeeded {
		op.Status = opSucceeded
		op.Error = ""
	}
	s.st.Operations[i] = op
	if !succeeded {
		return
	}
	switch op.Kind {
	case opInstall:
		s.finishInstall(op.Spec)
	case opUninstall:
		s.finishUninstall(op.PluginID)
	}
}

func splitSpec(spec string) (name, version string) {
	if i := strings.LastIndex(spec, "@"); i > 0 {
		return spec[:i], spec[i+1:]
	}
	return spec, ""
}

func (s *Server) finishInstall(spec string) {
	source := "npm"
	name, version := splitSpec(spec)
	if strings.HasPrefix(spec, "/") || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "~") {
		source, name, version = "path", spec, ""
	}
	entry := slices.IndexFunc(s.st.Catalog.Plugins, func(p api.CatalogPlugin) bool {
		return source == "npm" && (p.Package == name || p.ID == name)
	})
	id := filepath.Base(name)
	pkg := name
	display := api.LocalizedText(nil)
	var description api.LocalizedText
	kind := ""
	if entry >= 0 {
		match := s.st.Catalog.Plugins[entry]
		id, pkg, display, description = match.ID, match.Package, match.Name, match.Description
		if match.Category == typeEngine {
			kind = typeEngine
		}
		if version == "" {
			version = match.Version
		}
	}
	if source == "path" {
		pkg = ""
	}
	if i := s.pluginIndex(id); i >= 0 {
		s.st.Plugins[i].Version = version
	} else {
		status := api.PluginStatus{
			ID:          id,
			Type:        kind,
			Package:     pkg,
			Source:      source,
			Version:     version,
			Path:        "/tmp/openbee/plugins/" + id,
			Name:        display,
			Description: description,
			State:       "not_configured",
		}
		if kind == typeEngine {
			status.Enabled, status.State = true, stateReady
		}
		s.st.Plugins = append(s.st.Plugins, api.PluginDetail{
			PluginStatus: status,
			Config:       map[string]any{},
			Fields:       []api.PluginConfigField{},
		})
	}
	s.markCatalogInstalled(id, true, version)
}

func (s *Server) finishUninstall(id string) {
	s.st.Plugins = slices.DeleteFunc(s.st.Plugins, func(p api.PluginDetail) bool { return p.ID == id })
	s.markCatalogInstalled(id, false, "")
}

func (s *Server) markCatalogInstalled(id string, installed bool, version string) {
	for i, p := range s.st.Catalog.Plugins {
		if p.ID != id {
			continue
		}
		p.Installed = installed
		p.InstalledVersion = version
		p.UpdateAvailable = false
		s.st.Catalog.Plugins[i] = p
	}
}

func maskSecret(v any) any {
	text, isText := v.(string)
	if !isText || text == "" {
		return v
	}
	runes := []rune(text)
	if len(runes) < secretMinLength {
		return "****"
	}
	return "****" + string(runes[len(runes)-secretTail:])
}

func fieldTypeError(f api.PluginConfigField, v any) string {
	valid := true
	switch f.Type {
	case "string", "secret":
		_, valid = v.(string)
	case "bool":
		_, valid = v.(bool)
	case "int":
		_, valid = v.(float64)
	case "list":
		items, isList := v.([]any)
		valid = isList && !slices.ContainsFunc(items, func(item any) bool {
			_, isText := item.(string)
			return !isText
		})
	}
	if valid {
		return ""
	}
	return fmt.Sprintf("config %q must be of type %s", f.Key, f.Type)
}

func randomHex() string {
	buf := make([]byte, randomHexBytes)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func (s *Server) mergePluginConfig(p api.PluginDetail, update map[string]any) (map[string]any, reply) {
	declared := make(map[string]bool, len(p.Fields))
	for _, f := range p.Fields {
		declared[f.Key] = true
	}
	for _, key := range slices.Sorted(maps.Keys(update)) {
		if !declared[key] {
			return nil, codedReply(http.StatusBadRequest, "plugin_unknown_field",
				fmt.Sprintf("config %q is not declared by the plugin", key), map[string]any{"key": key})
		}
	}
	out := make(map[string]any, len(p.Fields))
	for _, f := range p.Fields {
		cur, hasCur := p.Config[f.Key]
		v, sent := update[f.Key]
		switch {
		case sent && f.Type == "secret" && (v == nil || v == "" || (hasCur && (v == cur || v == maskSecret(cur)))):
			if hasCur {
				out[f.Key] = cur
			}
		case sent && v == nil:
		case sent:
			if msg := fieldTypeError(f, v); msg != "" {
				return nil, configInvalid(msg)
			}
			if f.Type == "secret" {
				v = maskSecret(v)
			}
			out[f.Key] = v
		case hasCur:
			out[f.Key] = cur
		case f.RandomDefault:
			out[f.Key] = randomHex()
		}
	}
	if reason := missingRequired(p.Fields, out); reason != "" {
		return nil, configInvalid(reason)
	}
	return out, reply{}
}

func missingRequired(fields []api.PluginConfigField, cfg map[string]any) string {
	for _, f := range fields {
		if !f.Required || f.Default != nil {
			continue
		}
		if v, has := cfg[f.Key]; !has || v == "" {
			return fmt.Sprintf("config %q is required", f.Key)
		}
	}
	return ""
}

func (s *Server) updatePluginSettings(q *request) reply {
	id := q.param("id")
	var req api.PluginSettingsUpdate
	if bad, good := q.decode(&req); !good {
		return bad
	}
	i := s.pluginIndex(id)
	if i < 0 {
		return pluginNotFound(id)
	}
	p := s.st.Plugins[i]
	merged, bad := s.mergePluginConfig(p, req.Config)
	if bad.status != 0 {
		return bad
	}
	if req.BotName != nil {
		name := strings.TrimSpace(*req.BotName)
		if name != "" && p.Type == typeEngine {
			return codedReply(http.StatusBadRequest, "plugin_bot_name_not_supported",
				fmt.Sprintf("plugin %q is an engine and has no bot name", id), map[string]any{"id": id})
		}
		conflict := slices.ContainsFunc(s.st.Workers, func(w api.Worker) bool { return strings.EqualFold(w.Name, name) })
		if name != "" && conflict {
			return codedReply(http.StatusBadRequest, "plugin_bot_name_conflict",
				fmt.Sprintf("bot name %q is already used by a worker", name), map[string]any{"name": name})
		}
		p.BotName = name
	}
	p.Config = merged
	s.st.Plugins[i] = p
	return noContent()
}

func healthyState(p api.PluginDetail) string {
	if p.Type == typeEngine {
		return stateReady
	}
	return stateRunning
}

func (s *Server) enablePlugin(q *request) reply {
	id := q.param("id")
	i := s.pluginIndex(id)
	if i < 0 {
		return pluginNotFound(id)
	}
	p := s.st.Plugins[i]
	if reason := missingRequired(p.Fields, p.Config); reason != "" {
		return notConfigured(id, reason)
	}
	p.Enabled = true
	p.State = healthyState(p)
	p.Error = ""
	s.st.Plugins[i] = p
	return noContent()
}

func (s *Server) disablePlugin(q *request) reply {
	i := s.pluginIndex(q.param("id"))
	if i < 0 {
		return pluginNotFound(q.param("id"))
	}
	s.st.Plugins[i].Enabled = false
	s.st.Plugins[i].State = "disabled"
	s.st.Plugins[i].Error = ""
	return noContent()
}

func (s *Server) restartPlugin(q *request) reply {
	id := q.param("id")
	i := s.pluginIndex(id)
	if i < 0 {
		return pluginNotFound(id)
	}
	if !s.st.Plugins[i].Enabled {
		return notConfigured(id, "plugin is disabled")
	}
	s.st.Plugins[i].State = healthyState(s.st.Plugins[i])
	s.st.Plugins[i].Error = ""
	return noContent()
}

func (s *Server) startSetup(q *request) reply {
	id := q.param("id")
	i := s.pluginIndex(id)
	if i < 0 {
		return pluginNotFound(id)
	}
	if !s.st.Plugins[i].HasSetup {
		return codedReply(http.StatusBadRequest, "plugin_no_setup",
			fmt.Sprintf("plugin %q has no setup step", id), map[string]any{"id": id})
	}
	if previous, found := s.setups.byPlugin[id]; found {
		s.cancelSession(previous)
	}
	sid := s.nextID("setup")
	s.setups.sessions[sid] = &setupSession{plugin: id, subs: map[*stream]struct{}{}}
	s.setups.byPlugin[id] = sid
	return created(map[string]string{"id": sid})
}

func (s *Server) cancelSession(sid string) {
	s.publishSetup(sid, api.SetupEvent{Type: setupDone, Error: "setup was cancelled"})
}

func (s *Server) publishSetup(sid string, ev api.SetupEvent) {
	ses := s.setups.sessions[sid]
	if ses == nil || ses.done {
		return
	}
	ses.history = append(ses.history, ev)
	frame := dataFrame(ev)
	for st := range ses.subs {
		st.push(frame)
	}
	if ev.Type != setupDone {
		return
	}
	ses.done = true
	for st := range ses.subs {
		st.close()
		delete(ses.subs, st)
	}
}

func (s *Server) SetupEmit(sid string, ev api.SetupEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setups.sessions[sid] == nil {
		s.t.Errorf("fakeapi: unknown plugin setup session %q", sid)
		return
	}
	s.publishSetup(sid, ev)
}

func (s *Server) SetupSession(pluginID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setups.byPlugin[pluginID]
}

func (s *Server) SetupStreams(sid string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ses := s.setups.sessions[sid]; ses != nil {
		return len(ses.subs)
	}
	return 0
}

func (s *Server) DropSetupStreams(sid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ses := s.setups.sessions[sid]
	if ses == nil {
		s.t.Errorf("fakeapi: unknown plugin setup session %q", sid)
		return
	}
	for st := range ses.subs {
		st.close()
		delete(ses.subs, st)
	}
}

func (s *Server) setupInput(q *request) reply {
	sid := q.param("sid")
	ses := s.setups.sessions[sid]
	if ses == nil {
		return setupNotFound(sid)
	}
	var req struct {
		Text string `json:"text"`
	}
	if bad, good := q.decode(&req); !good {
		return bad
	}
	if ses.done {
		return codedReply(http.StatusConflict, "plugin_setup_finished", "setup session has finished", map[string]any{"id": sid})
	}
	return noContent()
}

func (s *Server) cancelSetup(q *request) reply {
	sid := q.param("sid")
	if s.setups.sessions[sid] == nil {
		return setupNotFound(sid)
	}
	s.cancelSession(sid)
	return noContent()
}

func (s *Server) setupEvents(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	s.mu.Lock()
	ses := s.setups.sessions[sid]
	if ses == nil {
		s.mu.Unlock()
		setupNotFound(sid).write(w)
		return
	}
	st := newStream()
	for _, ev := range ses.history {
		st.push(dataFrame(ev))
	}
	if ses.done {
		st.close()
	} else {
		ses.subs[st] = struct{}{}
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(ses.subs, st)
		s.mu.Unlock()
	}()

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	s.pump(w, r, st)
}
