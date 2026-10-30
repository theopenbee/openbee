package route_test

import (
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/views/route"
)

func TestConversationID(t *testing.T) {
	cases := map[string]string{
		"bee":       "bee",
		"w1":        "worker:w1",
		"":          "",
		"worker:w1": "",
	}
	for param, want := range cases {
		if got := route.ConversationID(param); got != want {
			t.Errorf("ConversationID(%q) = %q, want %q", param, got, want)
		}
	}
}

func TestParam(t *testing.T) {
	cases := map[string]string{
		"bee":         "bee",
		"worker:w1":   "w1",
		"":            "",
		"worker:":     "",
		"worker:a:b":  "",
		"unknown:w1":  "",
		"worker:w-42": "w-42",
	}
	for cid, want := range cases {
		if got := route.Param(cid); got != want {
			t.Errorf("Param(%q) = %q, want %q", cid, got, want)
		}
	}
	for _, param := range []string{"bee", "w1", "w-42"} {
		if got := route.Param(route.ConversationID(param)); got != param {
			t.Errorf("Param(ConversationID(%q)) = %q", param, got)
		}
	}
}

func TestChatPaths(t *testing.T) {
	cases := []struct{ got, want string }{
		{route.Chat(""), "/chat"},
		{route.Chat("bee"), "/chat/bee"},
		{route.Chat("w1"), "/chat/w1"},
		{route.Chat("a b"), "/chat/a%20b"},
		{route.ChatPanel("w9", route.PanelConstraints), "/chat/w9?panel=constraints"},
		{route.ChatPanel("bee", route.PanelNew), "/chat/bee?panel=new"},
		{route.ChatPanel("", route.PanelNew), "/chat?panel=new"},
		{route.ChatPanel("w1", ""), "/chat/w1"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("path = %q, want %q", c.got, c.want)
		}
	}
}

func TestParsePanelRoundTrip(t *testing.T) {
	states := []route.Panel{
		{},
		{Mode: route.ModeWorker, Tab: route.PanelConstraints},
		{Mode: route.ModeWorker, Tab: route.PanelPermissions},
		{Mode: route.ModeWorker, Tab: route.PanelEnv},
		{Mode: route.ModeNew},
	}
	for _, state := range states {
		value := state.Tab
		if state.Mode == route.ModeNew {
			value = route.PanelNew
		}
		if got := route.ParsePanel("w1", value); got != state {
			t.Errorf("ParsePanel(w1, %q) = %+v, want %+v", value, got, state)
		}
	}
}

func TestParsePanelUnknownIsClosed(t *testing.T) {
	for _, value := range []string{"", "tasks", "overview", "sessions"} {
		if got := route.ParsePanel("w1", value); got != (route.Panel{}) {
			t.Errorf("ParsePanel(w1, %q) = %+v, want closed", value, got)
		}
	}
}

func TestParsePanelNewOnAnyChat(t *testing.T) {
	for _, param := range []string{"bee", "w1", ""} {
		if got := route.ParsePanel(param, route.PanelNew); got != (route.Panel{Mode: route.ModeNew}) {
			t.Errorf("ParsePanel(%q, new) = %+v, want the new worker form", param, got)
		}
	}
}

func TestParsePanelWorkerTabsOnlyInWorkerChats(t *testing.T) {
	want := route.Panel{Mode: route.ModeWorker, Tab: route.PanelEnv}
	if got := route.ParsePanel("w1", route.PanelEnv); got != want {
		t.Errorf("ParsePanel(w1, env) = %+v, want %+v", got, want)
	}
	for _, param := range []string{"bee", ""} {
		if got := route.ParsePanel(param, route.PanelConstraints); got != (route.Panel{}) {
			t.Errorf("ParsePanel(%q, constraints) = %+v, want closed", param, got)
		}
	}
}

func TestIsTabSwitch(t *testing.T) {
	constraints := route.Panel{Mode: route.ModeWorker, Tab: route.PanelConstraints}
	env := route.Panel{Mode: route.ModeWorker, Tab: route.PanelEnv}
	created := route.Panel{Mode: route.ModeNew}
	cases := []struct {
		from, to route.Panel
		want     bool
	}{
		{constraints, env, true},
		{route.Panel{}, constraints, false},
		{constraints, route.Panel{}, false},
		{constraints, created, false},
		{created, constraints, false},
	}
	for _, c := range cases {
		if got := route.IsTabSwitch(c.from, c.to); got != c.want {
			t.Errorf("IsTabSwitch(%+v, %+v) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}
