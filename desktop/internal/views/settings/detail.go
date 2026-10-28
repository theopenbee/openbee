package settings

import (
	"net/http"
	"slices"
	"strings"
	"time"

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
	backGap          = 6
	backGlyph        = 16
	backBottom       = 16
	detailNameSize   = 22
	detailSectionGap = 24
	detailHeadGapX   = 24
	detailHeadGapY   = 16
	detailMetaTop    = 8
	detailMetaGap    = 8
	detailActionsGap = 8
	detailInfoWidth  = 320
	detailSkeletons  = 2
	setupCardPadX    = 24
	setupCardPadY    = 20
	setupCardGap     = 16
	setupHintGap     = 4
	setupTitleSize   = 16
	infoFieldGap     = 20
	infoLabelGap     = 4
	infoNoteGap      = 8
	infoUpdateGap    = 12
	linkGlyph        = 14
	linkGap          = 4
	stateFailed      = "failed"
	sourceNpm        = "npm"
	sourcePath       = "path"
	metaJoin         = " · "
)

var linkSchemes = []string{"https://", "http://"}

type detailState struct {
	remove  uninstallState
	reset   int
	saving  bool
	saved   bool
	savedAt time.Time
}

type detailKey struct{}

func (d *Dialog) pluginDetail(c *ui.Context, tk theme.Tokens, id string) {
	p := d.s.Plugins()
	p.Watch(id)
	p.List.Ensure()
	p.Ops.Ensure()
	p.Catalog.Ensure()
	r := p.Detail(id)
	root := ui.Column(c).Key(id).MinWidth(0)
	st := ui.Local(root, detailKey{}, func() detailState {
		if r.Err != nil {
			r.Invalidate()
		}
		return detailState{}
	})
	r.Ensure()
	root.Children(func() {
		switch {
		case r.Err != nil:
			d.backLink(c, tk)
			d.detailError(c, tk, r)
		case !r.Loaded:
			detailSkeleton(c, tk)
		default:
			d.detailPage(c, tk, st, r.Data)
		}
	})
}

func (d *Dialog) backLink(c *ui.Context, tk theme.Tokens) {
	back := ui.ButtonBase(c).Label(locales.PluginsDetailBack()).Role(ui.RoleLink).AlignSelf(ui.Start).
		MinHeight(theme.ControlSM).Gap(backGap).AlignItems(ui.Center).FontSize(textSize).Margin(0, 0, backBottom, 0)
	fg := tk.TextSecondary
	if back.Hovered() || back.Pressed() {
		fg = tk.Foreground
	}
	back.TextColor(back.AnimateColor("fg", fg, navFade)).OnClick(func() { d.showPlugins(tabInstalled) }).Children(func() {
		ui.Icon(c, icons.Get("arrow-left")).Size(backGlyph, backGlyph)
		ui.Text(c, locales.PluginsDetailBack()).SingleLine()
	})
}

func (d *Dialog) detailError(c *ui.Context, tk theme.Tokens, r *store.Resource[api.PluginDetail]) {
	if api.IsStatus(r.Err, http.StatusNotFound) {
		insetCard(c, tk).Children(func() {
			kit.EmptyState(c, tk, "package-x", locales.PluginsDetailNotFound(), locales.PluginsDetailNotFoundHint(), func() {
				kit.Button(c, tk, locales.PluginsEmptyInstalledAction(), kit.ButtonOpts{Variant: kit.Primary, Size: kit.SizeMD}).
					OnClick(func() { d.showPlugins(tabMarket) })
			})
		})
		return
	}
	kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.PluginsDetailLoadError(), Body: l10n.ErrorText(r.Err),
		ActionLabel: locales.CommonRetry(), ActionLoading: r.Loading, OnAction: r.Invalidate})
}

