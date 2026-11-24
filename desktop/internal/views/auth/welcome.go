package auth

import (
	"context"
	"errors"
	"slices"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	engineColumnWidth  = 560
	channelColumnWidth = 760
	channelGridMin     = 640
	choiceGap          = 12
	choicePadX         = 16
	choicePadY         = 12
	choiceTextGap      = 2
	choiceMetaSize     = 12
	choiceWeight       = 500
	choiceFace         = 36
	installedGlyph     = 16
	installedGap       = 6
	welcomeFooterTop   = 24
	welcomeFooterGap   = 12
	skeletonChoices    = 4
	skeletonLineGap    = 8
	skeletonNameW      = 40
	skeletonNameH      = 14
	skeletonDescW      = 72
	skeletonDescH      = 12
	specJoin           = "@"
)

type welcomeStep int

const (
	stepEngine welcomeStep = iota
	stepChannel
)

type welcomeState struct {
	step     welcomeStep
	skipping bool
}

type welcomeKey struct{}

func (a *Screens) Welcome(c *ui.Context, tk theme.Tokens, page ui.Element, configure func(pluginID string)) {
	st := ui.Local(page, welcomeKey{}, func() welcomeState { return welcomeState{} })
	p := a.s.Plugins()
	a.welcomeShown = true
	p.SetActive(true)
	p.Catalog.Ensure()
	p.Ops.Ensure()
	if st.step == stepChannel {
		a.channelStep(c, tk, configure)
		return
	}
	a.layout(c, tk, engineColumnWidth, locales.WelcomeEngineTitle(), locales.WelcomeEngineDescription(), func() {
		a.choices(c, tk, model.PluginEngine, nil)
		ui.Row(c).AlignItems(ui.Center).Gap(welcomeFooterGap).Margin(welcomeFooterTop, 0, 0, 0).Children(func() {
			kit.Button(c, tk, locales.WelcomeSkip(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeMD, Loading: st.skipping}).
				OnClick(func() { a.skipWelcome(st) })
			ui.Text(c, locales.WelcomeEngineSkipHint()).FontSize(choiceMetaSize).TextColor(tk.TextMuted).Grow(1).MinWidth(0)
			kit.Button(c, tk, locales.WelcomeNext(), kit.ButtonOpts{Size: kit.SizeMD, Disabled: !engineInstalled(p.Catalog.Data)}).
				OnClick(func() { st.step = stepChannel })
		})
	})
}

func (a *Screens) channelStep(c *ui.Context, tk theme.Tokens, configure func(pluginID string)) {
	a.layout(c, tk, channelColumnWidth, locales.WelcomeChannelTitle(), locales.WelcomeChannelDescription(), func() {
		a.choices(c, tk, model.PluginChannel, func(id string) {
			a.s.FinishOnboarding()
			configure(id)
		})
		ui.Row(c).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(welcomeFooterGap).Margin(welcomeFooterTop, 0, 0, 0).Children(func() {
			kit.Button(c, tk, locales.WelcomeSkip(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeMD}).OnClick(a.leaveWelcome)
			kit.Button(c, tk, locales.WelcomeDone(), kit.ButtonOpts{Size: kit.SizeMD}).OnClick(a.leaveWelcome)
		})
	})
}

func (a *Screens) HideWelcome() {
	if !a.welcomeShown {
		return
	}
	a.welcomeShown = false
	a.s.Plugins().SetActive(false)
}

func (a *Screens) skipWelcome(st *welcomeState) {
	if st.skipping {
		return
	}
	st.skipping = true
	a.s.SkipOnboarding(func(err error) {
		st.skipping = false
		if err != nil {
			a.toastError(err)
			return
		}
		a.nav.Replace(route.ChatPath)
	})
}

func (a *Screens) leaveWelcome() {
	a.s.FinishOnboarding()
	a.nav.Replace(route.ChatPath)
}

func (a *Screens) choices(c *ui.Context, tk theme.Tokens, category string, configure func(pluginID string)) {
	p := a.s.Plugins()
	r := p.Catalog
	if r.Err != nil {
		kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.PluginsCatalogError(), Body: l10n.ErrorText(r.Err),
			ActionLabel: locales.CommonRetry(), ActionLoading: r.Loading, OnAction: r.Invalidate})
		return
	}
	columns := 1
	if w, _ := c.Size(); category == model.PluginChannel && w >= channelGridMin {
		columns = 2
	}
	pending := model.PendingInstalls(p.Ops.Data)
	ui.Grid(c).Columns(columns).GapX(choiceGap).GapY(choiceGap).MinWidth(0).Children(func() {
		if !r.Loaded {
			for i := range skeletonChoices {
				choiceSkeleton(c, tk, i)
			}
			return
		}
		for _, plugin := range r.Data.Plugins {
			if plugin.Category != category {
				continue
			}
			op, installing := pending[plugin.Package]
			a.choice(c, tk, plugin, op, installing || p.Requested(plugin.Package), configure)
		}
	})
}

