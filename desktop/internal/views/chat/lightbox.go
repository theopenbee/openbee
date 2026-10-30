package chat

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	lightboxScrim     = 0.9
	lightboxFade      = 240 * time.Millisecond
	lightboxScale     = 0.98
	lightboxBarPad    = 16
	lightboxBarGap    = 12
	lightboxBottom    = 32
	lightboxPadNarrow = 16
	lightboxPadWide   = 48
	lightboxSpinner   = 24
)

type lightboxState struct {
	open bool
	name string
}

func (v *View) openLightbox(name string) {
	v.light = lightboxState{open: true, name: name}
}

func (v *View) lightbox(c *ui.Context, tk theme.Tokens) {
	l := &v.light
	if !l.open {
		return
	}
	w, h := c.Size()
	name := l.name
	ui.DialogBase(c, &l.open, func(backdrop, panel ui.Element) {
		fade := backdrop.Keyframes("fade", []ui.Keyframe{{At: 0, Value: 0}, {At: lightboxFade, Value: 1, Ease: easeEnter}})
		backdrop.Background(tk.Background.Alpha(lightboxScrim * fade))
		rise := panel.Keyframes("rise", []ui.Keyframe{{At: 0, Value: 0}, {At: lightboxFade, Value: 1, Ease: easeEnter}})
		scale := lightboxScale + (1-lightboxScale)*rise
		panel.Size(w, h).Label(locales.ChatViewImage(name)).TextColor(tk.Foreground).FontSize(bodySize).Opacity(rise).Scale(scale, scale)
		if panel.Clicked() {
			l.open = false
		}
		v.lightboxBar(c, tk, name, w)
		v.lightboxStage(c, tk, name, w, h)
	})
}

func (v *View) lightboxBar(c *ui.Context, tk theme.Tokens, name string, w float32) {
	inset := c.TitleBar()
	bar := ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(lightboxBarGap).PaddingY(lightboxBarPad).
		PaddingStart(max(lightboxBarPad, inset.Left)).PaddingEnd(max(lightboxBarPad, inset.Right))
	keepOpen(bar).Children(func() {
		ui.Text(c, name).FontWeight(mediumWeight).TextColor(tk.Foreground).SingleLine().Grow(1).MinWidth(0)
		label, text := locales.ChatOpenOriginal(), locales.ChatOpenOriginal()
		if w < theme.BreakComposer {
			text = ""
		}
		open := kit.Button(c, tk, text, kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Icon: "external-link", Label: label})
		if open.Clicked() {
			v.openMedia(name)
		}
		if kit.IconButton(c, tk, "x", locales.ChatCloseImage(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD}).Clicked() {
			v.light.open = false
		}
	})
}

func (v *View) lightboxStage(c *ui.Context, tk theme.Tokens, name string, w, h float32) {
	pad := float32(lightboxPadNarrow)
	if w >= theme.BreakComposer {
		pad = lightboxPadWide
	}
	ui.Column(c).Grow(1).MinHeight(0).Center().Padding(0, pad, lightboxBottom, pad).Children(func() {
		src, ok := v.photo(c, name, fullSize)
		if !ok {
			v.lightboxPending(c, tk, name)
			return
		}
		iw, ih := sourceSize(src)
		if iw <= 0 || ih <= 0 {
			return
		}
		barH := float32(theme.ControlMD + 2*lightboxBarPad)
		k := min(1, (w-2*pad)/iw, (h-barH-lightboxBottom)/ih)
		img := ui.Image(c, src).Label(name).Fit(ui.Contain).Size(iw*k, ih*k).Radius(theme.RadiusPanel).Border(1, tk.Border).
			Background(tk.Surface).Clip()
		theme.WithShadow(keepOpen(img), tk.ShadowFloating)
	})
}

func keepOpen(e ui.Element) ui.Element {
	return e.OnClick(func() {})
}

func (v *View) lightboxPending(c *ui.Context, tk theme.Tokens, name string) {
	if v.photos.hasFailed(fullSize.key(name)) {
		ui.Text(c, locales.FileOpenFailed()).TextColor(tk.TextSecondary)
		return
	}
	kit.Spinner(c, tk, lightboxSpinner)
}
