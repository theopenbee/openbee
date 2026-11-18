package views_test

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/settings"
	"github.com/theopenbee/openbee/desktop/internal/views/shortcuts"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

func press(h *testkit.Harness, s shortcuts.Shortcut) {
	h.T.Key(s.Mods, s.Key)
	h.Motion()
}

func wantPath(t *testing.T, h *testkit.Harness, want string) {
	t.Helper()
	if got := h.Root.Nav().Path(); got != want {
		t.Fatalf("path = %q, want %q; texts %q", got, want, h.T.Texts())
	}
}

func paletteOpen(h *testkit.Harness) bool {
	return h.T.Focused(locales.QuickSearchTitle())
}

func search(t *testing.T, h *testkit.Harness, query string) {
	t.Helper()
	press(h, shortcuts.QuickSearch)
	if !paletteOpen(h) {
		t.Fatalf("quick search did not open with its input focused; texts %q", h.T.Texts())
	}
	h.T.Type(query)
	h.Settle()
}

func TestQuickSearchOpensAConversation(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, staffed)
	search(t, h, "伟")
	h.Shot("quick-search")
	if !h.T.HasText("后端开发与接口维护") || h.T.HasText("Data analysis and monthly reports") {
		t.Fatalf("searching 伟 should list only 张伟; texts %q", h.T.Texts())
	}
	h.T.Key(0, ui.KeyEnter)
	h.Motion()
	wantPath(t, h, route.Chat("zhang"))
	if paletteOpen(h) {
		t.Fatal("quick search stayed open after choosing a conversation")
	}
	if !h.T.Focused(locales.ChatMessageLabel()) {
		t.Fatalf("the composer of the chosen conversation is not focused; texts %q", h.T.Texts())
	}
}

func TestQuickSearchEscapeClearsThenCloses(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, staffed)
	search(t, h, "nothing like this")
	if !h.T.HasText(locales.ArcCommandPaletteEmpty("nothing like this")) {
		t.Fatalf("no empty state for a query without results; texts %q", h.T.Texts())
	}
	h.T.Key(0, ui.KeyEscape)
	h.Settle()
	if !paletteOpen(h) || h.T.HasText(locales.ArcCommandPaletteEmpty("nothing like this")) {
		t.Fatalf("the first Escape should clear the query and keep the palette; texts %q", h.T.Texts())
	}
	h.T.Key(0, ui.KeyEscape)
	h.Motion()
	if h.T.HasText(locales.QuickSearchGroupsConversations()) {
		t.Fatalf("the second Escape should close the palette; texts %q", h.T.Texts())
	}
}

func TestQuickSearchTogglesWithCommandK(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, staffed)
	press(h, shortcuts.QuickSearch)
	if !paletteOpen(h) {
		t.Fatalf("⌘K did not open quick search; texts %q", h.T.Texts())
	}
	press(h, shortcuts.QuickSearch)
	if h.T.HasText(locales.QuickSearchGroupsConversations()) {
		t.Fatalf("⌘K again did not close quick search; texts %q", h.T.Texts())
	}
}

func TestQuickSearchOpensSettingsAndPanels(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, staffed)
	h.Root.Nav().Push(route.Chat("ada"))
	h.Settle()
	search(t, h, locales.ChatPanelTabsPermissions())
	h.T.Key(0, ui.KeyEnter)
	h.Motion()
	wantPath(t, h, route.Chat("ada"))
	if got := h.Root.Nav().Query(route.PanelQuery); got != route.PanelPermissions {
		t.Fatalf("panel = %q, want %q", got, route.PanelPermissions)
	}
	search(t, h, locales.AboutTitle())
	h.T.Key(0, ui.KeyEnter)
	h.Motion()
	wantPath(t, h, route.Settings(settings.SectionAbout, ""))
	search(t, h, "hop")
	h.T.Key(0, ui.KeyEnter)
	h.Motion()
	wantPath(t, h, route.Chat("grace"))
}

func TestStepThroughConversations(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, staffed)
	wantPath(t, h, route.Chat("bee"))
	press(h, shortcuts.NextConversation)
	wantPath(t, h, route.Chat("zhang"))
	if !h.T.Focused(locales.ChatMessageLabel()) {
		t.Fatal("the composer is not focused after switching")
	}
	press(h, shortcuts.NextConversation)
	wantPath(t, h, route.Chat("ada"))
	press(h, shortcuts.PreviousConversation)
	press(h, shortcuts.PreviousConversation)
	wantPath(t, h, route.Chat("bee"))
	press(h, shortcuts.PreviousConversation)
	wantPath(t, h, route.Chat("bee"))
}

func TestNewConversationShortcutAsksFirst(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, staffed)
	press(h, shortcuts.NewConversation)
	if !h.T.HasText(locales.ChatNewConversationConfirmBee()) {
		t.Fatalf("⇧⌘O did not ask to start a new conversation; texts %q", h.T.Texts())
	}
	press(h, shortcuts.NextConversation)
	wantPath(t, h, route.Chat("bee"))
	press(h, shortcuts.QuickSearch)
	if paletteOpen(h) {
		t.Fatal("quick search opened over the confirmation dialog")
	}
}

func TestShortcutListSwapsWithQuickSearch(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, staffed)
	press(h, shortcuts.ShowShortcuts)
	if !h.T.HasText(locales.ShortcutsPreviousConversation()) {
		t.Fatalf("⌘/ did not show the shortcut list; texts %q", h.T.Texts())
	}
	h.Shot("shortcut-list")
	press(h, shortcuts.QuickSearch)
	if h.T.HasText(locales.ShortcutsPreviousConversation()) || !paletteOpen(h) {
		t.Fatalf("⌘K in the shortcut list should swap to quick search; texts %q", h.T.Texts())
	}
	press(h, shortcuts.ShowShortcuts)
	if paletteOpen(h) || !h.T.HasText(locales.ShortcutsPreviousConversation()) {
		t.Fatalf("⌘/ in quick search should swap to the shortcut list; texts %q", h.T.Texts())
	}
	press(h, shortcuts.ShowShortcuts)
	if h.T.HasText(locales.ShortcutsPreviousConversation()) {
		t.Fatalf("⌘/ again did not close the shortcut list; texts %q", h.T.Texts())
	}
}
