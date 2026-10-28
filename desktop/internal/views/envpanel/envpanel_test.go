package envpanel_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/envpanel"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	wideW, narrowW = 700, 480
	hostH          = 560
	hostPad        = 24
	motion         = time.Second
	envsPath       = "/api/envs"
	workerID       = "w1"
	afterGap       = 30
)

type host struct {
	*testkit.Harness
	t         *testing.T
	panel     *envpanel.Panel
	effective bool
}

func envOf(id, scope, scopeID, key, masked string, updated time.Duration) api.EnvConfig {
	env := api.EnvConfig{ID: id, Scope: scope, Key: key, Masked: masked, UpdatedAt: time.Now().Add(-updated).UnixMilli()}
	if scopeID != "" {
		env.ScopeID = &scopeID
	}
	return env
}

func seedEnvs(envs ...api.EnvConfig) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		st.Workers = []api.Worker{{ID: workerID, Name: "Alice", Status: "idle", Engine: "claude"}}
		st.Envs = envs
	}
}

func workerEnvs() func(st *fakeapi.State) {
	return seedEnvs(
		envOf("e1", "worker", workerID, "DB_URL", "post****able", 3*time.Minute),
		envOf("e2", "worker", workerID, "API_KEY", "sk-1****9f2a", 5*time.Hour),
		envOf("e3", "worker", workerID, "OLD_TOKEN", "****", 72*time.Hour),
		envOf("e4", "worker", "w2", "OTHER_WORKER", "****", time.Minute),
		envOf("e5", "global", "", "REGION", "eu-w****", time.Minute),
	)
}

func newHost(t *testing.T, w int, scope, scopeID string, inset bool, seed func(st *fakeapi.State)) *host {
	t.Helper()
	hs := testkit.SignedIn(t, w, hostH, seed)
	h := &host{Harness: hs, t: t}
	h.panel = envpanel.New(hs.Store, scope, scopeID, "Worker Variables", "Passed to this worker.", inset)
	hs.T = ui.NewTester(h.build, w, hostH)
	h.settle()
	return h
}

func (h *host) build(c *ui.Context) {
	c.DisableAnimations()
	tk := theme.Apply(c)
	ui.Column(c).Fill().Background(tk.Background).TextColor(tk.Foreground).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(hostPad).Gap(hostPad).Children(func() {
				h.panel.View(c)
				if h.effective {
					envpanel.Effective(c, h.Store, workerID)
				}
			})
		})
	})
}

func (h *host) settle() {
	h.Settle()
	h.T.Advance(motion)
	h.Settle()
}

func (h *host) click(name string) {
	h.t.Helper()
	if err := h.T.Click(name); err != nil {
		h.t.Fatalf("click %q: %v (texts %q)", name, err, h.T.Texts())
	}
	h.settle()
}

func (h *host) clickAfter(name string) {
	h.t.Helper()
	r, ok := h.T.Find(name)
	if !ok {
		h.t.Fatalf("no %q to click after; texts %q", name, h.T.Texts())
	}
	h.T.ClickAt(r.X+r.W+afterGap, r.Y+r.H/2)
	h.settle()
}

func (h *host) typeInto(field, text string) {
	h.t.Helper()
	if !h.T.Focused(field) {
		h.click(field)
	}
	h.T.Type(text)
	h.settle()
}

func (h *host) fieldText(field string) string {
	h.t.Helper()
	if !h.T.Focused(field) {
		h.click(field)
	}
	h.T.SetClipboard("")
	h.T.Command("selectAll")
	h.T.Command("copy")
	return h.T.Clipboard()
}

func (h *host) shows(text string) bool {
	_, ok := h.T.Find(text)
	return ok
}

func (h *host) wantShown(texts ...string) {
	h.t.Helper()
	for _, text := range texts {
		if !h.shows(text) {
			h.t.Fatalf("screen does not show %q; texts %q", text, h.T.Texts())
		}
	}
}

func (h *host) wantHidden(texts ...string) {
	h.t.Helper()
	for _, text := range texts {
		if h.shows(text) {
			h.t.Fatalf("screen shows %q; texts %q", text, h.T.Texts())
		}
	}
}

