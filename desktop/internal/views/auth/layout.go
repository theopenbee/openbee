package auth

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
)

const (
	columnWidth   = 420
	columnGap     = 32
	screenPadY    = 48
	screenPadX    = 16
	toolbarInset  = 16
	dragBand      = 32
	logoHeight    = 32
	logoAspect    = 1448.0 / 420.0
	cardPadding   = 32
	headingGap    = 8
	headingBottom = 24
	titleSize     = 36
	titleTracking = -0.03
	titleLeading  = 1.1
	regularWeight = 400
	bodySize      = 14
	formGap       = 16
	submitGap     = 8
)

const (
	ringCount    = 12
	ringBase     = 232
	ringStep     = 64
	ringMajor    = 3
	ringSegment  = 12
	lineWidth    = 1
	maskWidth    = 0.72
	maskHeight   = 0.78
	maskSolid    = 0.38
	axisSegments = 96
)

type Screens struct {
	s        *store.Store
	nav      route.Nav
	langOpen bool
}

func New(s *store.Store, nav route.Nav) *Screens {
	return &Screens{s: s, nav: nav}
}

func (a *Screens) layout(c *ui.Context, tk theme.Tokens, title, description string, body func()) {
	w, h := c.Size()
	width := min(columnWidth, w-2*screenPadX)
	ui.Column(c).Fill().Background(tk.Background).Clip().Draw(backdrop(tk)).Children(func() {
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).MinHeight(h).AlignItems(ui.Center).Justify(ui.Center).Padding(screenPadY, screenPadX).Children(func() {
				ui.Column(c).Width(width).Gap(columnGap).AlignItems(ui.Center).Children(func() {
					ui.Image(c, icons.LogoFull(tk)).Size(logoHeight*logoAspect, logoHeight)
					card(c, tk, title, description, body)
				})
			})
		})
		bar := c.TitleBar()
		ui.Row(c).Absolute().Top(0).Left(0).Size(w, max(bar.Height, dragBand)).DragWindow()
		ui.Row(c).Absolute().Top(toolbarInset).Right(toolbarInset + bar.Right).Children(func() {
			a.languageMenu(c, tk)
		})
	})
}

func card(c *ui.Context, tk theme.Tokens, title, description string, body func()) {
	ui.Column(c).AlignSelf(ui.Stretch).Padding(cardPadding).Radius(theme.RadiusSurface).
		Background(tk.Surface).Border(1, tk.Border).TextColor(tk.Foreground).Children(func() {
		ui.Column(c).Gap(headingGap).Margin(0, 0, headingBottom, 0).Children(func() {
			ui.Text(c, title).Font(theme.FontDisplay).FontSize(titleSize).FontWeight(regularWeight).
				LetterSpacing(titleTracking * titleSize).LineHeight(titleLeading).Role(ui.RoleHeading).Level(1)
			if description != "" {
				ui.Text(c, description).FontSize(bodySize).TextColor(tk.TextSecondary)
			}
		})
		body()
	})
}

type fade struct {
	bounds         ui.Rect
	cx, cy, rx, ry float32
	background     ui.Color
}

func (f fade) color(line ui.Color, x, y float32) (ui.Color, bool) {
	b := f.bounds
	if x < b.X-ringSegment || x > b.X+b.W+ringSegment || y < b.Y-ringSegment || y > b.Y+b.H+ringSegment {
		return line, false
	}
	dx, dy := (x-f.cx)/f.rx, (y-f.cy)/f.ry
	t := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	switch {
	case t >= 1:
		return line, false
	case t <= maskSolid:
		return line, true
	}
	return f.background.Mix(line, (1-t)/(1-maskSolid)), true
}

func backdrop(tk theme.Tokens) func(p *ui.Painter, r ui.Rect) {
	return func(p *ui.Painter, r ui.Rect) {
		f := fade{bounds: r, cx: r.X + r.W/2, cy: r.Y + r.H/2, rx: r.W * maskWidth, ry: r.H * maskHeight, background: tk.Background}
		trace(p, f, tk.BorderSubtle, axisSegments, func(i int) (float32, float32) {
			return r.X + r.W*float32(i)/axisSegments, f.cy
		})
		trace(p, f, tk.BorderSubtle, axisSegments, func(i int) (float32, float32) {
			return f.cx, r.Y + r.H*float32(i)/axisSegments
		})
		for i := range ringCount {
			line := tk.BorderSubtle
			if i%ringMajor == ringMajor-1 {
				line = tk.Border
			}
			radius := float32(ringBase + ringStep*i)
			steps := int(math.Ceil(2 * math.Pi * float64(radius) / ringSegment))
			trace(p, f, line, steps, func(i int) (float32, float32) {
				angle := 2 * math.Pi * float64(i) / float64(steps)
				return f.cx + radius*float32(math.Cos(angle)), f.cy + radius*float32(math.Sin(angle))
			})
		}
	}
}

func trace(p *ui.Painter, f fade, line ui.Color, steps int, point func(i int) (float32, float32)) {
	x0, y0 := point(0)
	for i := 1; i <= steps; i++ {
		x1, y1 := point(i)
		if c, ok := f.color(line, (x0+x1)/2, (y0+y1)/2); ok {
			segment(p, x0, y0, x1, y1, c)
		}
		x0, y0 = x1, y1
	}
}

func segment(p *ui.Painter, x0, y0, x1, y1 float32, c ui.Color) {
	dx, dy := x1-x0, y1-y0
	length := float32(math.Hypot(float64(dx), float64(dy)))
	degrees := float32(math.Atan2(float64(dy), float64(dx)) * 180 / math.Pi)
	p.Transform(ui.Rotation(degrees), x0, y0, func() {
		p.Fill(ui.Rect{X: x0, Y: y0 - lineWidth/2, W: length, H: lineWidth}, c, 0)
	})
}
