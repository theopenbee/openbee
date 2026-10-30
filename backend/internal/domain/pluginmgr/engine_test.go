package pluginmgr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/ai/pluginengine"
	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/plugin"
)

type fakeEngineRunner struct {
	f    *fakeEngineFactory
	spec pluginengine.Spec
}

func (r *fakeEngineRunner) Run(context.Context, string, string, ai.RunOptions, string) (ai.RunResult, error) {
	return ai.RunResult{}, errors.New("fake engine does not run")
}

func (r *fakeEngineRunner) CollectTokenUsage(context.Context, []string) (map[string][]ai.TokenUsage, error) {
	return nil, nil
}

func (r *fakeEngineRunner) Check(ctx context.Context) (pluginengine.CheckResult, error) {
	r.f.mu.Lock()
	r.f.checks[r.spec.ID]++
	err, block := r.f.checkErr[r.spec.ID], r.f.block
	r.f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return pluginengine.CheckResult{}, ctx.Err()
		}
	}
	if err != nil {
		return pluginengine.CheckResult{}, err
	}
	return pluginengine.CheckResult{Version: "1.0.0", Path: "/usr/local/bin/" + r.spec.ID}, nil
}

type fakeEngineFactory struct {
	mu       sync.Mutex
	specs    map[string][]pluginengine.Spec
	checks   map[string]int
	checkErr map[string]error
	fields   map[string][]plugin.ConfigField
	block    chan struct{}
}

func newFakeEngineFactory() *fakeEngineFactory {
	return &fakeEngineFactory{
		specs:    map[string][]pluginengine.Spec{},
		checks:   map[string]int{},
		checkErr: map[string]error{},
		fields:   map[string][]plugin.ConfigField{},
	}
}

func (f *fakeEngineFactory) build(spec pluginengine.Spec) EngineRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.specs[spec.ID] = append(f.specs[spec.ID], spec)
	return &fakeEngineRunner{f: f, spec: spec}
}

func (f *fakeEngineFactory) builds(id string) []pluginengine.Spec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]pluginengine.Spec(nil), f.specs[id]...)
}

func (f *fakeEngineFactory) checkCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checks[id]
}

func (f *fakeEngineFactory) setCheckErr(id string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.checkErr, id)
		return
	}
	f.checkErr[id] = err
}

func (f *fakeEngineFactory) setBlock(ch chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.block = ch
}

func (f *fakeEngineFactory) fieldsFor(id string) []plugin.ConfigField {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fields[id]
}

func writeEngineManifest(t *testing.T, dir string, id string, fields []plugin.ConfigField) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := json.Marshal(map[string]any{
		"protocol_version": plugin.EngineProtocolVersion,
		"type":             plugin.TypeEngine,
		"id":               id,
		"name":             map[string]string{"en": id + " engine", "zh": id + " 引擎"},
		"command":          []string{"node", "dist/index.js"},
		"config":           fields,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, plugin.ManifestFile), data, 0o644))
}

var enginePathField = []plugin.ConfigField{{Key: "path", Type: plugin.FieldString}}

func (h *harness) installEngine(id string, fields []plugin.ConfigField) {
	h.t.Helper()
	dir := filepath.Join(h.reg.dir, "pkgs", id)
	writeEngineManifest(h.t, dir, id, fields)
	h.reg.put(plugin.Entry{ID: id, Source: plugin.SourcePath, Path: dir, Version: "0.1.0"})
}

func (h *harness) loadEngines() {
	h.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h.m.LoadEngines(ctx)
	h.t.Cleanup(func() {
		cancel()
		wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer wcancel()
		assert.NoError(h.t, h.m.Wait(wctx))
	})
}

func (h *harness) settings(id string) (Settings, bool) {
	h.t.Helper()
	row, found, err := h.store.Get(context.Background(), id)
	require.NoError(h.t, err)
	if !found {
		return Settings{}, false
	}
	s, err := h.codec.Decode(row)
	require.NoError(h.t, err)
	return s, true
}

func (h *harness) engineRegistered(id string) bool {
	_, ok := h.m.Engine(id)
	return ok
}