func (h *host) posts() []map[string]string {
	h.t.Helper()
	var out []map[string]string
	for _, call := range h.Fake.Calls(http.MethodPost, envsPath) {
		var body map[string]string
		if err := json.Unmarshal(call.Body, &body); err != nil {
			h.t.Fatalf("decode POST body %q: %v", call.Body, err)
		}
		out = append(out, body)
	}
	return out
}

func (h *host) puts(id string) []map[string]any {
	h.t.Helper()
	var out []map[string]any
	for _, call := range h.Fake.Calls(http.MethodPut, envsPath+"/"+id) {
		var body map[string]any
		if err := json.Unmarshal(call.Body, &body); err != nil {
			h.t.Fatalf("decode PUT body %q: %v", call.Body, err)
		}
		out = append(out, body)
	}
	return out
}

func (h *host) openAdd() {
	h.t.Helper()
	h.click(locales.EnvConfigAdd())
	h.wantShown(locales.EnvConfigAddDescription())
	if !h.T.Focused(locales.EnvConfigKey()) {
		h.t.Fatal("the key field does not take the focus when the dialog opens")
	}
}

func (h *host) serverKeys(scope, scopeID string) []string {
	var keys []string
	for _, e := range h.Fake.Snapshot().Envs {
		if e.Scope == scope && (scopeID == "" && e.ScopeID == nil || e.ScopeID != nil && *e.ScopeID == scopeID) {
			keys = append(keys, e.Key)
		}
	}
	return keys
}

func (h *host) rowY(key string) float32 {
	h.t.Helper()
	r, ok := h.T.Find(key)
	if !ok {
		h.t.Fatalf("no row %q; texts %q", key, h.T.Texts())
	}
	return r.Y
}

func TestListShowsRows(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.wantShown("Worker Variables", "Passed to this worker.", locales.EnvConfigAdd(),
		"DB_URL", "post****able", locales.EnvConfigUpdated(locales.TimeMinutesAgo(3)),
		"API_KEY", "sk-1****9f2a", locales.EnvConfigUpdated(locales.TimeHoursAgo(5)),
		"OLD_TOKEN", locales.EnvConfigUpdated(locales.TimeDaysAgo(3)),
		locales.EnvConfigCopyKey("DB_URL"), locales.EnvConfigEditLabel("DB_URL"), locales.EnvConfigDeleteLabel("DB_URL"),
		locales.EnvConfigCopyKey("OLD_TOKEN"), locales.EnvConfigEditLabel("OLD_TOKEN"), locales.EnvConfigDeleteLabel("OLD_TOKEN"))
	h.wantHidden("OTHER_WORKER", "REGION", locales.EnvConfigEmpty(), locales.EnvConfigLoadError())
	if h.rowY("DB_URL") >= h.rowY("API_KEY") || h.rowY("API_KEY") >= h.rowY("OLD_TOKEN") {
		t.Fatal("rows are not in the server order")
	}
	if got := h.Fake.Calls(http.MethodGet, envsPath); len(got) != 1 || got[0].Query != "scope=worker&scope_id=w1" {
		t.Fatalf("env calls = %+v, want one worker-scoped list", got)
	}
}

func TestJustNowAndMissingTime(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, seedEnvs(
		envOf("e1", "worker", workerID, "FRESH", "****", 10*time.Second),
		api.EnvConfig{ID: "e2", Scope: "worker", ScopeID: new(workerID), Key: "UNDATED", Masked: "****"},
	))
	h.wantShown(locales.EnvConfigUpdated(locales.TimeJustNow()), locales.EnvConfigUpdated("—"))
}

func TestUpdatedHiddenOnNarrowCard(t *testing.T) {
	h := newHost(t, narrowW, "worker", workerID, true, workerEnvs())
	h.wantShown("DB_URL", "post****able", locales.EnvConfigEditLabel("DB_URL"))
	h.wantHidden(locales.EnvConfigUpdated(locales.TimeMinutesAgo(3)), "·")

	h.T.SetSize(wideW, hostH)
	h.settle()
	h.wantShown(locales.EnvConfigUpdated(locales.TimeMinutesAgo(3)))

	h.T.SetSize(narrowW, hostH)
	h.settle()
	h.wantHidden(locales.EnvConfigUpdated(locales.TimeMinutesAgo(3)))
}

