package search

import (
	"cmp"
	"strconv"
	"strings"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/settings"
	"github.com/theopenbee/openbee/desktop/internal/views/shortcuts"
	"github.com/theopenbee/openbee/desktop/locales"
)

type Kind int

const (
	KindConversation Kind = iota
	KindPanel
	KindNewWorker
	KindNewConversation
	KindShortcuts
	KindSettings
	KindPlugin
)

type Target struct {
	Kind  Kind
	Value string
}

type Entry struct {
	Item   kit.PaletteItem
	Target Target
}

type Input struct {
	Conversations []api.ConversationSummary
	Workers       map[string]api.Worker
	Current       string
	Plugins       []api.PluginStatus
	Catalog       []api.CatalogPlugin
	Lang          string
	CanStartOver  bool
}

var panels = []struct {
	tab, icon string
	label     func() string
}{
	{route.PanelConstraints, "list-checks", locales.ChatPanelTabsConstraints},
	{route.PanelPermissions, "shield-check", locales.ChatPanelTabsPermissions},
	{route.PanelEnv, "variable", locales.ChatPanelTabsEnv},
}

var sections = []struct {
	id, icon string
	label    func() string
}{
	{settings.SectionEnv, "key-round", locales.EnvConfigNavTitle},
	{settings.SectionSystem, "sliders-horizontal", locales.SystemSettingsTitle},
	{settings.SectionPlugins, "puzzle", locales.PluginsTitle},
	{settings.SectionAbout, "info", locales.AboutTitle},
}

func Entries(in Input) []Entry {
	var out []Entry
	add := func(target Target, item kit.PaletteItem) {
		item.ID = itemID(target)
		out = append(out, Entry{Item: item, Target: target})
	}
	conversations := locales.QuickSearchGroupsConversations()
	for _, conv := range in.Conversations {
		target := Target{KindConversation, conv.ID}
		if conv.ID == api.BeeConversation {
			add(target, kit.PaletteItem{Label: locales.ChatBeeLabel(), Group: conversations, Logo: true})
			continue
		}
		w, ok := in.Workers[conv.WorkerID]
		if !ok {
			continue
		}
		item := kit.PaletteItem{Label: w.Name, Description: strings.Join(strings.Fields(w.Description), " "), Group: conversations, Icon: "bot"}
		if w.Engine != "" {
			item.Keywords = []string{w.Engine}
		}
		add(target, item)
	}
	actions := locales.QuickSearchGroupsActions()
	if in.CanStartOver {
		add(Target{Kind: KindNewConversation}, kit.PaletteItem{Label: locales.ChatNewConversation(), Group: actions, Icon: "square-pen",
			Keys: shortcuts.NewConversation.Caps()})
	}
	add(Target{Kind: KindNewWorker}, kit.PaletteItem{Label: locales.ChatNewWorker(), Group: actions, Icon: "plus",
		Keys: shortcuts.NewWorker.Caps()})
	add(Target{Kind: KindShortcuts}, kit.PaletteItem{Label: locales.ShortcutsTitle(), Group: actions, Icon: "keyboard",
		Keys: shortcuts.ShowShortcuts.Caps()})
	if id, isWorker := api.ConversationWorkerID(in.Current); isWorker {
		if w, ok := in.Workers[id]; ok {
			for _, p := range panels {
				add(Target{KindPanel, p.tab}, kit.PaletteItem{Label: p.label(), Group: w.Name, Icon: p.icon, Keywords: []string{w.Name}})
			}
		}
	}
	group := locales.QuickSearchGroupsSettings()
	for _, s := range sections {
		item := kit.PaletteItem{Label: s.label(), Group: group, Icon: s.icon, Keywords: []string{locales.NavSettings()}}
		if s.id == settings.SectionEnv {
			item.Keys = shortcuts.OpenSettings.Caps()
		}
		add(Target{KindSettings, s.id}, item)
	}
	plugins := locales.QuickSearchGroupsPlugins()
	for _, p := range in.Plugins {
		add(Target{KindPlugin, p.ID}, kit.PaletteItem{Label: cmp.Or(l10n.Localized(p.Name, in.Lang), p.ID),
			Description: l10n.Localized(p.Description, in.Lang), Group: plugins, Icon: "puzzle", Glyph: icons.Plugin(model.PluginIcon(p, in.Catalog)),
			Keywords: []string{p.ID}})
	}
	return out
}

func itemID(t Target) string {
	return strconv.Itoa(int(t.Kind)) + ":" + t.Value
}

type Actions struct {
	OpenConversation func(cid string)
	OpenPanel        func(tab string)
	NewWorker        func()
	NewConversation  func()
	ShowShortcuts    func()
	OpenSettings     func(section, plugin string)
}

type Palette struct {
	s     *store.Store
	act   Actions
	open  bool
	state kit.PaletteState
}

func New(s *store.Store, act Actions) *Palette {
	return &Palette{s: s, act: act}
}

func (p *Palette) Open() bool {
	return p.open
}

func (p *Palette) Toggle() {
	if p.open {
		p.open = false
		return
	}
	p.state.Reset()
	p.open = true
}

func (p *Palette) Close() {
	p.open = false
}

func (p *Palette) View(c *ui.Context, tk theme.Tokens, current string, canStartOver bool) {
	if !p.open {
		return
	}
	p.s.Conversations.Ensure()
	p.s.Workers.Ensure()
	list := p.s.Plugins().List
	list.Ensure()
	byID := p.s.WorkerMap()
	in := Input{Workers: byID, Current: current, Lang: i18n.Locale(), CanStartOver: canStartOver}
	if p.s.Conversations.Loaded {
		in.Conversations = model.SortConversations(model.VisibleConversations(p.s.Conversations.Data, byID), byID)
	}
	if list.Loaded {
		in.Plugins = list.Data
	}
	if catalog := p.s.Plugins().Catalog; catalog.Loaded {
		in.Catalog = catalog.Data.Plugins
	}
	entries := Entries(in)
	items := make([]kit.PaletteItem, len(entries))
	targets := make(map[string]Target, len(entries))
	for i, e := range entries {
		items[i] = e.Item
		targets[e.Item.ID] = e.Target
	}
	opts := kit.PaletteOpts{Label: locales.QuickSearchTitle(), Placeholder: locales.QuickSearchPlaceholder(), Keys: shortcuts.QuickSearch.Caps()}
	panel := kit.CommandPalette(c, tk, &p.open, &p.state, items, opts, func(it kit.PaletteItem) { p.run(targets[it.ID]) })
	shortcuts.QuickSearch.On(panel, p.Close)
	shortcuts.ShowShortcuts.On(panel, func() {
		p.Close()
		p.act.ShowShortcuts()
	})
}

func (p *Palette) run(t Target) {
	switch t.Kind {
	case KindConversation:
		p.act.OpenConversation(t.Value)
	case KindPanel:
		p.act.OpenPanel(t.Value)
	case KindNewWorker:
		p.act.NewWorker()
	case KindNewConversation:
		p.act.NewConversation()
	case KindShortcuts:
		p.act.ShowShortcuts()
	case KindSettings:
		p.act.OpenSettings(t.Value, "")
	case KindPlugin:
		p.act.OpenSettings(settings.SectionPlugins, t.Value)
	}
}