func TestEngine_InstallAutoEnables(t *testing.T) {
	h := newHarness(t)
	h.engines.fields["needs"] = []plugin.ConfigField{{Key: "api_key", Type: plugin.FieldSecret, Required: true}}
	h.start()

	done := h.waitOp(h.m.Install("@theopenbee/engine-pi@0.1.0").ID)
	require.Equal(t, OpSucceeded, done.Status)
	assert.Equal(t, "pi", done.PluginID)
	h.waitState("pi", StateReady)

	s, found := h.settings("pi")
	require.True(t, found)
	assert.True(t, s.Enabled)
	assert.True(t, h.state("pi").Enabled)
	assert.True(t, h.engineRegistered("pi"))
	assert.Equal(t, []string{"pi"}, h.m.EnabledEngineIDs())
	assert.Equal(t, []ai.EngineInfo{{ID: "pi", Name: map[string]string{"en": "pi engine", "zh": "pi 引擎"}}}, h.m.Engines())
	assert.Equal(t, 0, h.factory.startCount("pi"))
	_, ok := h.senders.Get("pi")
	assert.False(t, ok)

	done = h.waitOp(h.m.Install("@theopenbee/engine-needs@0.1.0").ID)
	require.Equal(t, OpSucceeded, done.Status)
	assert.Equal(t, StateNotConfigured, h.state("needs").State)
	_, found = h.settings("needs")
	assert.False(t, found)
	assert.False(t, h.engineRegistered("needs"))
	assert.Empty(t, h.engines.builds("needs"))
}

func TestEngine_ReinstallKeepsDisabled(t *testing.T) {
	h := newHarness(t)
	h.start()
	require.Equal(t, OpSucceeded, h.waitOp(h.m.Install("@theopenbee/engine-codex@0.1.0").ID).Status)
	h.waitState("codex", StateReady)
	require.NoError(t, h.m.Disable(context.Background(), "codex"))

	require.Equal(t, OpSucceeded, h.waitOp(h.m.Install("@theopenbee/engine-codex@0.2.0").ID).Status)
	st := h.state("codex")
	assert.Equal(t, StateDisabled, st.State)
	assert.False(t, st.Enabled)
	assert.Equal(t, "0.2.0", st.Version)
	assert.False(t, h.engineRegistered("codex"))
	assert.Len(t, h.engines.builds("codex"), 1)
}

func TestEngine_UpgradeRechecksEnabledEngine(t *testing.T) {
	h := newHarness(t)
	h.start()
	require.Equal(t, OpSucceeded, h.waitOp(h.m.Install("@theopenbee/engine-codex@0.1.0").ID).Status)
	h.waitState("codex", StateReady)

	require.Equal(t, OpSucceeded, h.waitOp(h.m.Install("@theopenbee/engine-codex@0.2.0").ID).Status)
	h.waitState("codex", StateReady)
	assert.True(t, h.engineRegistered("codex"))
	assert.Len(t, h.engines.builds("codex"), 2)
	assert.Equal(t, 2, h.engines.checkCount("codex"))
}

func TestEngine_FailedUpgradeReregisters(t *testing.T) {
	h := newHarness(t)
	h.start()
	require.Equal(t, OpSucceeded, h.waitOp(h.m.Install("@theopenbee/engine-codex@0.1.0").ID).Status)
	h.waitState("codex", StateReady)

	h.inst.fail = errors.New("npm install failed: ETARGET")
	assert.Equal(t, OpFailed, h.waitOp(h.m.Install("@theopenbee/engine-codex@9.9.9").ID).Status)
	h.waitState("codex", StateReady)
	assert.True(t, h.engineRegistered("codex"))
	assert.Equal(t, "0.1.0", h.state("codex").Version)
	assert.Len(t, h.engines.builds("codex"), 2)
}

func TestEngine_CheckReadyState(t *testing.T) {
	h := newHarness(t)
	h.installEngine("claude", enginePathField)
	h.save("claude", true, "", map[string]any{})
	block := make(chan struct{})
	h.engines.setBlock(block)
	h.loadEngines()

	st := h.state("claude")
	assert.Equal(t, StateStarting, st.State)
	assert.Equal(t, plugin.TypeEngine, st.Type)
	assert.True(t, h.engineRegistered("claude"))

	close(block)
	h.waitState("claude", StateReady)
	assert.Empty(t, h.state("claude").Error)
	assert.Equal(t, 1, h.engines.checkCount("claude"))
}

func TestEngine_CheckFailedState(t *testing.T) {
	h := newHarness(t)
	h.installEngine("claude", enginePathField)
	h.save("claude", true, "", map[string]any{})
	h.engines.setCheckErr("claude", errors.New("claude not found on PATH"))
	h.loadEngines()

	h.waitState("claude", StateFailed)
	assert.Equal(t, "claude not found on PATH", h.state("claude").Error)
	adapter, ok := h.m.Engine("claude")
	require.True(t, ok)
	assert.NotNil(t, adapter)
	assert.Equal(t, []string{"claude"}, h.m.EnabledEngineIDs())

	h.engines.setCheckErr("claude", nil)
	require.NoError(t, h.m.Restart(context.Background(), "claude"))
	h.waitState("claude", StateReady)
	assert.Empty(t, h.state("claude").Error)
}

