package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

type SegmentItem struct{ Text, Icon, Tooltip string }

const (
	segmentTrackPadding = 3
	segmentGap          = 2
	segmentPaddingX     = 12
	segmentLabelGap     = 6
	segmentIcon         = 16
)

func Segmented(c *ui.Context, tk theme.Tokens, selected *int, label string, items []SegmentItem) ui.Element {
	parts := ui.SegmentedBase(c, selected, len(items))
	radius := theme.Concentric(theme.RadiusControl, segmentTrackPadding+1)
	track := parts.Track.Label(label).AlignSelf(ui.Start).AlignItems(ui.Stretch).MinWidth(0).Padding(segmentTrackPadding).
		Gap(segmentGap).Radius(theme.RadiusControl).Background(tk.SurfaceMuted).Border(1, tk.Border)
	var chosen ui.Element
	track.Children(func() {
		for i, it := range items {
			on := i == *selected
			seg := parts.Segment(i).MinHeight(theme.ControlSM).PaddingX(segmentPaddingX).Radius(radius).
				FontSize(bodyFontSize).FontWeight(mediumWeight)
			if on {
				chosen = seg
			}
			fg := tk.TextMuted
			if on || seg.Hovered() || seg.Pressed() {
				fg = tk.Foreground
			}
			seg.TextColor(seg.AnimateColor("fg", fg, fast))
			if it.Text == "" {
				seg.Label(it.Tooltip)
			}
			Tooltip(c, tk, seg, it.Tooltip)
			seg.Children(func() {
				ui.Row(c).Gap(segmentLabelGap).Children(func() {
					if it.Icon != "" {
						ui.Icon(c, icons.Get(it.Icon)).Size(segmentIcon, segmentIcon)
					}
					if it.Text != "" {
						ui.Text(c, it.Text).SingleLine()
					}
				})
			})
		}
	})
	if len(items) == 0 {
		return track
	}
	return paintSelection(c, tk, track, chosen, radius)
}

func paintSelection(c *ui.Context, tk theme.Tokens, track, chosen ui.Element, radius float32) ui.Element {
	at := chosen.LayoutBounds()
	if at.W == 0 {
		c.AnimationFrame()
		return track
	}
	origin := track.LayoutBounds()
	x := track.AnimateTo("selection-x", at.X-origin.X, morph)
	w := track.AnimateTo("selection-w", at.W, morph)
	y, h := at.Y-origin.Y, at.H
	return track.Draw(func(p *ui.Painter, r ui.Rect) {
		box := ui.Rect{X: r.X + x, Y: r.Y + y, W: w, H: h}
		for _, s := range tk.ShadowResting {
			p.Shadow(box, radius, s.X, s.Y, s.Blur, s.Spread, s.Color)
		}
		p.Fill(box, tk.Surface, radius)
		p.Stroke(box, tk.Border, radius, 1)
	})
}
