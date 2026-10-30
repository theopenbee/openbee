package settings

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	SectionEnv     = "env"
	SectionSystem  = "system"
	SectionPlugins = "plugins"
)

const (
	windowInset  = 16
	dialogBorder = 1
	contentPad   = 24
	contentFade  = 240 * time.Millisecond
)

var easeEnter = ui.CubicBezier(0.16, 1, 0.3, 1)

type section struct {
	id          string
	icon        string
	title       func() string
	nav         func() string
	description func() string
}

var sections = []section{
	{SectionEnv, "key-round", locales.EnvConfigTitle, locales.EnvConfigNavTitle, locales.EnvConfigPageDescription},
	{SectionSystem, "sliders-horizontal", locales.SystemSettingsTitle, locales.SystemSettingsTitle, locales.SystemSettingsSubtitle},
	{SectionPlugins, "puzzle", locales.PluginsTitle, locales.PluginsTitle, locales.PluginsSubtitle},
}

type Dialog struct {
	s          *store.Store
	open       bool
	section    string
	pluginID   string
	pluginsTab int
	setup      setupState
	revealed   string
	winW       float32
}

func New(s *store.Store) *Dialog {
	return &Dialog{s: s, section: SectionEnv}
}

func (d *Dialog) Open(id, pluginID string) {
	d.open = true
	d.section = current(id).id
	d.pluginID = pluginID
	d.pluginsTab = tabMarket
	d.revealed = ""
	d.syncPolling()
}

func (d *Dialog) Close() {
	d.open = false
	d.syncPolling()
}

func (d *Dialog) syncPolling() {
	p := d.s.Plugins()
	shown := d.open && d.section == SectionPlugins
	if !shown || d.pluginID == "" {
		p.Watch("")
	}
	p.SetActive(shown)
	d.syncSetup()
}

func current(id string) section {
	for _, s := range sections {
		if s.id == id {
			return s
		}
	}
	return sections[0]
}

func (d *Dialog) choose(id string) {
	d.section = id
	d.pluginID = ""
	d.pluginsTab = tabMarket
	d.syncPolling()
}

func (d *Dialog) wide() bool {
	return d.winW >= theme.BreakTwoPane
}

func (d *Dialog) contentWidth() float32 {
	w := min(d.winW-2*windowInset, theme.SettingsMaxW) - 2*dialogBorder
	if d.wide() {
		w -= navWidth
	}
	return w - 2*contentPad
}

func (d *Dialog) View(c *ui.Context) {
	tk := theme.Apply(c)
	d.winW, _ = c.Size()
	cur := current(d.section)
	kit.Dialog(c, tk, &d.open, kit.DialogOpts{
		Title:       cur.title(),
		Description: cur.description(),
		Width:       theme.SettingsMaxW,
		Height:      theme.SettingsMaxH,
		Flush:       true,
	}, func() { d.frame(c, tk, cur) })
	d.syncPolling()
}

func (d *Dialog) frame(c *ui.Context, tk theme.Tokens, cur section) {
	box := ui.Column(c)
	if d.wide() {
		box.Row().AlignItems(ui.Stretch)
	}
	box.Grow(1).Shrink(1).MinWidth(0).MinHeight(0).Children(func() {
		d.nav(c, tk, cur)
		d.content(c, tk, cur)
	})
}

func (d *Dialog) content(c *ui.Context, tk theme.Tokens, cur section) {
	pane := ui.Scroll(c).Key(cur.id + ":" + d.pluginID).Grow(1).Shrink(1).MinWidth(0).MinHeight(0)
	shown := pane.Keyframes("fade", []ui.Keyframe{{At: 0, Value: 0}, {At: contentFade, Value: 1, Ease: easeEnter}})
	pane.Opacity(shown).Children(func() {
		ui.Column(c).Padding(contentPad).MinWidth(0).Children(func() {
			switch cur.id {
			case SectionSystem:
				d.system(c, tk)
			case SectionPlugins:
				d.plugins(c, tk)
			default:
				d.env(c)
			}
		})
	})
}
