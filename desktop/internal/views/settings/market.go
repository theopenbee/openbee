package settings

import (
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
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	marketGap       = 24
	toolbarGap      = 16
	searchWidth     = 288
	gridGap         = 16
	cardPad         = 24
	cardHeadGap     = 12
	cardTitleGap    = 8
	cardNameSize    = 16
	cardMetaGap     = 2
	cardDescGap     = 16
	cardDescLines   = 2
	cardDescMin     = 40
	cardTagsGap     = 12
	cardFooterGap   = 20
	cardFooterPad   = 16
	cardFooterMin   = 36
	cardFooterGapX  = 12
	cardFooterGapY  = 8
	installedGlyph  = 14
	installedGap    = 6
	skeletonCards   = 6
	skeletonLineGap = 12
	skeletonLineTop = 4
	skeletonFirstW  = 48
	skeletonFirstH  = 16
	skeletonRestH   = 12
	skeletonLastW   = 72
	tagJoin         = " · "
	versionJoin     = " · v"
	specJoin        = "@"
)

type marketState struct {
	query      string
	tag        string
	refreshing bool
}

type marketKey struct{}

func (d *Dialog) marketTab(c *ui.Context, tk theme.Tokens) {
	r := d.s.Plugins().Catalog
	r.Ensure()
	root := ui.Column(c).MinWidth(0).Gap(marketGap)
	st := ui.Local(root, marketKey{}, func() marketState { return marketState{} })
	root.Children(func() {
		d.marketToolbar(c, tk, st, r)
		switch {
		case r.Err != nil:
			kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.PluginsCatalogError(), Body: l10n.ErrorText(r.Err),
				ActionLabel: locales.CommonRetry(), ActionLoading: r.Loading, OnAction: r.Invalidate})
		case !r.Loaded:
			d.marketSkeleton(c, tk)
		default:
			d.marketGrid(c, tk, st, r.Data)
		}
	})
}

func (d *Dialog) marketToolbar(c *ui.Context, tk theme.Tokens, st *marketState, r *store.Resource[api.PluginCatalog]) {
	ui.Row(c).MinWidth(0).Wrap().AlignItems(ui.End).GapX(toolbarGap).GapY(toolbarGap).Children(func() {
		field := kit.Field(c, tk, locales.PluginsSearch(), "", "", func() {
			kit.SearchField(c, tk, &st.query, locales.PluginsSearch(), locales.PluginsSearchPlaceholder()).Disabled(!r.Loaded)
		}).MinWidth(0)
		if d.contentWidth() >= containerXL {
			field.Width(searchWidth)
		} else {
			field.WidthPercent(100)
		}
		if r.Loaded {
			d.tagFilter(c, tk, st, r.Data)
		}
		ui.Row(c).MarginStart(ui.Auto).Height(theme.ControlMD).AlignItems(ui.Center).Children(func() {
			kit.Button(c, tk, locales.PluginsRefreshCatalog(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "refresh-cw", Loading: st.refreshing}).
				OnClick(func() { d.refreshCatalog(st) })
		})
	})
}

func (d *Dialog) refreshCatalog(st *marketState) {
	if st.refreshing {
		return
	}
	st.refreshing = true
	d.s.Plugins().RefreshCatalog(func(err error) {
		st.refreshing = false
		d.toastError(err)
	})
}

func (d *Dialog) tagFilter(c *ui.Context, tk theme.Tokens, st *marketState, catalog api.PluginCatalog) {
	tags := model.UsedTags(catalog)
	items := []kit.SegmentItem{{Text: locales.PluginsAllTags()}}
	selected := 0
	for i, tag := range tags {
		items = append(items, kit.SegmentItem{Text: displayName(tag.Name, tag.ID)})
		if tag.ID == st.tag {
			selected = i + 1
		}
	}
	if selected == 0 {
		st.tag = ""
	}
	ui.Row(c).MinWidth(0).MaxWidthPercent(100).Children(func() {
		kit.Segmented(c, tk, &selected, locales.PluginsTagFilter(), items).MaxWidthPercent(100).OnChange(func() {
			st.tag = ""
			if selected > 0 {
				st.tag = tags[selected-1].ID
			}
		})
	})
}

func (d *Dialog) gridColumns() int {
	switch w := d.contentWidth(); {
	case w >= container4XL:
		return 3
	case w >= containerXL:
		return 2
	}
	return 1
}

func (d *Dialog) marketGrid(c *ui.Context, tk theme.Tokens, st *marketState, catalog api.PluginCatalog) {
	list := model.FilterCatalog(catalog, st.tag, st.query)
	if len(list) == 0 {
		insetCard(c, tk).Children(func() {
			kit.EmptyState(c, tk, "search-x", locales.PluginsNoResults(), locales.PluginsNoResultsHint(), func() {
				kit.Button(c, tk, locales.PluginsClearFilters(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD}).
					OnClick(func() { st.query, st.tag = "", "" })
			})
		})
		return
	}
	p := d.s.Plugins()
	pending := model.PendingInstalls(p.Ops.Data)
	tagNames := map[string]string{}
	for _, tag := range catalog.Tags {
		tagNames[tag.ID] = displayName(tag.Name, tag.ID)
	}
	ui.Grid(c).Columns(d.gridColumns()).GapX(gridGap).GapY(gridGap).MinWidth(0).Children(func() {
		for _, plugin := range list {
			op, installing := pending[plugin.Package]
			d.marketCard(c, tk, plugin, op, installing || p.Requested(plugin.Package), tagNames)
		}
	})
}

