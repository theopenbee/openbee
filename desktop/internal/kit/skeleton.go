package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	skeletonPulse = 1800 * time.Millisecond
	skeletonDim   = 0.52
	skeletonMix   = 0.38
)

func SkeletonBar(c *ui.Context, tk theme.Tokens, w, h float32) ui.Element {
	e := ui.Box(c).Height(h).Shrink(0).Radius(theme.RadiusControl).Background(skeletonFill(tk))
	if w > 0 {
		e.Width(w)
	}
	return pulse(e)
}

func SkeletonCircle(c *ui.Context, tk theme.Tokens, d float32) ui.Element {
	return pulse(ui.Box(c).Size(d, d).Shrink(0).Radius(theme.RadiusPill).Background(skeletonFill(tk)))
}

func skeletonFill(tk theme.Tokens) ui.Color {
	return tk.Surface.Mix(tk.BorderStrong, skeletonMix)
}

func pulse(e ui.Element) ui.Element {
	return e.Opacity(1 - (1-skeletonDim)*e.Loop("pulse", skeletonPulse, ui.Bounce(ui.EaseInOut)))
}
