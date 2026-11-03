package pluginmgr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/pluginchannel"
	"github.com/theopenbee/openbee/internal/plugin"
)

type fakeRegistry struct {
	mu      sync.Mutex
	dir     string
	entries map[string]plugin.Entry
}

func (r *fakeRegistry) List() ([]plugin.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]plugin.Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeRegistry) Get(id string) (plugin.Entry, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	return e, ok, nil
}

func (r *fakeRegistry) DataDir(id string) string { return filepath.Join(r.dir, "data", id) }

func (r *fakeRegistry) put(e plugin.Entry) {
	r.mu.Lock()
	r.entries[e.ID] = e
	r.mu.Unlock()
}

func (r *fakeRegistry) remove(id string) {
	r.mu.Lock()
	delete(r.entries, id)
	r.mu.Unlock()
}

type fakeSender struct {
	ctx context.Context
}

func (s fakeSender) Send(ctx context.Context, _ platform.OutboundMessage) error {
	select {
	case <-s.ctx.Done():
		return errors.New("plugin stopped")
	case <-ctx.Done():
		return ctx.Err()
	}
}

type fakeInstance struct {
	f        *fakeFactory
	spec     InstanceSpec
	ctx      context.Context
	cancel   context.CancelFunc
	sendCtx  context.Context
	finished chan struct{}
}

func (i *fakeInstance) Sender() platform.PlatformSenderAdapter { return fakeSender{ctx: i.sendCtx} }

func (i *fakeInstance) Run(ctx context.Context, _ func(platform.InboundMessage)) {
	defer close(i.finished)
	i.spec.OnState(pluginchannel.StateStarting, nil)
	if err := i.f.failWith(i.spec.ID); err != nil {
		i.spec.OnState(pluginchannel.StateFailed, err)
		return
	}
	i.spec.OnState(pluginchannel.StateRunning, nil)
	<-ctx.Done()
	i.f.mu.Lock()
	delay := i.f.stopDelay
	i.f.mu.Unlock()
	time.Sleep(delay)
	i.cancel()
	i.spec.OnState(pluginchannel.StateStopped, nil)
}

type fakeFactory struct {
	mu        sync.Mutex
	starts    map[string]int
	specs     map[string]InstanceSpec
	instances []*fakeInstance
	fail      map[string]error
	stopDelay time.Duration
}

func newFakeFactory() *fakeFactory {
	return &fakeFactory{starts: map[string]int{}, specs: map[string]InstanceSpec{}, fail: map[string]error{}}
}

func (f *fakeFactory) build(spec InstanceSpec) Instance {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts[spec.ID]++
	f.specs[spec.ID] = spec
	sendCtx, cancel := context.WithCancel(context.Background())
	inst := &fakeInstance{f: f, spec: spec, sendCtx: sendCtx, cancel: cancel, finished: make(chan struct{})}
	f.instances = append(f.instances, inst)
	return inst
}

func (f *fakeFactory) failWith(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fail[id]
}

func (f *fakeFactory) startCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts[id]
}

func (f *fakeFactory) spec(id string) InstanceSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.specs[id]
}

type harness struct {
	scripts  map[string]string
	t        *testing.T
	m        *Manager
	inst     *fakeInstaller
	reg      *fakeRegistry
	store    *store.PluginSettingsStore
	codec    *Codec
	factory  *fakeFactory
	engines  *fakeEngineFactory
	stateDir string
	senders  *platform.SenderRegistry
	workers  map[string]bool
	botNames chan map[string]string
	cancel   context.CancelFunc
}

func writeManifest(t *testing.T, dir string, id string, fields []plugin.ConfigField) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := json.Marshal(map[string]any{
		"protocol_version": 1,
		"id":               id,
		"name":             map[string]string{"en": id + " name"},
		"command":          []string{"node", "index.js"},
		"config":           fields,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, plugin.ManifestFile), data, 0o644))
}

var tokenField = []plugin.ConfigField{{Key: "token", Type: plugin.FieldSecret, Required: true}}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db, err := store.InitDB(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	codec, err := NewCodec(testSecret, true)
	require.NoError(t, err)
	h := &harness{
		t:        t,
		reg:      &fakeRegistry{dir: t.TempDir(), entries: map[string]plugin.Entry{}},
		store:    store.NewPluginSettingsStore(db),
		codec:    codec,
		factory:  newFakeFactory(),
		engines:  newFakeEngineFactory(),
		stateDir: t.TempDir(),
		senders:  platform.NewSenderRegistry(),
		workers:  map[string]bool{},
		botNames: make(chan map[string]string, 256),
	}
	h.inst = &fakeInstaller{h: h}
	h.scripts = map[string]string{}
	h.m = New(Deps{
		SetupCommand: func(ctx context.Context, man plugin.Manifest, lang string) (*exec.Cmd, error) {
			cmd := exec.CommandContext(ctx, "sh", "-c", h.scripts[man.ID])
			cmd.Env = append(os.Environ(), "OPENBEE_LANGUAGE="+lang)
			return cmd, nil
		},
		Language:      "zh",
		Registry:      h.reg,
		Installer:     h.inst,
		Store:         h.store,
		Codec:         h.codec,
		Senders:       h.senders,
		WrapSender:    func(_ string, s platform.PlatformSenderAdapter) platform.PlatformSenderAdapter { return s },
		Factory:       h.factory.build,
		EngineFactory: h.engines.build,
		StateDir: func(id string) string {
			return filepath.Join(h.stateDir, "."+id)
		},
		WorkerNameTaken: func(name string) (bool, error) {
			return h.workers[name], nil
		},
	})
	return h
}

