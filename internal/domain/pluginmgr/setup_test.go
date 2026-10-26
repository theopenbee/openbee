package pluginmgr

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/plugin"
)

func (h *harness) installWithSetup(id string) {
	h.t.Helper()
	dir := filepath.Join(h.reg.dir, "pkgs", id)
	require.NoError(h.t, os.MkdirAll(dir, 0o755))
	data, err := json.Marshal(map[string]any{
		"protocol_version": 1,
		"id":               id,
		"command":          []string{"node", "index.js"},
		"setup":            map[string]any{"command": []string{"node", "setup.js"}, "label": map[string]string{"en": "QR login"}},
		"config":           []plugin.ConfigField{{Key: "token", Type: plugin.FieldSecret, Required: true}, {Key: "region", Type: plugin.FieldString, Required: true}},
	})
	require.NoError(h.t, err)
	require.NoError(h.t, os.WriteFile(filepath.Join(dir, plugin.ManifestFile), data, 0o644))
	h.reg.put(plugin.Entry{ID: id, Source: plugin.SourcePath, Path: dir})
}

func collect(t *testing.T, m *Manager, sid string, timeout time.Duration) []SetupEvent {
	t.Helper()
	history, ch, unsubscribe, err := m.SubscribeSetup(sid)
	require.NoError(t, err)
	defer unsubscribe()
	events := append([]SetupEvent{}, history...)
	deadline := time.After(timeout)
	for {
		if len(events) > 0 && events[len(events)-1].Type == "done" {
			return events
		}
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-deadline:
			t.Fatalf("setup did not finish, events: %+v", events)
		}
	}
}

func lines(events []SetupEvent) []string {
	var out []string
	for _, e := range events {
		if e.Type == "line" {
			out = append(out, e.Text)
		}
	}
	return out
}

func TestSetup_SuccessSavesValuesWithoutEnabling(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `echo "scan the code ($OPENBEE_LANGUAGE)" >&2; echo "https://example.com/q/1" >&2; printf '{"token":"tok-1"}\n'`
	h.start()

	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	events := collect(t, h.m, sid, 5*time.Second)
	assert.Equal(t, []string{"scan the code (zh)", "https://example.com/q/1"}, lines(events))
	done := events[len(events)-1]
	assert.True(t, done.OK, done.Error)

	row, found, err := h.store.Get(context.Background(), "weixin")
	require.NoError(t, err)
	require.True(t, found)
	assert.False(t, row.Enabled)
	settings, err := h.codec.Decode(row)
	require.NoError(t, err)
	assert.Equal(t, "tok-1", settings.Config["token"])
	assert.Equal(t, StateNotConfigured, h.state("weixin").State)
	assert.Equal(t, 0, h.factory.startCount("weixin"))
}

func TestSetup_EnabledPluginRestarts(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.save("weixin", true, "", map[string]any{"token": "old", "region": "cn"})
	h.scripts["weixin"] = `printf '{"token":"new"}'`
	h.start()
	h.waitState("weixin", StateRunning)

	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	events := collect(t, h.m, sid, 5*time.Second)
	require.True(t, events[len(events)-1].OK)
	h.waitState("weixin", StateRunning)
	assert.Equal(t, 2, h.factory.startCount("weixin"))
	assert.Equal(t, "new", h.factory.spec("weixin").Config["token"])
	assert.Equal(t, "cn", h.factory.spec("weixin").Config["region"])
}