func TestEngine_DisableUnregisters(t *testing.T) {
	h := newHarness(t)
	h.installEngine("pi", nil)
	h.installEngine("codex", nil)
	h.save("pi", true, "", map[string]any{})
	h.save("codex", true, "", map[string]any{})
	h.loadEngines()
	h.waitState("pi", StateReady)
	h.waitState("codex", StateReady)
	assert.Equal(t, []string{"codex", "pi"}, h.m.EnabledEngineIDs())

	require.NoError(t, h.m.Disable(context.Background(), "pi"))
	assert.False(t, h.engineRegistered("pi"))
	assert.True(t, h.engineRegistered("codex"))
	assert.Equal(t, []string{"codex"}, h.m.EnabledEngineIDs())
	st := h.state("pi")
	assert.Equal(t, StateDisabled, st.State)
	assert.False(t, st.Enabled)

	require.NoError(t, h.m.Enable(context.Background(), "pi"))
	h.waitState("pi", StateReady)
	assert.Equal(t, []string{"codex", "pi"}, h.m.EnabledEngineIDs())
}

func TestEngine_EnableWithoutSettingsSavesDefaults(t *testing.T) {
	h := newHarness(t)
	h.installEngine("pi", enginePathField)
	h.installEngine("needs", []plugin.ConfigField{{Key: "api_key", Type: plugin.FieldSecret, Required: true}})
	h.start()
	assert.Equal(t, StateNotConfigured, h.state("pi").State)

	require.NoError(t, h.m.Enable(context.Background(), "pi"))
	h.waitState("pi", StateReady)
	s, found := h.settings("pi")
	require.True(t, found)
	assert.True(t, s.Enabled)
	assert.True(t, h.engineRegistered("pi"))

	err := h.m.Enable(context.Background(), "needs")
	assert.Equal(t, CodeNotConfigured, apperr.Code(err))
	_, found = h.settings("needs")
	assert.False(t, found)
	assert.False(t, h.engineRegistered("needs"))
}

func TestEngine_UninstallKeepsStateDir(t *testing.T) {
	h := newHarness(t)
	h.installNpm("telegram", "0.1.0")
	h.start()
	require.Equal(t, OpSucceeded, h.waitOp(h.m.Install("@theopenbee/engine-codex@0.1.0").ID).Status)
	h.waitState("codex", StateReady)

	stateDir := filepath.Join(h.stateDir, ".codex")
	dataDir := h.reg.DataDir("codex")
	require.DirExists(t, stateDir)
	require.DirExists(t, dataDir)
	sessionFile := filepath.Join(stateDir, "sessions", "thread.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(sessionFile), 0o700))
	require.NoError(t, os.WriteFile(sessionFile, []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "cache.json"), []byte("{}"), 0o600))

	op, err := h.m.Uninstall(context.Background(), "codex")
	require.NoError(t, err)
	require.Equal(t, OpSucceeded, h.waitOp(op.ID).Status)

	assert.FileExists(t, sessionFile)
	assert.NoDirExists(t, dataDir)
	_, found := h.settings("codex")
	assert.False(t, found)
	assert.False(t, h.engineRegistered("codex"))
	assert.Empty(t, h.m.EnabledEngineIDs())
	_, ok, _ := h.reg.Get("codex")
	assert.False(t, ok)
}

func TestEngine_RejectsBotName(t *testing.T) {
	h := newHarness(t)
	h.installEngine("pi", enginePathField)
	h.save("pi", true, "", map[string]any{})
	h.loadEngines()
	h.start()
	h.waitState("pi", StateReady)

	name := "PiBot"
	err := h.m.UpdateSettings(context.Background(), "pi", &name, nil)
	assert.Equal(t, CodeBotNameNotSupported, apperr.Code(err))
	assert.Equal(t, "plugin_bot_name_not_supported", CodeBotNameNotSupported)
	assert.Empty(t, h.state("pi").BotName)
	assert.Len(t, h.engines.builds("pi"), 1)

	empty := " "
	require.NoError(t, h.m.UpdateSettings(context.Background(), "pi", &empty, map[string]any{"path": "/opt/pi"}))
	assert.Empty(t, h.state("pi").BotName)
	assert.Empty(t, h.m.BotNames())
}