func choiceRow(c *ui.Context, tk theme.Tokens) ui.Element {
	return ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(choiceGap).Padding(choicePadY, choicePadX).
		Radius(theme.Concentric(theme.RadiusSurface, cardPadding)).Border(1, tk.Border).Background(tk.Surface)
}

func (a *Screens) choice(c *ui.Context, tk theme.Tokens, plugin api.CatalogPlugin, op api.PluginOperation, installing bool, configure func(pluginID string)) {
	lang := i18n.Locale()
	name := l10n.Localized(plugin.Name, lang)
	if name == "" {
		name = plugin.ID
	}
	choiceRow(c, tk).Key(plugin.ID).Children(func() {
		kit.InitialsAvatar(c, tk, name, kit.AvatarMD)
		ui.Column(c).Grow(1).MinWidth(0).Gap(choiceTextGap).Children(func() {
			ui.Text(c, name).FontSize(bodySize).FontWeight(choiceWeight).SingleLine().MinWidth(0).Role(ui.RoleHeading).Level(2)
			if description := l10n.Localized(plugin.Description, lang); description != "" {
				ui.Text(c, description).FontSize(choiceMetaSize).TextColor(tk.TextSecondary).SingleLine().MinWidth(0)
			}
		})
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(func() {
			a.choiceAction(c, tk, plugin, op, installing, configure)
		})
	})
}

func (a *Screens) choiceAction(c *ui.Context, tk theme.Tokens, plugin api.CatalogPlugin, op api.PluginOperation, installing bool, configure func(pluginID string)) {
	secondary := kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM}
	switch model.CatalogAction(plugin, installing) {
	case model.ActionInstalling:
		kit.OperationProgress(c, tk, locales.PluginsActionInstallingShort(), op.CreatedAt)
	case model.ActionIncompatible:
		ui.Text(c, locales.PluginsActionIncompatible()).FontSize(choiceMetaSize).TextColor(tk.TextSecondary)
	case model.ActionInstall:
		secondary.Icon = "download"
		kit.Button(c, tk, locales.PluginsActionInstall(), secondary).OnClick(func() { a.install(plugin) })
	default:
		if configure != nil {
			kit.Button(c, tk, locales.PluginsActionConfigure(), secondary).OnClick(func() { configure(plugin.ID) })
			return
		}
		ui.Row(c).MinHeight(theme.ControlSM).AlignItems(ui.Center).Gap(installedGap).Children(func() {
			ui.Icon(c, icons.Get("check")).Size(installedGlyph, installedGlyph).Shrink(0).TextColor(tk.Success)
			ui.Text(c, locales.WelcomeInstalled()).FontSize(bodySize).TextColor(tk.TextSecondary).SingleLine()
		})
	}
}

func (a *Screens) install(plugin api.CatalogPlugin) {
	a.s.Plugins().Install(plugin.Package+specJoin+plugin.Version, func(_ api.PluginOperation, err error) {
		if err != nil {
			a.toastError(err)
		}
	})
}

func (a *Screens) toastError(err error) {
	if !errors.Is(err, context.Canceled) {
		a.s.Toast(kit.Toast{Tone: kit.ToneDanger, Title: l10n.ErrorText(err)})
	}
}

func choiceSkeleton(c *ui.Context, tk theme.Tokens, key int) {
	choiceRow(c, tk).Key(key).Children(func() {
		kit.SkeletonCircle(c, tk, choiceFace)
		ui.Column(c).Grow(1).MinWidth(0).Gap(skeletonLineGap).Children(func() {
			kit.SkeletonBar(c, tk, 0, skeletonNameH).WidthPercent(skeletonNameW)
			kit.SkeletonBar(c, tk, 0, skeletonDescH).WidthPercent(skeletonDescW)
		})
	})
}

func engineInstalled(catalog api.PluginCatalog) bool {
	return slices.ContainsFunc(catalog.Plugins, func(p api.CatalogPlugin) bool {
		return p.Category == model.PluginEngine && p.Installed
	})
}