func TestLoadingShowsSkeleton(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	r := h.Store.Envs("worker", workerID)
	r.Reset()
	r.Loading = true
	h.settle()
	h.wantShown("Worker Variables", locales.EnvConfigAdd())
	h.wantHidden("DB_URL", locales.EnvConfigEmpty(), locales.EnvConfigLoadError())
}

func TestEmptyStatePerScope(t *testing.T) {
	for _, tc := range []struct {
		scope, scopeID string
		description    func() string
	}{
		{"global", "", locales.EnvConfigEmptyGlobal},
		{"bee", "default", locales.EnvConfigEmptyBee},
		{"worker", workerID, locales.EnvConfigEmptyWorker},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			h := newHost(t, wideW, tc.scope, tc.scopeID, false, seedEnvs())
			h.wantShown(locales.EnvConfigEmpty(), tc.description(), locales.EnvConfigAdd())
			if h.rowY(locales.EnvConfigAdd()) <= h.rowY(locales.EnvConfigEmpty()) {
				t.Fatal("the add button is not inside the empty state")
			}
			if tc.scope == "worker" {
				h.click(locales.EnvConfigAdd())
				h.wantShown(locales.EnvConfigAddDescription())
			}
		})
	}
}

func TestLoadErrorRetry(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.Fake.FailNext(http.MethodGet, envsPath, http.StatusInternalServerError, map[string]string{"error": "envs down"})
	h.Store.Envs("worker", workerID).Invalidate()
	h.settle()
	h.wantShown(locales.EnvConfigLoadError(), "envs down", locales.CommonRetry())
	h.wantHidden("DB_URL", locales.EnvConfigAdd())

	h.click(locales.CommonRetry())
	h.wantShown("DB_URL", locales.EnvConfigAdd())
	h.wantHidden(locales.EnvConfigLoadError())
}

func TestCopyKey(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.click(locales.EnvConfigCopyKey("API_KEY"))
	if got := h.T.Clipboard(); got != "API_KEY" {
		t.Fatalf("clipboard = %q, want API_KEY", got)
	}
}

func TestAddVariableNormalizesKey(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.openAdd()
	h.wantShown(locales.EnvConfigKeyHint())
	h.T.Type("db-url")
	h.settle()
	if got := h.fieldText(locales.EnvConfigKey()); got != "DBURL" {
		t.Fatalf("key field = %q, want DBURL", got)
	}

	h.T.Command("selectAll")
	h.T.Type("ab")
	h.T.Key(0, ui.KeyLeft)
	h.T.Type("x_-")
	h.T.Type("y")
	h.settle()
	if got := h.fieldText(locales.EnvConfigKey()); got != "AX_YB" {
		t.Fatalf("key field = %q, want AX_YB (the caret stays after the typed text)", got)
	}
	h.T.Command("selectAll")
	h.T.Type("a.b c/d:\u00e9")
	h.settle()
	if got := h.fieldText(locales.EnvConfigKey()); got != "ABCD" {
		t.Fatalf("key field = %q, want ABCD", got)
	}
}

func TestAddVariableDuplicate(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.openAdd()
	h.T.Type("api_key")
	h.settle()
	h.wantShown(locales.EnvConfigKeyExists("API_KEY"))
	h.wantHidden(locales.EnvConfigKeyHint())

	h.click(locales.EnvConfigValue())
	h.T.Type("secret")
	h.T.Key(0, ui.KeyEnter)
	h.settle()
	h.click(locales.EnvConfigSaveAndAddAnother())
	h.clickAfter(locales.EnvConfigSaveAndAddAnother())
	if got := h.posts(); len(got) != 0 {
		t.Fatalf("a duplicate key sent %v", got)
	}
	h.wantShown(locales.EnvConfigAddDescription(), locales.EnvConfigKeyExists("API_KEY"))

	h.click(locales.EnvConfigKey())
	h.T.Command("selectAll")
	h.T.Type("api_key2")
	h.settle()
	h.wantHidden(locales.EnvConfigKeyExists("API_KEY"), locales.EnvConfigKeyExists("API_KEY2"))
	h.wantShown(locales.EnvConfigKeyHint())

	h.T.Command("selectAll")
	h.T.Type("db_url")
	h.settle()
	h.wantShown(locales.EnvConfigKeyExists("DB_URL"))

	h.T.Command("selectAll")
	h.T.Type("region")
	h.settle()
	h.wantHidden(locales.EnvConfigKeyExists("REGION"))
	h.T.Command("selectAll")
	h.T.Type("other_worker")
	h.settle()
	h.wantHidden(locales.EnvConfigKeyExists("OTHER_WORKER"))
	h.wantShown(locales.EnvConfigKeyHint())
}