func (d *Dialog) marketCard(c *ui.Context, tk theme.Tokens, plugin api.CatalogPlugin, op api.PluginOperation, installing bool, tagNames map[string]string) {
	lang := i18n.Locale()
	name := displayName(plugin.Name, plugin.ID)
	insetCard(c, tk).Key(plugin.ID).Padding(cardPad).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c).MinWidth(0).AlignItems(ui.Start).Gap(cardHeadGap).Children(func() {
				kit.InitialsAvatar(c, tk, name, kit.AvatarMD)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Row(c).MinWidth(0).Wrap().AlignItems(ui.Center).Gap(cardTitleGap).Children(func() {
						ui.Text(c, name).FontSize(cardNameSize).FontWeight(mediumWeight).SingleLine().MinWidth(0).
							Role(ui.RoleHeading).Level(3)
						if plugin.Official {
							kit.SmallBadge(c, tk, locales.PluginsOfficial(), kit.ToneNeutral)
						}
						if notice := l10n.Localized(plugin.Deprecated, lang); notice != "" {
							kit.Tooltip(c, tk, kit.SmallBadge(c, tk, locales.PluginsDeprecated(), kit.ToneWarning), notice)
						}
					})
					ui.Text(c, plugin.ID+versionJoin+plugin.Version).Font(theme.FontMono).FontSize(metaSize).
						TextColor(tk.TextSecondary).SingleLine().MinWidth(0).Margin(cardMetaGap, 0, 0, 0)
				})
			})
			ui.Text(c, l10n.Localized(plugin.Description, lang)).TextColor(tk.TextSecondary).MaxLines(cardDescLines).
				MinHeight(cardDescMin).Margin(cardDescGap, 0, 0, 0)
			if len(plugin.Tags) > 0 {
				names := make([]string, len(plugin.Tags))
				for i, id := range plugin.Tags {
					names[i] = id
					if tagName, ok := tagNames[id]; ok {
						names[i] = tagName
					}
				}
				ui.Text(c, strings.Join(names, tagJoin)).FontSize(metaSize).TextColor(tk.TextMuted).SingleLine().
					Margin(cardTagsGap, 0, 0, 0)
			}
		})
		ui.Row(c).MinWidth(0).MinHeight(cardFooterMin).Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).
			GapX(cardFooterGapX).GapY(cardFooterGapY).Margin(cardFooterGap, 0, 0, 0).Padding(cardFooterPad, 0, 0, 0).
			BorderWidth(cardBorder, 0, 0, 0).BorderColor(tk.BorderSubtle).Children(func() {
			ui.Row(c).MinWidth(0).Children(func() {
				if plugin.Installed {
					installedNote(c, tk, plugin)
				}
			})
			d.marketAction(c, tk, plugin, op, installing)
		})
	})
}

func installedNote(c *ui.Context, tk theme.Tokens, plugin api.CatalogPlugin) {
	text := locales.PluginsInstalledVersion(plugin.InstalledVersion)
	if plugin.UpdateAvailable {
		text = locales.PluginsUpdateAvailable(plugin.InstalledVersion)
	}
	ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(installedGap).Children(func() {
		ui.Icon(c, icons.Get("check")).Size(installedGlyph, installedGlyph).Shrink(0).TextColor(tk.Success)
		ui.Text(c, text).FontSize(metaSize).TextColor(tk.TextSecondary).SingleLine().MinWidth(0)
	})
}

func (d *Dialog) marketAction(c *ui.Context, tk theme.Tokens, plugin api.CatalogPlugin, op api.PluginOperation, installing bool) {
	secondary := kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM}
	install := func() {
		d.s.Plugins().Install(plugin.Package+specJoin+plugin.Version, func(_ api.PluginOperation, err error) { d.toastError(err) })
	}
	switch model.CatalogAction(plugin, installing) {
	case model.ActionInstalling:
		operationProgress(c, tk, locales.PluginsActionInstallingShort(), op.CreatedAt)
	case model.ActionUpdate:
		kit.Button(c, tk, locales.PluginsActionUpdate(plugin.Version), secondary).OnClick(install)
	case model.ActionConfigure:
		kit.Button(c, tk, locales.PluginsActionConfigure(), secondary).OnClick(func() { d.Open(SectionPlugins, plugin.ID) })
	case model.ActionIncompatible:
		ui.Text(c, locales.PluginsActionIncompatible()).FontSize(metaSize).TextColor(tk.TextSecondary)
	default:
		secondary.Icon = "download"
		kit.Button(c, tk, locales.PluginsActionInstall(), secondary).OnClick(install)
	}
}

func (d *Dialog) marketSkeleton(c *ui.Context, tk theme.Tokens) {
	ui.Grid(c).Columns(d.gridColumns()).GapX(gridGap).GapY(gridGap).MinWidth(0).Children(func() {
		for i := range skeletonCards {
			insetCard(c, tk).Key(i).Padding(cardPad+skeletonLineTop, cardPad, cardPad, cardPad).Gap(skeletonLineGap).Children(func() {
				kit.SkeletonBar(c, tk, 0, skeletonFirstH).WidthPercent(skeletonFirstW)
				kit.SkeletonBar(c, tk, 0, skeletonRestH)
				kit.SkeletonBar(c, tk, 0, skeletonRestH).WidthPercent(skeletonLastW)
			})
		}
	})
}

func insetCard(c *ui.Context, tk theme.Tokens) ui.Element {
	return ui.Column(c).MinWidth(0).Radius(theme.RadiusInset).Border(cardBorder, tk.Border).Background(tk.Surface)
}