func detailSkeleton(c *ui.Context, tk theme.Tokens) {
	ui.Column(c).MinWidth(0).Gap(detailSectionGap).Children(func() {
		for i := range detailSkeletons {
			insetCard(c, tk).Key(i).Padding(cardPad+skeletonLineTop, cardPad, cardPad, cardPad).Gap(skeletonLineGap).Children(func() {
				kit.SkeletonBar(c, tk, 0, skeletonFirstH).WidthPercent(skeletonFirstW)
				kit.SkeletonBar(c, tk, 0, skeletonRestH)
				kit.SkeletonBar(c, tk, 0, skeletonRestH).WidthPercent(skeletonLastW)
			})
		}
	})
}

func (d *Dialog) detailPage(c *ui.Context, tk theme.Tokens, st *detailState, detail api.PluginDetail) {
	p := d.s.Plugins()
	name := displayName(detail.Name, detail.ID)
	entry, listed := catalogEntry(p.Catalog.Data, detail.ID)
	removing := p.Uninstalling(detail.ID)
	setupLabel := l10n.Localized(detail.SetupLabel, i18n.Locale())
	if setupLabel == "" {
		setupLabel = locales.PluginsSetupDefaultLabel()
	}
	d.backLink(c, tk)
	d.detailHeader(c, tk, st, detail, name, entry.Official, removing)
	if showsError(detail.PluginStatus) {
		d.detailBanner(c, tk, detail)
	}
	wide := d.contentWidth() >= container3XL
	body := ui.Column(c)
	if wide {
		body.Row().AlignItems(ui.Start)
	}
	body.MinWidth(0).Gap(detailSectionGap).Children(func() {
		main := ui.Column(c).MinWidth(0).Gap(detailSectionGap)
		if wide {
			main.Grow(1).Basis(0)
		}
		main.Children(func() {
			if detail.HasSetup {
				d.setupCard(c, tk, detail.ID, setupLabel)
			}
			kit.Panel(c, tk, locales.PluginsDetailSettings(), locales.PluginsDetailSettingsHint(name), nil, func() {
				d.settingsForm(c, tk, st, detail, name)
			}).Radius(theme.RadiusInset)
		})
		info := d.infoPanel(c, tk, detail, entry, listed)
		if wide {
			info.Width(detailInfoWidth).Shrink(0)
		}
	})
	d.uninstallDialog(c, tk, &st.remove)
	if detail.HasSetup {
		d.setupDialog(c, tk, setupLabel)
	}
}

func catalogEntry(catalog api.PluginCatalog, id string) (api.CatalogPlugin, bool) {
	if i := slices.IndexFunc(catalog.Plugins, func(p api.CatalogPlugin) bool { return p.ID == id }); i >= 0 {
		return catalog.Plugins[i], true
	}
	return api.CatalogPlugin{}, false
}

func (d *Dialog) detailHeader(c *ui.Context, tk theme.Tokens, st *detailState, detail api.PluginDetail, name string, official, removing bool) {
	meta := detail.ID
	if detail.Package != "" {
		meta = detail.Package + metaJoin + detail.ID
	}
	ui.Row(c).MinWidth(0).Wrap().AlignItems(ui.Start).Justify(ui.SpaceBetween).GapX(detailHeadGapX).GapY(detailHeadGapY).
		Margin(0, 0, detailSectionGap, 0).Children(func() {
		ui.Column(c).MinWidth(0).Shrink(1).Children(func() {
			ui.Text(c, name).FontSize(detailNameSize).FontWeight(mediumWeight).Role(ui.RoleHeading).Level(2)
			ui.Row(c).MinWidth(0).Wrap().AlignItems(ui.Center).Gap(detailMetaGap).Margin(detailMetaTop, 0, 0, 0).Role(ui.RoleStatus).Children(func() {
				stateBadge(c, tk, detail.State, removing)
				if official {
					kit.SmallBadge(c, tk, locales.PluginsOfficial(), kit.ToneNeutral)
				}
				ui.Text(c, meta).Font(theme.FontMono).FontSize(metaSize).TextColor(tk.TextSecondary).MinWidth(0)
			})
		})
		d.detailActions(c, tk, st, detail, name, removing)
	})
}