func TestAddVariableEnterSubmitsAndCloses(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.openAdd()
	h.T.Type("new_key")
	h.T.Key(0, ui.KeyEnter)
	h.settle()
	if got := h.posts(); len(got) != 0 {
		t.Fatalf("Enter without a value sent %v", got)
	}
	h.click(locales.EnvConfigValue())
	h.T.Type(" s3cret ")
	h.T.Key(0, ui.KeyEnter)
	h.settle()
	want := map[string]string{"scope": "worker", "scope_id": workerID, "key": "NEW_KEY", "value": " s3cret "}
	got := h.posts()
	if len(got) != 1 || len(got[0]) != len(want) {
		t.Fatalf("POST bodies = %v, want one %v", got, want)
	}
	for k, v := range want {
		if got[0][k] != v {
			t.Fatalf("POST body = %v, want %v (values are not trimmed)", got[0], want)
		}
	}
	h.wantHidden(locales.EnvConfigAddDescription(), locales.EnvConfigAdded("NEW_KEY"))
	h.wantShown("NEW_KEY")
}

func TestAddVariableReservedKey(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.openAdd()
	h.T.Type("openbee_api_key")
	h.settle()
	h.click(locales.EnvConfigValue())
	h.T.Type("x")
	h.T.Key(0, ui.KeyEnter)
	h.settle()
	h.wantShown(locales.ErrorsEnvReservedKey("OPENBEE_API_KEY"), locales.EnvConfigAddDescription())
	h.wantHidden(locales.EnvConfigSaveError(), locales.EnvConfigKeyHint())
	if len(h.posts()) != 1 {
		t.Fatalf("POST calls = %d, want 1", len(h.posts()))
	}

	h.click(locales.EnvConfigKey())
	h.T.Type("2")
	h.settle()
	h.wantHidden(locales.ErrorsEnvReservedKey("OPENBEE_API_KEY"))
	h.wantShown(locales.EnvConfigKeyHint())

	h.Fake.FailNext(http.MethodPost, envsPath, http.StatusInternalServerError, map[string]string{"error": "disk full"})
	h.T.Key(0, ui.KeyEnter)
	h.settle()
	h.wantShown(locales.EnvConfigSaveError(), "disk full", locales.EnvConfigAddDescription(), locales.EnvConfigKeyHint())
	h.wantHidden(locales.ErrorsEnvReservedKey("OPENBEE_API_KEY"))
	if got := h.fieldText(locales.EnvConfigKey()); got != "OPENBEE_API_KEY2" {
		t.Fatalf("key field = %q after a failed save, want it kept", got)
	}

	h.click(locales.EnvConfigValue())
	h.T.Type("y")
	h.settle()
	h.wantHidden(locales.EnvConfigSaveError())
}

