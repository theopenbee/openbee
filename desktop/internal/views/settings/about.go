package settings

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	ReleasesURL       = "https://github.com/theopenbee/openbee/releases"
	LatestReleaseURL  = ReleasesURL + "/latest"
	UpgradeCommand    = "openbee upgrade"
	commandPadStart   = 16
	commandPadEnd     = 4
	commandPadY       = 4
	commandGap        = 8
	versionSkeletonW  = 64
	versionSkeletonH  = 20
	relativeMinutes   = "minutes"
	relativeHours     = "hours"
	relativeDays      = "days"
	switchRowMinH     = 64
	aboutSwitchBorder = 1
)

func (d *Page) about(c *ui.Context, tk theme.Tokens) {
	r := d.s.ServerVersion
	r.Ensure()
	ui.Column(c).Key(SectionAbout).MinWidth(0).Gap(formGap).Children(func() {
		d.desktopGroup(c, tk)
		d.serverGroup(c, tk, r)
	})
}

func (d *Page) desktopGroup(c *ui.Context, tk theme.Tokens) {
	ui.Column(c).MinWidth(0).Gap(groupGap).Children(func() {
		heading(c, locales.AboutDesktopTitle())
		card(c, tk, func() {
			d.row(c, tk, true, locales.AboutVersion(), locales.AboutDesktopDescription(), func() {
				infoValue(c, tk, d.s.AppVersion(), true)
			})
			if u := d.s.Updater(); u != nil {
				d.updaterRows(c, tk, u)
			} else {
				d.row(c, tk, false, locales.AboutUpdates(), locales.AboutManualUpdates(), func() {
					kit.Button(c, tk, locales.AboutDownloadLatest(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Icon: "download"}).
						AlignSelf(ui.Start).OnClick(func() { c.OpenURL(LatestReleaseURL) })
				})
			}
			d.row(c, tk, false, locales.AboutReleasesTitle(), locales.AboutReleasesDescription(), func() {
				externalLink(c, tk, ReleasesURL, locales.AboutReleasesLink())
			})
		})
	})
}

func (d *Page) updaterRows(c *ui.Context, tk theme.Tokens, u store.Updater) {
	d.row(c, tk, false, locales.AboutUpdates(), lastChecked(u.LastCheck(), c.Now()), func() {
		kit.Button(c, tk, locales.AboutCheckUpdates(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Icon: "refresh-cw"}).
			AlignSelf(ui.Start).OnClick(u.Check)
	})
	auto := u.AutomaticChecks()
	d.switchRow(c, tk, &auto, locales.AboutAutoCheck(), locales.AboutAutoCheckDescription(), false).
		OnChange(func() { u.SetAutomaticChecks(auto) })
	install := u.AutomaticDownloads()
	d.switchRow(c, tk, &install, locales.AboutAutoInstall(), locales.AboutAutoInstallDescription(), !auto).
		OnChange(func() { u.SetAutomaticDownloads(install) })
}

func (d *Page) switchRow(c *ui.Context, tk theme.Tokens, on *bool, title, description string, disabled bool) ui.Element {
	padX := float32(rowPadXNarrow)
	if d.contentWidth() >= containerLG {
		padX = rowPadXWide
	}
	return kit.SwitchRow(c, tk, on, title, description, disabled).MinHeight(switchRowMinH).Padding(rowPadY, padX).
		BorderWidth(aboutSwitchBorder, 0, 0, 0).BorderColor(tk.BorderSubtle)
}

func (d *Page) serverGroup(c *ui.Context, tk theme.Tokens, r *store.Resource[api.ServerVersion]) {
	ui.Column(c).MinWidth(0).Gap(groupGap).Children(func() {
		heading(c, locales.AboutServerTitle())
		switch {
		case r.Err != nil:
			kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.AboutLoadError(), Body: l10n.ErrorText(r.Err),
				ActionLabel: locales.CommonRetry(), ActionLoading: r.Loading, OnAction: r.Invalidate})
		default:
			v := r.Data
			card(c, tk, func() {
				description := locales.AboutServerDescription()
				if v.Bundled {
					description = locales.AboutServerBundledDescription()
				}
				d.row(c, tk, true, locales.AboutVersion(), description, func() {
					switch {
					case !r.Loaded:
						kit.SkeletonBar(c, tk, versionSkeletonW, versionSkeletonH)
					case v.Version == "":
						infoValue(c, tk, locales.AboutVersionUnknown(), false)
					default:
						infoValue(c, tk, v.Version, true)
					}
				})
				if r.Loaded && !v.Bundled {
					d.row(c, tk, false, locales.AboutServerUpgrade(), locales.AboutServerUpgradeDescription(), func() {
						upgradeCommand(c, tk)
					})
				}
			})
		}
	})
}

func upgradeCommand(c *ui.Context, tk theme.Tokens) {
	ui.Row(c).MinWidth(0).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(commandGap).
		Padding(commandPadY, commandPadEnd, commandPadY, commandPadStart).Radius(theme.RadiusControl).
		Border(cardBorder, tk.Border).Background(tk.SurfaceMuted).Children(func() {
		ui.Text(c, UpgradeCommand).Font(theme.FontMono).FontSize(textSize).TextColor(tk.Foreground).SingleLine().MinWidth(0).Selectable()
		kit.CopyIconButton(c, tk, UpgradeCommand, locales.AboutServerCopyCommand())
	})
}

func lastChecked(at, now time.Time) string {
	if at.IsZero() {
		return locales.AboutNeverChecked()
	}
	r := model.RelativeTime(at.UnixMilli(), now)
	switch r.Unit {
	case relativeMinutes:
		return locales.AboutLastChecked(locales.TimeMinutesAgo(r.N))
	case relativeHours:
		return locales.AboutLastChecked(locales.TimeHoursAgo(r.N))
	case relativeDays:
		return locales.AboutLastChecked(locales.TimeDaysAgo(r.N))
	}
	return locales.AboutLastChecked(locales.TimeJustNow())
}