func (h *harness) install(id string) {
	h.t.Helper()
	dir := filepath.Join(h.reg.dir, "pkgs", id)
	writeManifest(h.t, dir, id, tokenField)
	h.reg.put(plugin.Entry{ID: id, Source: plugin.SourcePath, Path: dir, Version: "1.0.0"})
}

func (h *harness) save(id string, enabled bool, botName string, cfg map[string]any) {
	h.t.Helper()
	row, err := h.codec.Encode(Settings{Enabled: enabled, BotName: botName, Config: cfg}, id)
	require.NoError(h.t, err)
	require.NoError(h.t, h.store.Upsert(context.Background(), row))
}

func (h *harness) start() {
	h.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.m.Start(ctx, func(platform.InboundMessage) {}, func(names map[string]string) { h.botNames <- names })
	h.t.Cleanup(func() {
		cancel()
		wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer wcancel()
		assert.NoError(h.t, h.m.Wait(wctx))
	})
}

func (h *harness) state(id string) Status {
	h.t.Helper()
	list, err := h.m.List(context.Background())
	require.NoError(h.t, err)
	for _, s := range list {
		if s.ID == id {
			return s
		}
	}
	h.t.Fatalf("plugin %s not listed", id)
	return Status{}
}

func (h *harness) waitState(id string, want State) {
	h.t.Helper()
	require.Eventually(h.t, func() bool { return h.state(id).State == want }, 5*time.Second, 5*time.Millisecond, "waiting for %s to be %s", id, want)
}

func TestManager_StartRunsEnabledOnly(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.install("discord")
	h.install("lark")
	h.save("telegram", true, "", map[string]any{"token": "t"})
	h.save("discord", false, "", map[string]any{"token": "d"})
	h.start()

	h.waitState("telegram", StateRunning)
	assert.Equal(t, StateDisabled, h.state("discord").State)
	assert.Equal(t, StateNotConfigured, h.state("lark").State)
	assert.Equal(t, 1, h.factory.startCount("telegram"))
	assert.Equal(t, 0, h.factory.startCount("discord"))
	_, ok := h.senders.Get("telegram")
	assert.True(t, ok)
	_, ok = h.senders.Get("discord")
	assert.False(t, ok)
	assert.Equal(t, "t", h.factory.spec("telegram").Config["token"])
	assert.DirExists(t, h.reg.DataDir("telegram"))
}

func TestManager_EnableRequiresConfig(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.start()

	err := h.m.Enable(context.Background(), "telegram")
	assert.Equal(t, CodeNotConfigured, apperr.Code(err))

	err = h.m.Enable(context.Background(), "missing")
	assert.Equal(t, CodeNotFound, apperr.Code(err))

	require.NoError(t, h.m.UpdateSettings(context.Background(), "telegram", nil, map[string]any{"token": "t"}))
	assert.Equal(t, StateDisabled, h.state("telegram").State)
	require.NoError(t, h.m.Enable(context.Background(), "telegram"))
	h.waitState("telegram", StateRunning)
	assert.True(t, h.state("telegram").Enabled)
}

func TestManager_UpdateSettingsRestartsOnlyThatPlugin(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.install("discord")
	h.save("telegram", true, "", map[string]any{"token": "t1"})
	h.save("discord", true, "", map[string]any{"token": "d1"})
	h.start()
	h.waitState("telegram", StateRunning)
	h.waitState("discord", StateRunning)

	require.NoError(t, h.m.UpdateSettings(context.Background(), "telegram", nil, map[string]any{"token": "t2"}))
	h.waitState("telegram", StateRunning)
	assert.Equal(t, 2, h.factory.startCount("telegram"))
	assert.Equal(t, 1, h.factory.startCount("discord"))
	assert.Equal(t, "t2", h.factory.spec("telegram").Config["token"])
}

func TestManager_UpdateSettingsOnDisabledPluginDoesNotStart(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.start()
	require.NoError(t, h.m.UpdateSettings(context.Background(), "telegram", nil, map[string]any{"token": "t"}))
	assert.Equal(t, 0, h.factory.startCount("telegram"))
}

func TestManager_DisableRemovesSenderAndStops(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.save("telegram", true, "", map[string]any{"token": "t"})
	h.start()
	h.waitState("telegram", StateRunning)

	require.NoError(t, h.m.Disable(context.Background(), "telegram"))
	assert.Equal(t, StateDisabled, h.state("telegram").State)
	assert.False(t, h.state("telegram").Enabled)
	_, ok := h.senders.Get("telegram")
	assert.False(t, ok)
	h.factory.mu.Lock()
	inst := h.factory.instances[0]
	h.factory.mu.Unlock()
	select {
	case <-inst.finished:
	case <-time.After(time.Second):
		t.Fatal("instance still running after disable")
	}
}