func TestSaveAndAddAnother(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.openAdd()
	h.T.Type("first_key")
	h.click(locales.EnvConfigValue())
	h.T.Type("one")
	h.settle()
	h.click(locales.EnvConfigSaveAndAddAnother())

	h.wantShown(locales.EnvConfigAddDescription(), locales.EnvConfigAdded("FIRST_KEY"), locales.EnvConfigAddedDescription(), "FIRST_KEY")
	if !h.T.Focused(locales.EnvConfigKey()) {
		t.Fatal("the key field does not take the focus after saving")
	}
	if got := h.fieldText(locales.EnvConfigKey()); got != "" {
		t.Fatalf("key field = %q, want it cleared", got)
	}
	h.wantShown(locales.EnvConfigKeyHint())
	h.wantHidden(locales.EnvConfigValueLength(3))
	got := h.posts()
	if len(got) != 1 || got[0]["key"] != "FIRST_KEY" || got[0]["value"] != "one" || got[0]["scope"] != "worker" || got[0]["scope_id"] != workerID {
		t.Fatalf("POST bodies = %v, want FIRST_KEY=one in worker w1", got)
	}

	h.T.Type("first_key")
	h.settle()
	h.wantShown(locales.EnvConfigKeyExists("FIRST_KEY"))
	h.T.Command("selectAll")
	h.T.Type("second_key")
	h.settle()
	h.click(locales.EnvConfigValue())
	h.T.Type("two")
	h.settle()
	h.clickAfter(locales.EnvConfigSaveAndAddAnother())
	h.wantHidden(locales.EnvConfigAddDescription(), locales.EnvConfigAdded("FIRST_KEY"))
	if got := h.serverKeys("worker", workerID); !slices.Equal(got, []string{"DB_URL", "API_KEY", "OLD_TOKEN", "FIRST_KEY", "SECOND_KEY"}) {
		t.Fatalf("server keys = %v", got)
	}
	h.wantShown("FIRST_KEY", "SECOND_KEY")

	h.openAdd()
	h.wantHidden(locales.EnvConfigAdded("FIRST_KEY"), locales.EnvConfigAdded("SECOND_KEY"))
	h.T.Type("draft_key")
	h.click(locales.EnvConfigMultiline())
	h.typeInto(locales.EnvConfigValue(), "draft")
	h.T.Key(0, ui.KeyEscape)
	h.settle()
	h.wantHidden(locales.EnvConfigAddDescription())
	if len(h.posts()) != 2 {
		t.Fatalf("POST calls = %d, want 2 (closing sends nothing)", len(h.posts()))
	}

	h.openAdd()
	if got := h.fieldText(locales.EnvConfigKey()); got != "" {
		t.Fatalf("key field = %q after reopening, want it empty", got)
	}
	h.wantShown(locales.EnvConfigShowValue())
	h.wantHidden(locales.EnvConfigValueLength(5))
}

func TestValueField(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.openAdd()
	h.wantShown(locales.EnvConfigShowValue())
	h.wantHidden(locales.EnvConfigValueLength(0), locales.EnvConfigHideValue())

	h.click(locales.EnvConfigValue())
	h.T.Type("abc")
	h.settle()
	h.wantShown(locales.EnvConfigValueLength(3))
	if got := h.fieldText(locales.EnvConfigValue()); got != "" {
		t.Fatalf("masked value copied as %q, want nothing", got)
	}
	h.click(locales.EnvConfigShowValue())
	h.wantShown(locales.EnvConfigHideValue())
	h.wantHidden(locales.EnvConfigShowValue())
	if got := h.fieldText(locales.EnvConfigValue()); got != "abc" {
		t.Fatalf("shown value = %q, want abc", got)
	}

	h.T.Command("selectAll")
	h.T.Type(" abc")
	h.settle()
	h.wantShown(locales.EnvConfigValueLengthSpace(4))
	h.wantHidden(locales.EnvConfigValueLength(4))
	h.T.Command("selectAll")
	h.T.Type("abc\U0001F600 ")
	h.settle()
	h.wantShown(locales.EnvConfigValueLengthSpace(6))
	h.T.Command("selectAll")
	h.T.Key(0, ui.KeyBackspace)
	h.settle()
	h.wantHidden(locales.EnvConfigValueLength(0), locales.EnvConfigValueLengthSpace(0))

	h.click(locales.EnvConfigMultiline())
	h.wantHidden(locales.EnvConfigShowValue(), locales.EnvConfigHideValue())
	h.click(locales.EnvConfigKey())
	h.T.Type("pem_cert")
	h.click(locales.EnvConfigValue())
	h.T.Type("line one")
	h.T.Key(0, ui.KeyEnter)
	h.T.Type("line two")
	h.settle()
	if len(h.posts()) != 0 {
		t.Fatal("Enter in the multi-line field submitted the form")
	}
	h.wantShown(locales.EnvConfigValueLength(17))
	h.clickAfter(locales.EnvConfigSaveAndAddAnother())
	got := h.posts()
	if len(got) != 1 || got[0]["value"] != "line one\nline two" {
		t.Fatalf("POST bodies = %v, want the two-line value", got)
	}
}

