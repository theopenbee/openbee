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
		v.viewerFrame(tk, backdrop, panel, w, h, locales.ChatViewImage(name))
		if panel.Clicked() {
			l.open = false
		}
		v.viewerBar(c, tk, name, w, viewerAction{
			text:  locales.ChatOpenOriginal(),
			icon:  "external-link",
			click: func() { v.openMedia(name) },
		}, locales.ChatCloseImage(), func() { l.open = false })
		v.lightboxStage(c, tk, name, w, h)
	})
}

func (v *View) viewerFrame(tk theme.Tokens, backdrop, panel ui.Element, w, h float32, label string) {
	fade := backdrop.Keyframes("fade", []ui.Keyframe{{At: 0, Value: 0}, {At: lightboxFade, Value: 1, Ease: easeEnter}})
	backdrop.Background(tk.Background.Alpha(lightboxScrim * fade))
	rise := panel.Keyframes("rise", []ui.Keyframe{{At: 0, Value: 0}, {At: lightboxFade, Value: 1, Ease: easeEnter}})
	scale := lightboxScale + (1-lightboxScale)*rise
	panel.Size(w, h).Label(label).TextColor(tk.Foreground).FontSize(bodySize).Opacity(rise).Scale(scale, scale)
}

type viewerAction struct {
	text, icon string
	busy       bool
	click      func()
}

func (v *View) viewerBar(c *ui.Context, tk theme.Tokens, title string, w float32, action viewerAction, closeLabel string, close func()) {
	inset := c.TitleBar()
	bar := ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(lightboxBarGap).PaddingY(lightboxBarPad).
		PaddingStart(max(lightboxBarPad, inset.Left)).PaddingEnd(max(lightboxBarPad, inset.Right))
	keepOpen(bar).Children(func() {
		ui.Text(c, title).FontWeight(mediumWeight).TextColor(tk.Foreground).SingleLine().Grow(1).MinWidth(0)
		text := action.text
		if w < theme.BreakComposer {
			text = ""
		}
		opts := kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Icon: action.icon, Label: action.text, Loading: action.busy}
		if kit.Button(c, tk, text, opts).Clicked() {
			action.click()
		}
		if kit.IconButton(c, tk, "x", closeLabel, kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD}).Clicked() {
			close()
		}
	})
}

func viewerPad(w float32) float32 {
	if w >= theme.BreakComposer {
		return lightboxPadWide
	}
	return lightboxPadNarrow
}

func (v *View) lightboxStage(c *ui.Context, tk theme.Tokens, name string, w, h float32) {
	pad := viewerPad(w)
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
