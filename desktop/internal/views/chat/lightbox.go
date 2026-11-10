package chat

import (
	"fmt"
	"math"
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
	lightboxZoomGap   = 4
	lightboxBottom    = 32
	lightboxPadNarrow = 16
	lightboxPadWide   = 48
	lightboxSpinner   = 24
	lightboxDragSlop  = 4
	lightboxBorder    = 1
	zoomLabelWidth    = 56
)

type lightboxState struct {
	open   bool
	name   string
	zoom   float32
	scroll ui.ScrollState
	stage  zoomStage
	press  *lightboxPress
}

type lightboxPress struct {
	x, y, sx, sy float32
	image, moved bool
}

func (v *View) openLightbox(name string) {
	v.light = lightboxState{open: true, name: name}
}

func (l *lightboxState) scale() float32 {
	if l.zoom > 0 {
		return l.zoom
	}
	return l.stage.fit()
}

func (l *lightboxState) zoomTo(next, px, py float32) {
	s := l.stage
	if !s.ready() {
		return
	}
	lo, hi := s.bounds()
	from, to := l.scale(), clampZoom(next, lo, hi)
	if sameZoom(from, to) {
		return
	}
	l.scroll.X, l.scroll.Y = s.rescroll(from, to, px, py, l.scroll.X, l.scroll.Y)
	l.zoom = to
	if sameZoom(to, s.fit()) {
		l.zoom = 0
	}
}

func (l *lightboxState) zoomCenter(next float32) {
	l.zoomTo(next, l.stage.w/2, l.stage.h/2)
}

func (l *lightboxState) step(up bool) {
	if !l.stage.ready() {
		return
	}
	lo, hi := l.stage.bounds()
	l.zoomCenter(stepZoom(l.scale(), up, lo, hi))
}

func (l *lightboxState) input(ev ui.InputEvent) bool {
	s := l.stage
	if !s.ready() {
		return false
	}
	switch ev.Kind {
	case ui.InputZoom:
		l.zoomTo(l.scale()*ev.Scale, ev.X, ev.Y)
		return true
	case ui.InputScroll:
		if ev.Mods&(ui.Ctrl|ui.Super) == 0 {
			return false
		}
		l.zoomTo(l.scale()*wheelZoomFactor(ev.DY, ev.Precise), ev.X, ev.Y)
		return true
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		scale := l.scale()
		onImage := s.onImage(scale, l.scroll.X+ev.X, l.scroll.Y+ev.Y)
		if !onImage && !s.overflows(scale) {
			return false
		}
		if ev.Clicks == 2 && onImage {
			l.zoomTo(toggleZoom(scale, s.fit()), ev.X, ev.Y)
		}
		l.press = &lightboxPress{x: ev.X, y: ev.Y, sx: l.scroll.X, sy: l.scroll.Y, image: onImage}
		return true
	case ui.InputPointerMove:
		p := l.press
		if p == nil {
			return false
		}
		dx, dy := ev.X-p.x, ev.Y-p.y
		if !p.moved && math.Hypot(float64(dx), float64(dy)) < lightboxDragSlop {
			return true
		}
		p.moved = true
		l.scroll.X, l.scroll.Y = s.clampScroll(l.scale(), p.sx-dx, p.sy-dy)
		return true
	case ui.InputPointerUp:
		p := l.press
		if p == nil {
			return false
		}
		l.press = nil
		if !p.moved && !p.image {
			l.open = false
		}
		return true
	}
	return false
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
		if c.Shortcut(0, ui.KeyEqual) || c.Shortcut(ui.Shift, ui.KeyEqual) {
			l.step(true)
		}
		if c.Shortcut(0, ui.KeyMinus) {
			l.step(false)
		}
		if c.Shortcut(0, ui.Key0) {
			l.zoomCenter(l.stage.fit())
		}
		labels := w >= theme.BreakTwoPane
		v.viewerBar(c, tk, name, w, locales.ChatCloseImage(), func() { l.open = false }, func() {
			v.zoomControls(c, tk, w)
			v.viewerButton(c, tk, labels, viewerAction{
				text:  locales.ChatDownload(),
				icon:  "download",
				busy:  v.downloading(name),
				click: func() { v.downloadMedia(name) },
			})
			v.viewerButton(c, tk, labels, viewerAction{
				text:  locales.ChatOpenOriginal(),
				icon:  "external-link",
				click: func() { v.openMedia(name) },
			})
		})
		v.lightboxStage(c, tk, name, w, h)
	})
}