func TestEditVariable(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.wantHidden(locales.EnvConfigEditDescription())
	h.click(locales.EnvConfigEditLabel("DB_URL"))
	h.wantShown(locales.EnvConfigEditDescription(), locales.EnvConfigNewValue(), locales.EnvConfigSaveValue(),
		locales.CommonCancel(), locales.EnvConfigShowValue())
	if !h.T.Focused(locales.EnvConfigNewValue()) {
		t.Fatal("the new value field does not take the focus")
	}
	h.click(locales.EnvConfigSaveValue())
	if len(h.puts("e1")) != 0 {
		t.Fatal("saving an empty value sent a request")
	}
	h.wantShown(locales.EnvConfigEditDescription())

	h.typeInto(locales.EnvConfigNewValue(), "new")
	h.wantShown(locales.EnvConfigValueLength(3))
	h.click(locales.EnvConfigSaveValue())
	got := h.puts("e1")
	if len(got) != 1 || len(got[0]) != 1 || got[0]["value"] != "new" {
		t.Fatalf("PUT bodies = %v, want one {\"value\":\"new\"}", got)
	}
	h.wantHidden(locales.EnvConfigEditDescription())
	h.wantShown("DB_URL", locales.EnvConfigUpdated(locales.TimeJustNow()))
}

func TestEditVariableErrorsAndReset(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.click(locales.EnvConfigEditLabel("API_KEY"))
	h.click(locales.EnvConfigShowValue())
	if got := h.fieldText(locales.EnvConfigNewValue()); got != "" {
		t.Fatalf("new value field = %q, want it empty (the old value is never shown)", got)
	}
	h.T.Type("new")
	h.Fake.FailNext(http.MethodPut, envsPath+"/e2", http.StatusInternalServerError, map[string]string{"error": "no space"})
	h.T.Key(0, ui.KeyEnter)
	h.settle()
	h.wantShown(locales.EnvConfigSaveError(), "no space", locales.EnvConfigEditDescription())
	h.T.Type("er")
	h.settle()
	h.wantHidden(locales.EnvConfigSaveError())
	h.T.Key(0, ui.KeyEnter)
	h.settle()
	if got := h.puts("e2"); len(got) != 2 || got[1]["value"] != "newer" {
		t.Fatalf("PUT bodies = %v, want the retry with newer", got)
	}
	h.wantHidden(locales.EnvConfigEditDescription())

	h.click(locales.EnvConfigEditLabel("OLD_TOKEN"))
	h.wantShown(locales.EnvConfigEditDescription(), locales.EnvConfigShowValue())
	h.wantHidden(locales.EnvConfigHideValue(), locales.EnvConfigValueLength(5), locales.EnvConfigSaveError())
	h.click(locales.CommonCancel())
	h.wantHidden(locales.EnvConfigEditDescription())
}

func TestDeleteVariable(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.click(locales.EnvConfigDeleteLabel("API_KEY"))
	h.wantShown(locales.EnvConfigDeleteTitle("API_KEY"), locales.EnvConfigDeleteConfirm(), locales.CommonCancel())
	h.click(locales.CommonCancel())
	h.wantHidden(locales.EnvConfigDeleteTitle("API_KEY"))
	if got := h.Fake.Calls(http.MethodDelete, envsPath); len(got) != 0 {
		t.Fatalf("cancelling sent %v", got)
	}

	h.click(locales.EnvConfigDeleteLabel("API_KEY"))
	h.Fake.FailNext(http.MethodDelete, envsPath+"/e2", http.StatusInternalServerError, map[string]string{"error": "locked"})
	h.clickAfter(locales.CommonCancel())
	h.wantShown(locales.EnvConfigDeleteError(), "locked", locales.EnvConfigDeleteTitle("API_KEY"))
	h.wantShown(locales.EnvConfigEditLabel("API_KEY"))

	h.clickAfter(locales.CommonCancel())
	if got := h.Fake.Calls(http.MethodDelete, envsPath+"/e2"); len(got) != 2 {
		t.Fatalf("DELETE calls = %d, want 2 (the failure and the retry); texts %q", len(got), h.T.Texts())
	}
	h.wantHidden(locales.EnvConfigDeleteTitle("API_KEY"), locales.EnvConfigEditLabel("API_KEY"), locales.EnvConfigDeleteError())
	h.wantShown("DB_URL", "OLD_TOKEN")
	if got := h.serverKeys("worker", workerID); !slices.Equal(got, []string{"DB_URL", "OLD_TOKEN"}) {
		t.Fatalf("server keys = %v", got)
	}
}