func TestSetup_PromptWithoutNewlineAndInput(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `printf "enter the number: " >&2; read code; echo "got $code" >&2; printf '{"token":"t"}'`
	h.start()

	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	history, ch, unsubscribe, err := h.m.SubscribeSetup(sid)
	require.NoError(t, err)
	defer unsubscribe()
	seen := lines(history)
	deadline := time.After(5 * time.Second)
	for !contains(seen, "enter the number:") {
		select {
		case ev := <-ch:
			if ev.Type == "line" {
				seen = append(seen, ev.Text)
			}
		case <-deadline:
			t.Fatalf("prompt never arrived: %v", seen)
		}
	}
	require.NoError(t, h.m.SetupInput(sid, "1234"))
	events := collect(t, h.m, sid, 5*time.Second)
	assert.Contains(t, lines(events), "got 1234")
	assert.True(t, events[len(events)-1].OK)
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestSetup_FailureReportsLastLine(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `echo "first" >&2; echo "boom happened" >&2; exit 3`
	h.start()

	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	events := collect(t, h.m, sid, 5*time.Second)
	done := events[len(events)-1]
	assert.False(t, done.OK)
	assert.Contains(t, done.Error, "boom happened")
	_, found, err := h.store.Get(context.Background(), "weixin")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestSetup_InvalidOutput(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `echo "not json"`
	h.start()
	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	events := collect(t, h.m, sid, 5*time.Second)
	assert.False(t, events[len(events)-1].OK)
	assert.Contains(t, events[len(events)-1].Error, "JSON")
}

func TestSetup_CancelStopsProcess(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `echo "waiting" >&2; sleep 30`
	h.start()
	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	require.NoError(t, h.m.CancelSetup(sid))
	events := collect(t, h.m, sid, 5*time.Second)
	assert.Less(t, time.Since(start), 4*time.Second)
	assert.False(t, events[len(events)-1].OK)
}

func TestSetup_ReplayAfterDone(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `echo "one" >&2; echo "two" >&2; printf '{"token":"t"}'`
	h.start()
	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	collect(t, h.m, sid, 5*time.Second)
	history, ch, unsubscribe, err := h.m.SubscribeSetup(sid)
	require.NoError(t, err)
	defer unsubscribe()
	assert.Equal(t, []string{"one", "two"}, lines(history))
	assert.Equal(t, "done", history[len(history)-1].Type)
	_, open := <-ch
	assert.False(t, open)
	assert.Equal(t, CodeSetupFinished, apperr.Code(h.m.SetupInput(sid, "x")))
}

func TestSetup_UnknownSession(t *testing.T) {
	h := newHarness(t)
	h.start()
	_, _, _, err := h.m.SubscribeSetup("nope")
	assert.Equal(t, CodeSetupNotFound, apperr.Code(err))
	assert.Equal(t, CodeSetupNotFound, apperr.Code(h.m.SetupInput("nope", "1")))
	assert.Equal(t, CodeSetupNotFound, apperr.Code(h.m.CancelSetup("nope")))
}

func TestSetup_SecondStartCancelsFirst(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `sleep 30`
	h.start()
	first, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	h.scripts["weixin"] = `printf '{"token":"t"}'`
	second, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
	assert.False(t, collect(t, h.m, first, 5*time.Second)[0].OK)
	events := collect(t, h.m, second, 5*time.Second)
	assert.True(t, events[len(events)-1].OK)
}

func TestSetup_DropsTerminalQRLines(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `printf '\033[32mgreen text\033[0m\n' >&2; printf '█▀▀▀█ ▄▄ █▀▀▀█\n' >&2; printf '▀▀▀▀▀ ▀▀ ▀▀▀▀▀\n' >&2; printf '\n' >&2; echo "real line" >&2; printf 'spin 1\rspin 2\rspinning done\n' >&2; printf '{"token":"t"}'`
	h.start()
	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	assert.Equal(t, []string{"green text", "real line", "spinning done"}, lines(collect(t, h.m, sid, 5*time.Second)))
}

func TestSetup_PluginWithoutSetup(t *testing.T) {
	h := newHarness(t)
	h.install("telegram")
	h.start()
	_, err := h.m.StartSetup(context.Background(), "telegram")
	assert.Equal(t, CodeNoSetup, apperr.Code(err))
	_, err = h.m.StartSetup(context.Background(), "missing")
	assert.Equal(t, CodeNotFound, apperr.Code(err))
}

func TestSetup_UninstallCancelsSetup(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `echo "waiting" >&2; sleep 30`
	h.start()
	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond)

	op, err := h.m.Uninstall(context.Background(), "weixin")
	require.NoError(t, err)
	assert.Equal(t, OpSucceeded, h.waitOp(op.ID).Status)
	events := collect(t, h.m, sid, time.Second)
	assert.False(t, events[len(events)-1].OK)
}

func TestSetup_ShutdownStopsSetup(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `echo "waiting" >&2; sleep 30`
	h.start()
	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	time.Sleep(200 * time.Millisecond)

	h.cancel()
	events := collect(t, h.m, sid, 4*time.Second)
	assert.False(t, events[len(events)-1].OK)
}

func TestSetup_ConcurrentStartsKeepOneSession(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `sleep 30`
	h.start()

	ids := make([]string, 3)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sid, err := h.m.StartSetup(context.Background(), "weixin")
			assert.NoError(t, err)
			ids[i] = sid
		}(i)
	}
	wg.Wait()

	running := 0
	for _, sid := range ids {
		history, _, unsubscribe, err := h.m.SubscribeSetup(sid)
		require.NoError(t, err)
		unsubscribe()
		if len(history) == 0 || history[len(history)-1].Type != "done" {
			running++
		}
	}
	assert.Equal(t, 1, running)
}

func TestSetup_BackgroundChildDoesNotBlockResult(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.scripts["weixin"] = `sleep 30 >&2 & printf '{"token":"t"}'`
	h.start()
	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	events := collect(t, h.m, sid, 6*time.Second)
	done := events[len(events)-1]
	assert.True(t, done.OK, done.Error)
}

func TestSetup_SavedValuesReportSuccessWhenRestartFails(t *testing.T) {
	h := newHarness(t)
	h.installWithSetup("weixin")
	h.save("weixin", true, "", map[string]any{"token": "old"})
	h.scripts["weixin"] = `printf '{"token":"new"}'`
	h.start()

	sid, err := h.m.StartSetup(context.Background(), "weixin")
	require.NoError(t, err)
	events := collect(t, h.m, sid, 5*time.Second)
	done := events[len(events)-1]
	assert.True(t, done.OK, done.Error)
	row, _, err := h.store.Get(context.Background(), "weixin")
	require.NoError(t, err)
	settings, err := h.codec.Decode(row)
	require.NoError(t, err)
	assert.Equal(t, "new", settings.Config["token"])
}
