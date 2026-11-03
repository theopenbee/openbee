package settings

import (
	"image"
	"image/color"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
	"rsc.io/qr"
)

const (
	setupQRSize        = 240
	setupQuietZone     = 2
	setupVisibleLines  = 6
	setupGap           = 16
	setupCodePadX      = 16
	setupCodePadY      = 20
	setupCodeGap       = 12
	setupStateGap      = 8
	setupStoppedGlyph  = 24
	setupWaitGlyph     = 16
	setupOutputPadX    = 16
	setupOutputPadY    = 12
	setupOutputGap     = 4
	setupReplyGap      = 8
	setupReconnectIcon = 14
)

var qrPalette = color.Palette{color.White, color.Black}

type setupState struct {
	open    bool
	plugin  string
	session *store.SetupSession
	reply   string
	sending bool
	qrURL   string
	qr      *ui.Bitmap
}

func (st *setupState) code(url string) *ui.Bitmap {
	if st.qrURL != url {
		st.qrURL, st.qr = url, qrBitmap(url)
	}
	return st.qr
}

func qrBitmap(url string) *ui.Bitmap {
	img, err := qrImage(url)
	if err != nil {
		return nil
	}
	return ui.NewBitmap(img).Pixelated()
}

func qrImage(url string) (*image.Paletted, error) {
	code, err := qr.Encode(url, qr.L)
	if err != nil {
		return nil, err
	}
	side := code.Size + 2*setupQuietZone
	img := image.NewPaletted(image.Rect(0, 0, side, side), qrPalette)
	for y := range code.Size {
		for x := range code.Size {
			if code.Black(x, y) {
				img.SetColorIndex(x+setupQuietZone, y+setupQuietZone, 1)
			}
		}
	}
	return img, nil
}

func (d *Dialog) startSetup(id string) {
	d.setup = setupState{open: true, plugin: id}
	d.restartSetup()
}

func (d *Dialog) restartSetup() {
	st := &d.setup
	st.sending = false
	st.session = d.s.Plugins().StartSetup(st.plugin, func(s *store.SetupSession) {
		if s != st.session || !s.Done || !s.OK {
			return
		}
		d.s.Toast(kit.Toast{Tone: kit.ToneSuccess, Title: locales.PluginsSetupSuccess()})
		st.open = false
	})
}

func (d *Dialog) syncSetup() {
	st := &d.setup
	if st.session == nil {
		return
	}
	if st.open && d.open && d.section == SectionPlugins && d.pluginID == st.plugin {
		return
	}
	d.s.Plugins().CancelSetup(st.session)
	d.setup = setupState{}
}

func (d *Dialog) setupDialog(c *ui.Context, tk theme.Tokens, title string) {
	st := &d.setup
	s := st.session
	if s == nil {
		s = &store.SetupSession{}
	}
	link := model.LatestLink(s.Lines)
	output := model.LastOutputLines(s.Lines, setupVisibleLines)
	failed := s.Done && !s.OK
	startFailed := s.ID == "" && !s.Starting && s.Err != ""
	stopped := startFailed || failed || s.Lost
	send := func() {
		text := strings.TrimSpace(st.reply)
		session := st.session
		if text == "" || session == nil || !session.Active || st.sending {
			return
		}
		st.sending = true
		d.s.Plugins().SendSetupInput(session, text, func(err error) {
			if st.session != session {
				return
			}
			st.sending = false
			if err != nil {
				d.toastError(err)
				return
			}
			st.reply = ""
		})
	}
	kit.Dialog(c, tk, &st.open, kit.DialogOpts{
		Title:       title,
		Description: locales.PluginsSetupScanHint(),
		Footer: func() {
			kit.Button(c, tk, locales.CommonCancel(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeMD}).OnClick(func() { st.open = false })
			kit.Button(c, tk, locales.PluginsSetupAgain(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Icon: "refresh-cw", Loading: s.Starting}).
				OnClick(d.restartSetup)
		},
	}, func() {
		ui.Column(c).MinWidth(0).Gap(setupGap).Children(func() {
			setupCode(c, tk, st, link, stopped)
			if len(output) > 0 {
				setupOutput(c, tk, output)
			}
			if s.Reconnecting {
				ui.Row(c).AlignItems(ui.Center).Gap(setupStateGap).Role(ui.RoleStatus).Children(func() {
					kit.Spinner(c, tk, setupReconnectIcon).TextColor(tk.TextSecondary)
					kit.Shimmer(c, tk, locales.PluginsSetupReconnecting())
				})
			}
			if stopped {
				body := locales.PluginsSetupLost()
				switch {
				case startFailed:
					body = s.Err
				case failed:
					body = locales.PluginsSetupFailed(s.Err)
				}
				kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.PluginsSetupStoppedTitle(), Body: body})
			}
			ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(setupReplyGap).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					kit.TextInput(c, tk, &st.reply, locales.PluginsSetupInputLabel(), locales.PluginsSetupInputPlaceholder()).
						Disabled(!s.Active).OnSubmit(send)
				})
				blank := strings.TrimSpace(st.reply) == ""
				kit.Button(c, tk, locales.PluginsSetupSend(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD,
					Loading: st.sending, Disabled: !s.Active || blank}).OnClick(send)
			})
		})
	})
}

func setupCode(c *ui.Context, tk theme.Tokens, st *setupState, link string, stopped bool) {
	ui.Column(c).MinWidth(0).AlignItems(ui.Center).Gap(setupCodeGap).Padding(setupCodePadY, setupCodePadX).
		Radius(theme.RadiusInset).Border(cardBorder, tk.BorderSubtle).Background(tk.SurfaceMuted).Children(func() {
		ui.Box(c).Size(setupQRSize, setupQRSize).Center().Children(func() {
			switch {
			case stopped:
				ui.Column(c).AlignItems(ui.Center).Gap(setupStateGap).TextColor(tk.TextSecondary).Children(func() {
					ui.Icon(c, icons.Get("circle-slash")).Size(setupStoppedGlyph, setupStoppedGlyph)
					ui.Text(c, locales.PluginsSetupStopped())
				})
			case link != "":
				if bmp := st.code(link); bmp != nil {
					kit.Tooltip(c, tk, ui.Image(c, bmp).Size(setupQRSize, setupQRSize).Label(link), link)
				} else {
					ui.Text(c, link).Font(theme.FontMono).FontSize(metaSize).TextAlign(ui.Center).Selectable()
				}
			default:
				ui.Row(c).AlignItems(ui.Center).Gap(setupStateGap).Role(ui.RoleStatus).Children(func() {
					kit.Spinner(c, tk, setupWaitGlyph).TextColor(tk.TextSecondary)
					kit.Shimmer(c, tk, locales.PluginsSetupWaiting())
				})
			}
		})
		ui.Text(c, locales.PluginsSetupRefreshHint()).FontSize(metaSize).TextColor(tk.TextSecondary).TextAlign(ui.Center)
	})
}

func setupOutput(c *ui.Context, tk theme.Tokens, lines []string) {
	ui.Column(c).MinWidth(0).Gap(setupOutputGap).Padding(setupOutputPadY, setupOutputPadX).Radius(theme.RadiusInset).
		Border(cardBorder, tk.BorderSubtle).Background(tk.SurfaceMuted).Role(ui.RoleStatus).Label(locales.PluginsSetupOutput()).
		Font(theme.FontMono).FontSize(metaSize).TextColor(tk.TextSecondary).Children(func() {
		for i, line := range lines {
			text := ui.Text(c, line).MinWidth(0)
			if i == len(lines)-1 {
				text.TextColor(tk.Foreground)
			}
		}
	})
}