func TestDeleteLastVariableShowsEmptyState(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, seedEnvs(envOf("e1", "worker", workerID, "ONLY", "****", time.Minute)))
	h.click(locales.EnvConfigDeleteLabel("ONLY"))
	h.clickAfter(locales.CommonCancel())
	h.wantShown(locales.EnvConfigEmpty(), locales.EnvConfigEmptyWorker())
	h.wantHidden("ONLY")
}

func TestDialogFooterLayout(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, workerEnvs())
	h.openAdd()
	cr, _ := h.T.Find(locales.CommonCancel())
	ar, _ := h.T.Find(locales.EnvConfigSaveAndAddAnother())
	if cr.Y != ar.Y || cr.X >= ar.X {
		t.Fatalf("wide footer: cancel at %v,%v and save and add another at %v,%v, want one row with cancel on the left", cr.X, cr.Y, ar.X, ar.Y)
	}
	if hint := h.rowY(locales.EnvConfigKeyHint()); ar.Y <= hint {
		t.Fatal("the footer is above the key field")
	}

	h.T.SetSize(narrowW, hostH)
	h.settle()
	cr, _ = h.T.Find(locales.CommonCancel())
	ar, _ = h.T.Find(locales.EnvConfigSaveAndAddAnother())
	if ar.Y >= cr.Y || ar.W != cr.W {
		t.Fatalf("narrow footer: save and add another %+v and cancel %+v, want stacked buttons of equal width with cancel last", ar, cr)
	}
}

func TestEffectiveVarsWorkerOverrides(t *testing.T) {
	h := newHost(t, wideW, "bee", "default", true, seedEnvs(
		envOf("g1", "global", "", "TOKEN", "glob****", time.Minute),
		envOf("g2", "global", "", "DB_URL", "glob-db****", time.Minute),
		envOf("w1", "worker", workerID, "DB_URL", "work-db****", time.Minute),
		envOf("w2", "worker", workerID, "API_KEY", "work-api****", time.Minute),
		envOf("w3", "worker", "w2", "OTHER", "other****", time.Minute),
		envOf("b1", "bee", "default", "BEE_ONLY", "bee****", time.Minute),
	))
	h.effective = true
	h.settle()
	h.wantShown(locales.EnvConfigEffectiveTitle(), locales.WorkerDetailEffectiveEnvDescription(),
		"TOKEN", "glob****", "work-db****", "work-api****", locales.EnvConfigSourceGlobal(), locales.EnvConfigSourceWorker())
	h.wantHidden("glob-db****", "OTHER", "other****")

	last := h.rowY(locales.EnvConfigEffectiveTitle())
	for _, key := range []string{"API_KEY", "DB_URL", "TOKEN"} {
		y := h.rowY(key)
		if y <= last {
			t.Fatalf("effective row %q at y=%v is not below the previous row (y=%v); want the keys sorted", key, y, last)
		}
		last = y
	}
	if work, glob := h.rowY("work-api****"), h.rowY("glob****"); work >= glob {
		t.Fatalf("masked values are not under their keys: work-api at y=%v, glob at y=%v", work, glob)
	}
}

func TestEffectiveVarsEmptyAndLoading(t *testing.T) {
	h := newHost(t, wideW, "worker", workerID, true, seedEnvs(envOf("b1", "bee", "default", "BEE_ONLY", "****", time.Minute)))
	h.effective = true
	h.settle()
	h.wantShown(locales.EnvConfigEffectiveTitle(), locales.EnvConfigNoEffective())

	global := h.Store.Envs("global", "")
	global.Reset()
	global.Loading = true
	h.settle()
	h.wantShown(locales.EnvConfigEffectiveTitle())
	h.wantHidden(locales.EnvConfigNoEffective())
}

func TestEffectiveVarsFollowEdits(t *testing.T) {
	h := newHost(t, wideW, "bee", "default", true, seedEnvs(envOf("g1", "global", "", "TOKEN", "glob****", time.Minute)))
	h.effective = true
	h.settle()
	h.wantShown("glob****", locales.EnvConfigSourceGlobal())
	h.wantHidden(locales.EnvConfigSourceWorker())

	h.Store.CreateEnv(api.CreateEnvRequest{Scope: "worker", ScopeID: workerID, Key: "TOKEN", Value: "worker-token-value"}, nil)
	h.settle()
	h.wantShown("work****alue", locales.EnvConfigSourceWorker())
	h.wantHidden("glob****", locales.EnvConfigSourceGlobal())
}
