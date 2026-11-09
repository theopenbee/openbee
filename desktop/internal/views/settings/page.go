package settings

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	SectionEnv     = "env"
	SectionSystem  = "system"
	SectionPlugins = "plugins"
)

const (
	contentPadNarrow = 24
	contentPadWide   = 32
	descriptionGap   = 24
	contentFade      = 240 * time.Millisecond
)

var easeEnter = ui.CubicBezier(0.16, 1, 0.3, 1)

type mainKey struct{}

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

type Page struct {
	s          *store.Store
	router     route.Nav
	leave      func()
	shown      bool
	section    string
	pluginID   string
	pluginsTab int
	setup      setupState
	revealed   string
	winW       float32
}

func New(s *store.Store, nav route.Nav, leave func()) *Page {
	return &Page{s: s, router: nav, leave: leave, section: SectionEnv}
}

func Routable(path string) bool {
	rest, ok := strings.CutPrefix(path, route.SettingsPath+"/")
	if !ok {
		return false
	}
	id, plugin, nested := strings.Cut(rest, "/")
	if id == SectionPlugins && nested {
		return plugin != "" && !strings.Contains(plugin, "/")
	}
	return !nested && current(id).id == id
}

func (d *Page) Open(id, pluginID string) {
	d.show(current(id).id, pluginID, tabMarket)
}

func (d *Page) show(id, pluginID string, tab int) {
	d.pluginsTab = tab
	d.router.Push(route.Settings(id, pluginID))
}

func (d *Page) Hide() {
	if !d.shown {
		return
	}
	d.shown = false
	d.revealed = ""
	d.syncPolling()
}

func (d *Page) syncPolling() {
	p := d.s.Plugins()
	shown := d.shown && d.section == SectionPlugins
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

func (d *Page) choose(id string) {
	d.show(id, "", tabMarket)
}

func (d *Page) wide() bool {
	return d.winW >= theme.BreakTwoPane
}

func (d *Page) contentPad() float32 {
	if d.wide() {
		return contentPadWide
	}
	return contentPadNarrow
}

func (d *Page) contentWidth() float32 {
	w := d.winW
	if d.wide() {
		w -= theme.SidebarWidth + sidebarBorder
	}
	return min(w, theme.SettingsColumnMax) - 2*d.contentPad()
}

func (d *Page) View(c *ui.Context, id, pluginID string) {
	tk := theme.Apply(c)
	d.winW, _ = c.Size()
	d.shown = true
	d.section, d.pluginID = current(id).id, pluginID
	cur := current(d.section)
	if c.Shortcut(0, ui.KeyEscape) {
		d.leave()
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(tk.Background).Children(func() {
		if d.wide() {
			d.sidebar(c, tk, cur)
		}
		ui.Column(c).Key(mainKey{}).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			d.header(c, tk, cur)
			if !d.wide() {
				d.nav(c, tk, cur)
			}
			d.content(c, tk, cur)
		})
	})
	d.syncPolling()
}

func (d *Page) content(c *ui.Context, tk theme.Tokens, cur section) {
	pane := ui.Scroll(c).Key(cur.id + ":" + d.pluginID).Grow(1).Shrink(1).MinWidth(0).MinHeight(0)
	shown := pane.Keyframes("fade", []ui.Keyframe{{At: 0, Value: 0}, {At: contentFade, Value: 1, Ease: easeEnter}})
	pane.Opacity(shown).Children(func() {
		ui.Column(c).AlignItems(ui.Center).Children(func() {
			ui.Column(c).WidthPercent(100).MaxWidth(theme.SettingsColumnMax).Padding(d.contentPad()).MinWidth(0).FontSize(textSize).Children(func() {
				if d.pluginID == "" {
					ui.Text(c, cur.description()).TextColor(tk.TextSecondary).Margin(0, 0, descriptionGap, 0)
				}
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
	})
}