func (d *Dialog) detailActions(c *ui.Context, tk theme.Tokens, st *detailState, detail api.PluginDetail, name string, removing bool) {
	p := d.s.Plugins()
	id := detail.ID
	busy := p.Acting() || p.Pending[id]
	kind := p.Action(id)
	enabled := p.Enabled(detail.PluginStatus)
	toggling := kind == store.ActionEnable || kind == store.ActionDisable
	opts := func(loading, blocked bool) kit.ButtonOpts {
		return kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Loading: loading, Disabled: removing || blocked || (busy && !loading)}
	}
	ui.Row(c).Shrink(0).Wrap().AlignItems(ui.Center).Gap(detailActionsGap).Children(func() {
		if enabled {
			kit.Button(c, tk, locales.PluginsActionDisable(), opts(toggling, false)).OnClick(func() { p.Toggle(id, false, nil) })
		} else {
			kit.Button(c, tk, locales.PluginsActionEnable(), opts(toggling, detail.State == stateNoConfig)).OnClick(func() { p.Toggle(id, true, nil) })
		}
		restart := opts(kind == store.ActionRestart, !enabled)
		restart.Icon = "rotate-cw"
		kit.Button(c, tk, locales.PluginsActionRestart(), restart).OnClick(func() { p.Restart(id, nil) })
		kit.Button(c, tk, locales.PluginsActionUninstall(), kit.ButtonOpts{Variant: kit.Danger, Size: kit.SizeSM, Icon: "trash-2", Disabled: removing}).
			OnClick(func() { st.remove.showLeaving(id, name) })
	})
}

func (d *Dialog) detailBanner(c *ui.Context, tk theme.Tokens, detail api.PluginDetail) {
	p := d.s.Plugins()
	id := detail.ID
	o := kit.AlertOpts{Tone: kit.ToneWarning, Title: locales.PluginsDetailRetryingTitle(), Body: detail.Error, Mono: true}
	if detail.State == stateFailed {
		o.Tone, o.Title = kit.ToneDanger, locales.PluginsDetailFailedTitle()
		if p.Enabled(detail.PluginStatus) {
			restarting := p.Action(id) == store.ActionRestart
			o.ActionLabel, o.ActionLoading = locales.PluginsDetailRestartNow(), restarting
			o.ActionDisabled = !restarting && (p.Acting() || p.Pending[id])
			o.OnAction = func() { p.Restart(id, nil) }
		}
	}
	kit.Alert(c, tk, o).Margin(0, 0, detailSectionGap, 0)
}

func (d *Dialog) setupCard(c *ui.Context, tk theme.Tokens, id, label string) {
	insetCard(c, tk).Padding(setupCardPadY, setupCardPadX).Children(func() {
		ui.Row(c).MinWidth(0).Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(setupCardGap).Children(func() {
			ui.Column(c).MinWidth(0).Shrink(1).Children(func() {
				ui.Text(c, label).FontSize(setupTitleSize).FontWeight(mediumWeight).Role(ui.RoleHeading).Level(3)
				ui.Text(c, locales.PluginsSetupCardHint()).TextColor(tk.TextSecondary).Margin(setupHintGap, 0, 0, 0)
			})
			kit.Button(c, tk, locales.PluginsSetupStart(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Icon: "qr-code"}).
				OnClick(func() { d.startSetup(id) })
		})
	})
}