func (v *View) zoomControls(c *ui.Context, tk theme.Tokens, w float32) {
	l := &v.light
	ready := l.stage.ready()
	scale := l.scale()
	lo, hi := l.stage.bounds()
	md := kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD}
	if w >= theme.BreakComposer {
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(lightboxZoomGap).Children(func() {
			out := md
			out.Disabled = !ready || scale <= lo+zoomEpsilon
			if kit.IconButton(c, tk, "zoom-out", locales.ChatZoomOut(), out).Clicked() {
				l.step(false)
			}
			percent := "–"
			if ready {
				percent = fmt.Sprintf("%d%%", int(math.Round(float64(scale*100))))
			}
			ui.Text(c, percent).FontFeatures("tnum").TextColor(tk.TextSecondary).TextAlign(ui.Center).Width(zoomLabelWidth).SingleLine()
			in := md
			in.Disabled = !ready || scale >= hi-zoomEpsilon
			if kit.IconButton(c, tk, "zoom-in", locales.ChatZoomIn(), in).Clicked() {
				l.step(true)
			}
		})
	}
	fit := md
	fit.Disabled = !ready || l.zoom == 0
	if kit.IconButton(c, tk, "scan", locales.ChatZoomFit(), fit).Clicked() {
		l.zoomCenter(l.stage.fit())
	}
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

func (v *View) viewerButton(c *ui.Context, tk theme.Tokens, labelled bool, action viewerAction) {
	text := action.text
	if !labelled {
		text = ""
	}
	opts := kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Icon: action.icon, Label: action.text, Loading: action.busy}
	if kit.Button(c, tk, text, opts).Clicked() {
		action.click()
	}
}

func (v *View) viewerBar(c *ui.Context, tk theme.Tokens, title string, w float32, closeLabel string, close func(), actions func()) {
	inset := c.TitleBar()
	bar := ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(lightboxBarGap).PaddingY(lightboxBarPad).
		PaddingStart(max(lightboxBarPad, inset.Left)).PaddingEnd(max(lightboxBarPad, inset.Right))
	keepOpen(bar).Children(func() {
		ui.Text(c, title).FontWeight(mediumWeight).TextColor(tk.Foreground).SingleLine().Grow(1).MinWidth(0)
		actions()
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
	l := &v.light
	pad := viewerPad(w)
	src, ok := v.photo(c, name, fullSize)
	if !ok {
		ui.Column(c).Grow(1).MinHeight(0).Center().Padding(0, pad, lightboxBottom, pad).Children(func() {
			v.lightboxPending(c, tk, name)
		})
		return
	}
	iw, ih := sourceSize(src)
	if iw <= 0 || ih <= 0 {
		return
	}
	barH := float32(theme.ControlMD + 2*lightboxBarPad)
	l.stage = zoomStage{w: w, h: h - barH, pad: pad, bottom: lightboxBottom, border: lightboxBorder, iw: iw, ih: ih}
	s := l.stage
	scale := l.scale()
	cw, ch := s.content(scale)
	ox, oy := s.origin(scale)
	ow, oh := s.outer(scale)
	stage := ui.ScrollBoth(c).Grow(1).MinHeight(0).TrackScroll(&l.scroll).HandleInput(l.input)
	switch {
	case l.press != nil && l.press.moved:
		stage.Cursor(ui.CursorGrabbing)
	case s.overflows(scale):
		stage.Cursor(ui.CursorGrab)
	}
	stage.Children(func() {
		ui.Column(c).Size(cw, ch).Padding(oy, 0, 0, ox).AlignItems(ui.Start).Children(func() {
			img := ui.Image(c, src).Label(name).Fit(ui.Contain).Size(ow, oh).Radius(theme.RadiusPanel).
				Border(lightboxBorder, tk.Border).Background(tk.Surface).Clip().Shrink(0)
			theme.WithShadow(img, tk.ShadowFloating)
		})
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
