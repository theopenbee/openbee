package pluginmgr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/plugin"
)

type fakeInstaller struct {
	h         *harness
	mu        sync.Mutex
	fail      error
	block     chan struct{}
	active    atomic.Int32
	maxActive atomic.Int32
	calls     []string
	onInstall func(spec string)
	removed   []string
	ctxErr    error
}

func (f *fakeInstaller) Install(ctx context.Context, spec string) (plugin.Entry, plugin.Manifest, error) {
	defer func() {
		f.mu.Lock()
		f.ctxErr = ctx.Err()
		f.mu.Unlock()
	}()
	n := f.active.Add(1)
	defer f.active.Add(-1)
	for {
		m := f.maxActive.Load()
		if n <= m || f.maxActive.CompareAndSwap(m, n) {
			break
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, spec)
	fail, block, hook := f.fail, f.block, f.onInstall
	f.mu.Unlock()
	if hook != nil {
		hook(spec)
	}
	if block != nil {
		<-block
	}
	if fail != nil {
		return plugin.Entry{}, plugin.Manifest{}, fail
	}
	name := packageNameFromSpec(spec)
	id := strings.TrimPrefix(name[strings.LastIndex(name, "/")+1:], "channel-")
	version := "1.0.0"
	if i := strings.LastIndex(spec, "@"); i > 0 {
		version = spec[i+1:]
	}
	dir := filepath.Join(f.h.reg.dir, "node_modules", name)
	writeManifest(f.h.t, dir, id, tokenField)
	e := plugin.Entry{ID: id, Source: plugin.SourceNpm, Package: name, Path: dir, Version: version}
	f.h.reg.put(e)
	return e, plugin.Manifest{ID: id}, nil
}

func (f *fakeInstaller) Remove(_ context.Context, id string) (plugin.Entry, error) {
	e, ok, _ := f.h.reg.Get(id)
	if !ok {
		return plugin.Entry{}, plugin.ErrNotInstalled
	}
	f.h.reg.remove(id)
	f.mu.Lock()
	f.removed = append(f.removed, id)
	f.mu.Unlock()
	return e, nil
}

func (h *harness) installNpm(id, version string) {
	h.t.Helper()
	name := "@theopenbee/channel-" + id
	dir := filepath.Join(h.reg.dir, "node_modules", name)
	writeManifest(h.t, dir, id, tokenField)
	h.reg.put(plugin.Entry{ID: id, Source: plugin.SourceNpm, Package: name, Path: dir, Version: version})
}

func (h *harness) waitOp(id string) Operation {
	h.t.Helper()
	var op Operation
	require.Eventually(h.t, func() bool {
		var ok bool
		op, ok = h.m.Operation(id)
		return ok && (op.Status == OpSucceeded || op.Status == OpFailed)
	}, 5*time.Second, 5*time.Millisecond)
	return op
}

func TestPackageNameFromSpec(t *testing.T) {
	cases := map[string]string{
		"@theopenbee/channel-telegram":        "@theopenbee/channel-telegram",
		"@theopenbee/channel-telegram@0.2.0":  "@theopenbee/channel-telegram",
		"@theopenbee/channel-telegram@^0.2.0": "@theopenbee/channel-telegram",
		"openbee-channel-matrix":              "openbee-channel-matrix",
		"openbee-channel-matrix@1.0.0":        "openbee-channel-matrix",
		"github:someone/repo":                 "",
		"someone/repo":                        "",
		"https://example.com/x.tgz":           "",
		"./local/dir":                         "",
		"/abs/dir":                            "",
		"@scope":                              "",
	}
	for spec, want := range cases {
		assert.Equal(t, want, packageNameFromSpec(spec), spec)
	}
}

func TestInstall_NewPluginIsNotStartedWithoutSettings(t *testing.T) {
	h := newHarness(t)
	h.start()
	op := h.m.Install("@theopenbee/channel-telegram@0.1.0")
	assert.Equal(t, "install", op.Kind)
	done := h.waitOp(op.ID)
	assert.Equal(t, OpSucceeded, done.Status)
	assert.Equal(t, "telegram", done.PluginID)
	assert.Equal(t, StateNotConfigured, h.state("telegram").State)
	assert.Equal(t, 0, h.factory.startCount("telegram"))
}

func TestInstall_NewPluginStartsWhenEnabledSettingsExist(t *testing.T) {
	h := newHarness(t)
	h.save("telegram", true, "", map[string]any{"token": "t"})
	h.start()
	done := h.waitOp(h.m.Install("@theopenbee/channel-telegram@0.1.0").ID)
	require.Equal(t, OpSucceeded, done.Status)
	h.waitState("telegram", StateRunning)
}

func TestInstall_UpgradeStopsAndRestarts(t *testing.T) {
	h := newHarness(t)
	h.installNpm("telegram", "0.1.0")
	h.save("telegram", true, "", map[string]any{"token": "t"})
	h.start()
	h.waitState("telegram", StateRunning)

	var stoppedDuringInstall atomic.Bool
	h.inst.onInstall = func(string) {
		_, ok := h.senders.Get("telegram")
		stoppedDuringInstall.Store(!ok)
	}
	done := h.waitOp(h.m.Install("@theopenbee/channel-telegram@0.2.0").ID)
	require.Equal(t, OpSucceeded, done.Status)
	assert.True(t, stoppedDuringInstall.Load())
	h.waitState("telegram", StateRunning)
	assert.Equal(t, 2, h.factory.startCount("telegram"))
	assert.Equal(t, "0.2.0", h.state("telegram").Version)
}

func TestInstall_FailureRestartsOldVersion(t *testing.T) {
	h := newHarness(t)
	h.installNpm("telegram", "0.1.0")
	h.save("telegram", true, "", map[string]any{"token": "t"})
	h.start()
	h.waitState("telegram", StateRunning)

	h.inst.fail = errors.New("npm install failed: ETARGET")
	done := h.waitOp(h.m.Install("@theopenbee/channel-telegram@9.9.9").ID)
	assert.Equal(t, OpFailed, done.Status)
	assert.Contains(t, done.Error, "ETARGET")
	h.waitState("telegram", StateRunning)
	assert.Equal(t, 2, h.factory.startCount("telegram"))
	assert.Equal(t, "0.1.0", h.state("telegram").Version)
}

func TestInstall_Serialized(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.inst.block = make(chan struct{})
	a := h.m.Install("@theopenbee/channel-telegram@0.1.0")
	b := h.m.Install("@theopenbee/channel-discord@0.1.0")
	time.Sleep(50 * time.Millisecond)
	opB, _ := h.m.Operation(b.ID)
	opA, _ := h.m.Operation(a.ID)
	assert.True(t, opA.Status == OpRunning || opB.Status == OpRunning)
	assert.True(t, opA.Status == OpQueued || opB.Status == OpQueued)
	close(h.inst.block)
	assert.Equal(t, OpSucceeded, h.waitOp(a.ID).Status)
	assert.Equal(t, OpSucceeded, h.waitOp(b.ID).Status)
	assert.Equal(t, int32(1), h.inst.maxActive.Load())
}

func TestUninstall_DeletesSettingsAndDataDir(t *testing.T) {
	h := newHarness(t)
	h.installNpm("telegram", "0.1.0")
	h.save("telegram", true, "TBot", map[string]any{"token": "t"})
	h.start()
	h.waitState("telegram", StateRunning)
	require.NoError(t, os.WriteFile(filepath.Join(h.reg.DataDir("telegram"), "state.json"), []byte("{}"), 0o600))

	op, err := h.m.Uninstall(context.Background(), "telegram")
	require.NoError(t, err)
	assert.Equal(t, "uninstall", op.Kind)
	assert.Equal(t, OpSucceeded, h.waitOp(op.ID).Status)

	_, ok, _ := h.reg.Get("telegram")
	assert.False(t, ok)
	_, found, err := h.store.Get(context.Background(), "telegram")
	require.NoError(t, err)
	assert.False(t, found)
	assert.NoDirExists(t, h.reg.DataDir("telegram"))
	_, ok = h.senders.Get("telegram")
	assert.False(t, ok)
	assert.Empty(t, h.m.BotNames())
}

func TestUninstall_NotInstalled(t *testing.T) {
	h := newHarness(t)
	h.start()
	_, err := h.m.Uninstall(context.Background(), "nope")
	assert.Equal(t, CodeNotFound, apperr.Code(err))
}

func TestOperations_KeepsLast50(t *testing.T) {
	h := newHarness(t)
	h.start()
	var ids []string
	for i := 0; i < 55; i++ {
		ids = append(ids, h.m.Install(fmt.Sprintf("@theopenbee/channel-p%d@1.0.0", i)).ID)
	}
	h.waitOp(ids[54])
	_, ok := h.m.Operation(ids[0])
	assert.False(t, ok)
	_, ok = h.m.Operation(ids[5])
	assert.True(t, ok)
}

func TestOperation_NotFound(t *testing.T) {
	h := newHarness(t)
	_, ok := h.m.Operation("missing")
	assert.False(t, ok)
}

func TestInstall_UpgradeShowsStartingWhileInstalling(t *testing.T) {
	h := newHarness(t)
	h.installNpm("telegram", "0.1.0")
	h.save("telegram", true, "", map[string]any{"token": "t"})
	h.start()
	h.waitState("telegram", StateRunning)

	var during atomic.Value
	h.inst.onInstall = func(string) {
		list, err := h.m.List(context.Background())
		if err == nil && len(list) == 1 {
			during.Store(list[0].State)
		}
	}
	require.Equal(t, OpSucceeded, h.waitOp(h.m.Install("@theopenbee/channel-telegram@0.2.0").ID).Status)
	assert.Equal(t, StateStarting, during.Load())
}

func TestInstall_NotCancelledByShutdown(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	h.m.Start(ctx, func(platform.InboundMessage) {}, nil)
	h.inst.block = make(chan struct{})
	op := h.m.Install("@theopenbee/channel-telegram@0.1.0")
	require.Eventually(t, func() bool { o, _ := h.m.Operation(op.ID); return o.Status == OpRunning }, 2*time.Second, 5*time.Millisecond)

	cancel()
	wctx, wcancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer wcancel()
	assert.ErrorIs(t, h.m.Wait(wctx), context.DeadlineExceeded)

	close(h.inst.block)
	require.NoError(t, h.m.Wait(context.Background()))
	done, _ := h.m.Operation(op.ID)
	assert.Equal(t, OpSucceeded, done.Status)
	h.inst.mu.Lock()
	assert.NoError(t, h.inst.ctxErr)
	h.inst.mu.Unlock()
}

func TestManager_NoStartAfterShutdown(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.save("telegram", false, "", map[string]any{"token": "t"})
	ctx, cancel := context.WithCancel(context.Background())
	h.m.Start(ctx, func(platform.InboundMessage) {}, nil)
	cancel()
	require.NoError(t, h.m.Wait(context.Background()))

	assert.Error(t, h.m.Enable(context.Background(), "telegram"))
	assert.Equal(t, 0, h.factory.startCount("telegram"))
}

func TestManager_OperationsNewestFirst(t *testing.T) {
	h := newHarness(t)
	h.start()
	first := h.m.Install("@theopenbee/channel-telegram@0.1.0")
	second := h.m.Install("@theopenbee/channel-discord@0.1.0")
	h.waitOp(first.ID)
	h.waitOp(second.ID)
	ops := h.m.Operations()
	require.Len(t, ops, 2)
	assert.Equal(t, second.ID, ops[0].ID)
	assert.Equal(t, first.ID, ops[1].ID)
}