func TestEngine_ListHasType(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.installEngine("pi", nil)
	brokenDir := filepath.Join(h.reg.dir, "pkgs", "broken")
	require.NoError(t, os.MkdirAll(brokenDir, 0o755))
	h.reg.put(plugin.Entry{ID: "broken", Source: plugin.SourcePath, Path: brokenDir, Version: "1.0.0"})
	h.start()

	types := map[string]string{}
	list, err := h.m.List(context.Background())
	require.NoError(t, err)
	for _, st := range list {
		types[st.ID] = st.Type
	}
	assert.Equal(t, map[string]string{"telegram": plugin.TypeChannel, "pi": plugin.TypeEngine, "broken": plugin.TypeChannel}, types)

	d, err := h.m.Get(context.Background(), "pi")
	require.NoError(t, err)
	assert.Equal(t, plugin.TypeEngine, d.Type)
	assert.False(t, d.HasSetup)
}

func TestEngine_SettingsChangeRebuildsAdapter(t *testing.T) {
	h := newHarness(t)
	h.installEngine("claude", enginePathField)
	h.save("claude", true, "", map[string]any{})
	h.loadEngines()
	h.start()
	h.waitState("claude", StateReady)
	first, ok := h.m.Engine("claude")
	require.True(t, ok)

	require.NoError(t, h.m.UpdateSettings(context.Background(), "claude", nil, map[string]any{"path": "/opt/bin/claude"}))
	h.waitState("claude", StateReady)

	specs := h.engines.builds("claude")
	require.Len(t, specs, 2)
	assert.Nil(t, specs[0].Config["path"])
	spec := specs[1]
	assert.Equal(t, "/opt/bin/claude", spec.Config["path"])
	assert.Equal(t, "claude", spec.ID)
	assert.Equal(t, filepath.Join(h.reg.dir, "pkgs", "claude"), spec.Dir)
	assert.Equal(t, []string{"node", "dist/index.js"}, spec.Command)
	assert.Equal(t, h.reg.DataDir("claude"), spec.DataDir)
	assert.Equal(t, filepath.Join(h.stateDir, ".claude"), spec.StateDir)
	assert.Equal(t, "zh", spec.Language)
	assert.Equal(t, 2, h.engines.checkCount("claude"))

	second, ok := h.m.Engine("claude")
	require.True(t, ok)
	assert.NotSame(t, first, second)
}

func TestEngine_StartSkipsEngines(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.installEngine("pi", nil)
	h.save("telegram", true, "", map[string]any{"token": "t"})
	h.save("pi", true, "", map[string]any{})
	h.start()

	h.waitState("telegram", StateRunning)
	assert.Equal(t, 0, h.factory.startCount("pi"))
	assert.Empty(t, h.engines.builds("pi"))
	assert.False(t, h.engineRegistered("pi"))
	_, ok := h.senders.Get("pi")
	assert.False(t, ok)
}

func TestLoadEngines_Synchronous(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.installEngine("pi", nil)
	h.installEngine("claude", enginePathField)
	h.installEngine("codex", nil)
	h.save("telegram", true, "", map[string]any{"token": "t"})
	h.save("pi", true, "", map[string]any{})
	h.save("claude", true, "", map[string]any{"path": "/usr/bin/claude"})
	h.save("codex", false, "", map[string]any{})
	block := make(chan struct{})
	h.engines.setBlock(block)
	h.loadEngines()

	assert.Equal(t, []string{"claude", "pi"}, h.m.EnabledEngineIDs())
	assert.True(t, h.engineRegistered("pi"))
	assert.True(t, h.engineRegistered("claude"))
	assert.False(t, h.engineRegistered("codex"))
	engines := h.m.Engines()
	require.Len(t, engines, 2)
	assert.Equal(t, "claude", engines[0].ID)
	assert.Equal(t, "pi", engines[1].ID)
	assert.Equal(t, StateStarting, h.state("pi").State)
	assert.Equal(t, 0, h.factory.startCount("telegram"))
	assert.Equal(t, "/usr/bin/claude", h.engines.builds("claude")[0].Config["path"])

	close(block)
	h.waitState("pi", StateReady)
	h.waitState("claude", StateReady)
}

func TestLoadEngines_WaitCoversChecks(t *testing.T) {
	h := newHarness(t)
	h.installEngine("pi", nil)
	h.save("pi", true, "", map[string]any{})
	h.engines.setBlock(make(chan struct{}))
	ctx, cancel := context.WithCancel(context.Background())
	h.m.LoadEngines(ctx)

	wctx, wcancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer wcancel()
	assert.ErrorIs(t, h.m.Wait(wctx), context.DeadlineExceeded)

	cancel()
	require.NoError(t, h.m.Wait(context.Background()))
}
