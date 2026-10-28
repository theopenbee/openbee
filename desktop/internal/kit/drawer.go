package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	drawerSlideTime = 480 * time.Millisecond
	drawerCloseRest = 0.65
	drawerInset     = 12
	squareCorner    = 0
)

var easeDrawer = ui.CubicBezier(0.32, 0.72, 0, 1)

func Drawer(c *ui.Context, tk theme.Tokens, open *bool, title string, width float32, body func()) ui.Element {
	w, _ := c.Size()
	if width <= 0 {
		width = theme.DrawerMaxWidth
	}
	width = min(width, w)
	radius := theme.Concentric(theme.RadiusSurface, drawerInset)
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(tk.DrawerScrim.Alpha(fadeIn(backdrop)))
		slide := panel.Keyframes("slide", []ui.Keyframe{{At: 0, Value: 0}, {At: drawerSlideTime, Value: 1, Ease: easeDrawer}})
		floatingPanel(panel, tk, radius).Radius(radius, squareCorner, squareCorner, radius).BorderWidth(1, 0, 1, 1).
			Label(title).Absolute().Top(0).Right(0).Bottom(0).Width(width).Clip().Translate(width*(1-slide), 0)
		overlayHeader(c, tk, title, "", func() {
			closeButton(c, tk, locales.ArcDrawerClose(), drawerCloseRest).
				Margin(closeOffset, 0, 0, 0).OnClick(func() { *open = false })
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Padding(dialogPadding).FontSize(bodyFontSize).Children(body)
		})
	})
}
