package plugincmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakeAPI struct {
	mu          sync.Mutex
	catalog     *Catalog
	catalogErr  string
	plugins     map[string]*Detail
	installs    []string
	failInstall map[string]string
	settings    map[string]map[string]any
	botNames    map[string]*string
	actions     []string
	states      map[string][]string
	ops         map[string]Operation
	opSeq       int
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		plugins:     map[string]*Detail{},
		failInstall: map[string]string{},
		settings:    map[string]map[string]any{},
		botNames:    map[string]*string{},
		states:      map[string][]string{},
		ops:         map[string]Operation{},
	}
}

func (f *fakeAPI) client(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return &Client{baseURL: srv.URL, token: "tok", http: srv.Client()}
}

func (f *fakeAPI) addPlugin(d Detail) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := d
	f.plugins[d.ID] = &cp
}

func idFromSpec(spec string) string {
	name := spec
	if i := strings.LastIndex(spec, "@"); i > 0 {
		name = spec[:i]
	}
	name = name[strings.LastIndex(name, "/")+1:]
	return strings.TrimPrefix(name, "channel-")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api/plugins")
	switch {
	case path == "" && r.Method == http.MethodGet:
		var list []Plugin
		for _, id := range sortedKeys(f.plugins) {
			list = append(list, f.plugins[id].Plugin)
		}
		writeJSON(w, http.StatusOK, map[string]any{"plugins": list})
	case path == "/catalog":
		if f.catalogErr != "" {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": f.catalogErr, "code": "plugin_catalog_unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, f.catalog)
	case path == "/install":
		var body struct{ Spec string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.installs = append(f.installs, body.Spec)
		f.opSeq++
		op := Operation{ID: fmt.Sprintf("op%d", f.opSeq), Kind: "install", Spec: body.Spec, Status: "succeeded", PluginID: idFromSpec(body.Spec)}
		if msg, ok := f.failInstall[body.Spec]; ok {
			op.Status, op.Error, op.PluginID = "failed", msg, ""
		} else if _, exists := f.plugins[op.PluginID]; !exists {
			f.plugins[op.PluginID] = &Detail{Plugin: Plugin{ID: op.PluginID, State: "not_configured"}}
		}
		f.ops[op.ID] = op
		writeJSON(w, http.StatusAccepted, Operation{ID: op.ID, Kind: "install", Spec: body.Spec, Status: "queued"})
	case strings.HasPrefix(path, "/operations/"):
		op, ok := f.ops[strings.TrimPrefix(path, "/operations/")]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "operation not found", "code": "plugin_operation_not_found"})
			return
		}
		writeJSON(w, http.StatusOK, op)
	default:
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		id := parts[0]
		d, ok := f.plugins[id]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("plugin %q is not installed", id), "code": "plugin_not_found"})
			return
		}
		switch {
		case len(parts) == 1 && r.Method == http.MethodGet:
			if seq := f.states[id]; len(seq) > 0 {
				d.State = seq[0]
				if len(seq) > 1 {
					f.states[id] = seq[1:]
				}
				if d.State == "failed" {
					d.Error = "plugin rejected its config: bad token"
				}
			}
			writeJSON(w, http.StatusOK, d)
		case len(parts) == 1 && r.Method == http.MethodDelete:
			f.actions = append(f.actions, "remove:"+id)
			delete(f.plugins, id)
			f.opSeq++
			op := Operation{ID: fmt.Sprintf("op%d", f.opSeq), Kind: "uninstall", PluginID: id, Status: "succeeded"}
			f.ops[op.ID] = op
			writeJSON(w, http.StatusAccepted, op)
		case parts[1] == "settings":
			var body struct {
				BotName *string        `json:"bot_name"`
				Config  map[string]any `json:"config"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.settings[id] = body.Config
			f.botNames[id] = body.BotName
			f.actions = append(f.actions, "settings:"+id)
			w.WriteHeader(http.StatusNoContent)
		default:
			f.actions = append(f.actions, parts[1]+":"+id)
			if parts[1] == "enable" {
				d.Enabled = true
			}
			if parts[1] == "disable" {
				d.Enabled = false
				d.State = "disabled"
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func sortedKeys(m map[string]*Detail) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

type fakePrompter struct {
	interactive bool
	answers     []any
	asked       []string
	defaults    map[string]string
}

func (p *fakePrompter) next(kind, message string) any {
	p.asked = append(p.asked, kind+":"+message)
	if len(p.answers) == 0 {
		panic("no answer left for " + kind + ": " + message)
	}
	a := p.answers[0]
	p.answers = p.answers[1:]
	return a
}

func (p *fakePrompter) Interactive() bool { return p.interactive }

func (p *fakePrompter) Confirm(message string, _ bool) (bool, error) {
	return p.next("confirm", message).(bool), nil
}

func (p *fakePrompter) Input(message, _, def string, validate func(string) error) (string, error) {
	if p.defaults == nil {
		p.defaults = map[string]string{}
	}
	p.defaults[message] = def
	v := p.next("input", message).(string)
	if validate != nil {
		if err := validate(v); err != nil {
			return "", err
		}
	}
	return v, nil
}

func (p *fakePrompter) Password(message, _ string) (string, error) {
	return p.next("password", message).(string), nil
}

func (p *fakePrompter) MultiSelect(message string, _ []string) ([]int, error) {
	return p.next("multiselect", message).([]int), nil
}
