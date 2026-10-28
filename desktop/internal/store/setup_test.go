package store_test

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
)

const (
	setupPlugin = "wechat"
	scanLine    = "Open WeChat and scan the code"
	firstLink   = "https://login.example.com/qr/1"
	secondLink  = "https://login.example.com/qr/2"
)

func withSetupPlugin(st *fakeapi.State) {
	team(st)
	st.Plugins = []api.PluginDetail{{PluginStatus: api.PluginStatus{ID: setupPlugin, Package: "@openbee/plugin-wechat", Source: "npm",
		Name: api.LocalizedText{"en": "WeChat"}, State: "not_configured", HasSetup: true}}}
}

func setupEvents(sid string) string {
	return "/api/plugins/setup/" + sid + "/events"
}

func setupLine(text string) api.SetupEvent {
	return api.SetupEvent{Type: "line", Text: text}
}

func connectedSetup(t *testing.T, fix *storetest.Fixture, fake *fakeapi.Server, s *store.SetupSession) {
	t.Helper()
	fix.Await(t, func() bool { return s.ID != "" && fake.SetupStreams(s.ID) == 1 })
}

func startedSetup(t *testing.T) (*store.PluginsState, *fakeapi.Server, *storetest.Fixture, *store.SetupSession) {
	t.Helper()
	st, fake, fix := loaded(t, withSetupPlugin)
	p := st.Plugins()
	s := p.StartSetup(setupPlugin, nil)
	if !s.Starting || s.Active {
		t.Fatalf("session before the start request returned = %+v, want starting and not active", s)
	}
	connectedSetup(t, fix, fake, s)
	if s.Starting || !s.Active || s.ID != fake.SetupSession(setupPlugin) {
		t.Fatalf("session after start = %+v, want active with the server's id %q", s, fake.SetupSession(setupPlugin))
	}
	return p, fake, fix, s
}

func TestSetupReplaysClearLines(t *testing.T) {
	_, fake, fix, s := startedSetup(t)
	fake.SetupEmit(s.ID, setupLine(scanLine))
	fake.SetupEmit(s.ID, setupLine(firstLink))
	fix.Await(t, func() bool { return len(s.Lines) == 2 })
	if s.Reconnecting {
		t.Fatal("reconnecting while the stream is open")
	}

	fake.DropSetupStreams(s.ID)
	fix.Await(t, func() bool { return s.Reconnecting })
	fake.SetupEmit(s.ID, setupLine(secondLink))
	armed(t, fix, 1)
	fix.Clock.Advance(time.Second)
	fix.Await(t, func() bool { return !s.Reconnecting && len(s.Lines) >= 3 })
	fix.Settle(t)

	if want := []string{scanLine, firstLink, secondLink}; !slices.Equal(s.Lines, want) {
		t.Fatalf("lines after the replay = %q, want %q", s.Lines, want)
	}
	if n := countCalls(fake, http.MethodGet, setupEvents(s.ID)); n != 2 {
		t.Fatalf("event stream connections = %d, want 2", n)
	}
}

func TestSetupGivesUpAfterFive(t *testing.T) {
	_, fake, fix, s := startedSetup(t)
	fake.SetupEmit(s.ID, setupLine(scanLine))
	fix.Await(t, func() bool { return len(s.Lines) == 1 })

	path := setupEvents(s.ID)
	for range 5 {
		fake.FailNext(http.MethodGet, path, http.StatusServiceUnavailable, map[string]string{"error": "setup is busy"})
	}
	fake.DropSetupStreams(s.ID)
	for _, delay := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second} {
		fix.Await(t, func() bool { return s.Reconnecting })
		armed(t, fix, 1)
		n := countCalls(fake, http.MethodGet, path)
		fix.Clock.Advance(delay)
		fix.Await(t, func() bool { return countCalls(fake, http.MethodGet, path) > n })
	}
	fix.Await(t, func() bool { return s.Lost })
	fix.Settle(t)

	if s.Reconnecting || s.Done || !s.Active {
		t.Fatalf("session after giving up = %+v, want lost, still active and not reconnecting", s)
	}
	if n := countCalls(fake, http.MethodGet, path); n != 6 {
		t.Fatalf("event stream connections = %d, want 6", n)
	}
	if n := fix.Clock.Sleepers(); n != 0 {
		t.Fatalf("%d reconnect timers still sleeping after giving up", n)
	}
}

func TestSetupDoneOK(t *testing.T) {
	st, fake, fix := loaded(t, withSetupPlugin)
	p := st.Plugins()
	p.List.Ensure()
	fix.Settle(t)
	var updates []store.SetupSession
	s := p.StartSetup(setupPlugin, func(s *store.SetupSession) { updates = append(updates, *s) })
	connectedSetup(t, fix, fake, s)
	fake.SetupEmit(s.ID, setupLine(firstLink))
	fix.Await(t, func() bool { return len(s.Lines) == 1 })
	lists := countCalls(fake, http.MethodGet, pluginsPath)

	fake.SetupEmit(s.ID, api.SetupEvent{Type: "done", OK: true})
	fix.Await(t, func() bool { return s.Done })
	fix.Await(t, func() bool { return countCalls(fake, http.MethodGet, pluginsPath) > lists })
	fix.Settle(t)

	if !s.OK || s.Active || s.Reconnecting || s.Lost || s.Err != "" {
		t.Fatalf("session after done = %+v, want ok and inactive", s)
	}
	if last := updates[len(updates)-1]; !last.Done || !last.OK {
		t.Fatalf("last update = %+v, want the done state", last)
	}
	if n := countCalls(fake, http.MethodGet, setupEvents(s.ID)); n != 1 {
		t.Fatalf("event stream connections = %d, want no reconnect after done", n)
	}
	if n := fix.Clock.Sleepers(); n != 0 {
		t.Fatalf("%d reconnect timers sleeping after done", n)
	}

	p.CancelSetup(s)
	fix.Settle(t)
	if n := countCalls(fake, http.MethodDelete, "/api/plugins/setup/"+s.ID); n != 0 {
		t.Fatalf("cancel after done sent %d DELETE requests, want none", n)
	}
}

func TestSetupStartAgainCancelsPrevious(t *testing.T) {
	p, fake, fix, first := startedSetup(t)
	firstID := first.ID
	second := p.StartSetup(setupPlugin, nil)
	connectedSetup(t, fix, fake, second)
	fix.Settle(t)

	if second.ID == firstID {
		t.Fatalf("second session reused id %q", firstID)
	}
	if n := countCalls(fake, http.MethodDelete, "/api/plugins/setup/"+firstID); n != 1 {
		t.Fatalf("DELETE for the first session = %d, want 1", n)
	}
	if first.Done || first.Err != "" || len(first.Lines) != 0 {
		t.Fatalf("first session changed after it was replaced: %+v", first)
	}

	release := fake.HoldNext(http.MethodPost, pluginsPath+"/"+setupPlugin+"/setup")
	third := p.StartSetup(setupPlugin, nil)
	fix.Pump(t, func() bool { return len(fake.Calls(http.MethodPost, pluginsPath+"/"+setupPlugin+"/setup")) == 3 })
	p.CancelSetup(third)
	release()
	fix.Await(t, func() bool { return fake.SetupSession(setupPlugin) != second.ID })
	late := fake.SetupSession(setupPlugin)
	fix.Await(t, func() bool { return countCalls(fake, http.MethodDelete, "/api/plugins/setup/"+late) == 1 })
	if third.ID != "" || third.Active {
		t.Fatalf("session cancelled while starting = %+v, want it left without an id", third)
	}
}