func (d *Dialog) infoPanel(c *ui.Context, tk theme.Tokens, detail api.PluginDetail, entry api.CatalogPlugin, listed bool) ui.Element {
	return kit.Panel(c, tk, locales.PluginsDetailInfo(), "", nil, func() {
		ui.Column(c).MinWidth(0).Gap(infoFieldGap).Children(func() {
			if detail.Package != "" {
				infoField(c, tk, locales.PluginsDetailPackage(), func() { infoValue(c, tk, detail.Package, true) })
			}
			infoField(c, tk, locales.PluginsDetailVersion(), func() { d.versionValue(c, tk, detail, entry, listed) })
			infoField(c, tk, locales.PluginsDetailSource(), func() { infoValue(c, tk, sourceLabel(detail.Source), false) })
			if detail.Source == sourcePath {
				infoField(c, tk, locales.PluginsDetailPath(), func() { infoValue(c, tk, detail.Path, true) })
			}
			if entry.Repository != "" {
				infoField(c, tk, locales.PluginsDetailRepository(), func() { repositoryLink(c, tk, entry.Repository) })
			}
			if entry.License != "" {
				infoField(c, tk, locales.PluginsDetailLicense(), func() { infoValue(c, tk, entry.License, false) })
			}
		})
	}).Radius(theme.RadiusInset)
}

func infoField(c *ui.Context, tk theme.Tokens, label string, value func()) {
	ui.Column(c).MinWidth(0).Gap(infoLabelGap).Children(func() {
		ui.Text(c, label).FontSize(metaSize).TextColor(tk.TextSecondary)
		value()
	})
}

func infoValue(c *ui.Context, tk theme.Tokens, text string, mono bool) ui.Element {
	t := ui.Text(c, text).FontSize(textSize).TextColor(tk.Foreground).MinWidth(0).Selectable()
	if mono {
		t.Font(theme.FontMono)
	}
	return t
}

func sourceLabel(source string) string {
	switch source {
	case sourceNpm:
		return locales.PluginsDetailSourceNpm()
	case sourcePath:
		return locales.PluginsDetailSourcePath()
	}
	return source
}

func (d *Dialog) versionValue(c *ui.Context, tk theme.Tokens, detail api.PluginDetail, entry api.CatalogPlugin, listed bool) {
	p := d.s.Plugins()
	version := detail.Version
	if version == "" {
		version = noValue
	}
	ui.Column(c).MinWidth(0).AlignItems(ui.Start).Gap(infoUpdateGap).Children(func() {
		ui.Row(c).MinWidth(0).Wrap().AlignItems(ui.Center).Gap(infoNoteGap).Children(func() {
			infoValue(c, tk, version, true).FontFeatures("tnum")
			if listed && detail.Source == sourceNpm {
				note := locales.PluginsDetailLatest()
				if entry.UpdateAvailable {
					note = locales.PluginsDetailNewer(entry.Version)
				}
				ui.Text(c, note).FontSize(metaSize).TextColor(tk.TextSecondary)
			}
		})
		if !listed || !entry.UpdateAvailable || !entry.Installable {
			return
		}
		_, pending := model.PendingInstalls(p.Ops.Data)[entry.Package]
		spec := entry.Package + specJoin + entry.Version
		kit.Button(c, tk, locales.PluginsActionUpdate(entry.Version), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM,
			Loading: pending || p.Requested(entry.Package)}).
			OnClick(func() {
				p.Install(spec, func(_ api.PluginOperation, err error) { d.toastError(err) })
			})
	})
}

func repositoryLink(c *ui.Context, tk theme.Tokens, repo string) {
	shown := repo
	for _, scheme := range linkSchemes {
		if rest, ok := strings.CutPrefix(repo, scheme); ok {
			shown = rest
			break
		}
	}
	if !ui.SafeToOpen(repo) {
		infoValue(c, tk, shown, false)
		return
	}
	link := ui.ButtonBase(c).Label(shown).Role(ui.RoleLink).AlignSelf(ui.Start).MinWidth(0).AlignItems(ui.Center).Gap(linkGap).
		FontSize(textSize).TextColor(tk.AccentStrong)
	hovered := link.Hovered()
	if link.Clicked() {
		c.OpenURL(repo)
	}
	link.Children(func() {
		text := ui.Text(c, shown).MinWidth(0)
		if hovered {
			text.Underline()
		}
		ui.Icon(c, icons.Get("external-link")).Size(linkGlyph, linkGlyph).Shrink(0)
	})
}
