package search

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/settings"
)

const (
	larkIcon  = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M0 0h24v24H0z"/></svg>`
	slackIcon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M12 2l11 19H1z"/></svg>`
)

func input() Input {
	return Input{
		Conversations: []api.ConversationSummary{
			{ID: api.BeeConversation},
			{ID: api.WorkerConversation("w1"), WorkerID: "w1"},
			{ID: api.WorkerConversation("gone"), WorkerID: "gone"},
		},
		Workers: map[string]api.Worker{"w1": {ID: "w1", Name: "Qiao", Description: "Builds\nopenbee", Engine: "claude"}},
		Lang:    "zh",
		Plugins: []api.PluginStatus{{ID: "lark", Name: api.LocalizedText{"en": "Lark", "zh": "飞书"}, Icon: larkIcon}, {ID: "bare"}, {ID: "slack"}},
		Catalog: []api.CatalogPlugin{{ID: "bare"}, {ID: "slack", Icon: slackIcon}},
	}
}

func targets(entries []Entry, kind Kind) []string {
	var out []string
	for _, e := range entries {
		if e.Target.Kind == kind {
			out = append(out, e.Target.Value)
		}
	}
	return out
}

func TestEntriesListVisibleConversationsFirst(t *testing.T) {
	entries := Entries(input())
	if got := targets(entries, KindConversation); !slices.Equal(got, []string{api.BeeConversation, api.WorkerConversation("w1")}) {
		t.Fatalf("conversations = %q, want Bee and the worker that still exists", got)
	}
	if !entries[0].Item.Logo || entries[1].Item.Description != "Builds openbee" || !slices.Equal(entries[1].Item.Keywords, []string{"claude"}) {
		t.Fatalf("conversation items = %+v", entries[:2])
	}
}

func TestEntriesOfferNewConversationOnlyInAChat(t *testing.T) {
	in := input()
	if got := targets(Entries(in), KindNewConversation); len(got) != 0 {
		t.Fatal("new conversation offered outside a chat")
	}
	in.CanStartOver = true
	if got := targets(Entries(in), KindNewConversation); len(got) != 1 {
		t.Fatal("new conversation missing in a chat")
	}
}

func TestEntriesAddPanelsForTheOpenWorker(t *testing.T) {
	in := input()
	in.Current = api.BeeConversation
	if got := targets(Entries(in), KindPanel); len(got) != 0 {
		t.Fatalf("panels for Bee = %q, want none", got)
	}
	in.Current = api.WorkerConversation("w1")
	want := []string{route.PanelConstraints, route.PanelPermissions, route.PanelEnv}
	if got := targets(Entries(in), KindPanel); !slices.Equal(got, want) {
		t.Fatalf("panels = %q, want %q", got, want)
	}
}

func TestEntriesListSettingsAndPlugins(t *testing.T) {
	entries := Entries(input())
	want := []string{settings.SectionEnv, settings.SectionSystem, settings.SectionPlugins, settings.SectionAbout}
	if got := targets(entries, KindSettings); !slices.Equal(got, want) {
		t.Fatalf("settings = %q, want %q", got, want)
	}
	var names []string
	var glyphs []*ui.SVG
	for _, e := range entries {
		if e.Target.Kind == KindPlugin {
			names = append(names, e.Item.Label)
			glyphs = append(glyphs, e.Item.Glyph)
			if e.Item.Icon != "puzzle" {
				t.Errorf("plugin %q falls back to %q, want the puzzle icon", e.Item.Label, e.Item.Icon)
			}
		}
	}
	if !slices.Equal(names, []string{"飞书", "bare", "slack"}) {
		t.Fatalf("plugin names = %q", names)
	}
	if glyphs[0] != icons.Plugin(larkIcon) || glyphs[1] != nil || glyphs[2] != icons.Plugin(slackIcon) {
		t.Errorf("plugin glyphs = %v, want the lark logo, none and the catalog logo of slack", glyphs)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.Item.ID] {
			t.Fatalf("duplicate item id %q", e.Item.ID)
		}
		seen[e.Item.ID] = true
	}
}