func TestManager_StatesFollowInstance(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.save("telegram", true, "", map[string]any{"token": "t"})
	h.factory.fail["telegram"] = errors.New("plugin rejected its config: bad token")
	h.start()

	h.waitState("telegram", StateFailed)
	assert.Contains(t, h.state("telegram").Error, "bad token")

	h.factory.mu.Lock()
	delete(h.factory.fail, "telegram")
	h.factory.mu.Unlock()
	require.NoError(t, h.m.Restart(context.Background(), "telegram"))
	h.waitState("telegram", StateRunning)
	assert.Empty(t, h.state("telegram").Error)
}

func TestManager_DecryptFailureMarksFailed(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	other, err := NewCodec("ff02030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20", true)
	require.NoError(t, err)
	row, err := other.Encode(Settings{Enabled: true, Config: map[string]any{"token": "t"}}, "telegram")
	require.NoError(t, err)
	require.NoError(t, h.store.Upsert(context.Background(), row))
	h.install("discord")
	h.save("discord", true, "", map[string]any{"token": "d"})
	h.start()

	h.waitState("discord", StateRunning)
	st := h.state("telegram")
	assert.Equal(t, StateFailed, st.State)
	assert.Contains(t, st.Error, "env_secret")
	assert.Equal(t, 0, h.factory.startCount("telegram"))
}

func TestManager_BotNameConflictWithWorker(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.workers["alice"] = true
	h.start()

	name := "alice"
	err := h.m.UpdateSettings(context.Background(), "telegram", &name, map[string]any{"token": "t"})
	assert.Equal(t, CodeBotNameConflict, apperr.Code(err))

	name = "bee"
	require.NoError(t, h.m.UpdateSettings(context.Background(), "telegram", &name, map[string]any{"token": "t"}))
	assert.Equal(t, "bee", h.state("telegram").BotName)
}

func TestManager_BotNamesCallbackOnChange(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.save("telegram", false, "TBot", map[string]any{"token": "t"})
	h.start()

	assert.Equal(t, map[string]string{"telegram": "TBot"}, <-h.botNames)
	assert.Equal(t, map[string]string{"telegram": "TBot"}, h.m.BotNames())
	assert.Equal(t, []string{"TBot"}, h.m.BotNameList())

	name := "Bee"
	require.NoError(t, h.m.UpdateSettings(context.Background(), "telegram", &name, nil))
	assert.Equal(t, map[string]string{"telegram": "Bee"}, <-h.botNames)
}

func TestManager_UpdateSettingsUnknownPlugin(t *testing.T) {
	h := newHarness(t)
	h.start()
	err := h.m.UpdateSettings(context.Background(), "nope", nil, map[string]any{"token": "t"})
	assert.Equal(t, CodeNotFound, apperr.Code(err))
}

func TestManager_GetMasksSecrets(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.save("telegram", false, "", map[string]any{"token": "123456:ABCDEFGH"})
	h.start()

	d, err := h.m.Get(context.Background(), "telegram")
	require.NoError(t, err)
	assert.Equal(t, "****EFGH", d.Config["token"])
	require.Len(t, d.Fields, 1)
	assert.Equal(t, "token", d.Fields[0].Key)
	assert.Equal(t, "telegram name", d.Name.Get("en"))
}

func TestManager_DisableDuringInFlightSend(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.save("telegram", true, "", map[string]any{"token": "t"})
	h.start()
	h.waitState("telegram", StateRunning)

	sender, ok := h.senders.Get("telegram")
	require.True(t, ok)
	sendErr := make(chan error, 1)
	go func() { sendErr <- sender.Send(context.Background(), platform.OutboundMessage{}) }()

	require.NoError(t, h.m.Disable(context.Background(), "telegram"))
	select {
	case err := <-sendErr:
		assert.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight send did not return after disable")
	}
}

func TestManager_UpdateSettingsSurvivesCancelledRequest(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.save("telegram", true, "", map[string]any{"token": "t1"})
	h.start()
	h.waitState("telegram", StateRunning)
	h.factory.mu.Lock()
	h.factory.stopDelay = 200 * time.Millisecond
	h.factory.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	require.NoError(t, h.m.UpdateSettings(ctx, "telegram", nil, map[string]any{"token": "t2"}))
	h.waitState("telegram", StateRunning)
	assert.Equal(t, "t2", h.factory.spec("telegram").Config["token"])
	_, ok := h.senders.Get("telegram")
	assert.True(t, ok)
}

func TestManager_RestartSurvivesCancelledRequest(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.save("telegram", true, "", map[string]any{"token": "t1"})
	h.start()
	h.waitState("telegram", StateRunning)
	h.factory.mu.Lock()
	h.factory.stopDelay = 200 * time.Millisecond
	h.factory.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	require.NoError(t, h.m.Restart(ctx, "telegram"))
	h.waitState("telegram", StateRunning)
	assert.Equal(t, 2, h.factory.startCount("telegram"))
}
