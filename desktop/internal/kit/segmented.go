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

	compactTrackHeight   = 26
	compactTrackPadding  = 2
	compactSegment       = 28
	compactSegmentWide   = 42
	compactFontSize      = 12
	compactIcon          = 14
	compactTrackTint     = 0.05
	compactThumbLine     = 0.09
	compactThumbShadow   = 0.08
	compactThumbDark     = 0.2
	compactThumbHairline = 0.5
	compactDisabled      = 0.5
)

type CompactOpts struct{ Wide, Disabled bool }

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
	return paintSelection(c, track, chosen, radius, paintRegularThumb(tk))
}

func SegmentedCompact(c *ui.Context, tk theme.Tokens, selected *int, label string, items []SegmentItem, o CompactOpts) ui.Element {
	parts := ui.SegmentedBase(c, selected, len(items))
	trackRadius := float32(theme.RadiusInset)
	thumbRadius := theme.Concentric(trackRadius, compactTrackPadding)
	width := float32(compactSegment)
	if o.Wide {
		width = compactSegmentWide
	}
	track := parts.Track.Label(label).AlignSelf(ui.Start).AlignItems(ui.Stretch).MinWidth(0).Height(compactTrackHeight).
		Padding(compactTrackPadding).Radius(trackRadius).Background(tk.Foreground.Alpha(compactTrackTint))
	var chosen ui.Element
	track.Children(func() {
		for i, it := range items {
			on := i == *selected
			seg := parts.Segment(i).Width(width).Shrink(0).Radius(thumbRadius).FontSize(compactFontSize).FontWeight(mediumWeight)
			if on {
				chosen = seg
			}
			fg := tk.TextMuted
			switch {
			case on:
				fg = tk.Foreground
			case seg.Hovered() || seg.Pressed():
				fg = tk.TextSecondary
			}
			seg.TextColor(seg.AnimateColor("fg", fg, fast))
			if it.Text == "" {
				seg.Label(it.Tooltip)
			}
			Tooltip(c, tk, seg, it.Tooltip)
			seg.Children(func() {
				if it.Icon != "" {
					ui.Icon(c, icons.Get(it.Icon)).Size(compactIcon, compactIcon)
				}
				if it.Text != "" {
					ui.Text(c, it.Text).SingleLine()
				}
			})
		}
	})
	if o.Disabled {
		track.Disabled(true).Opacity(compactDisabled)
	}
	if len(items) == 0 {
		return track
	}
	return paintSelection(c, track, chosen, thumbRadius, paintCompactThumb(tk))
}

type thumbPainter func(p *ui.Painter, box ui.Rect, radius float32)

func paintRegularThumb(tk theme.Tokens) thumbPainter {
	return func(p *ui.Painter, box ui.Rect, radius float32) {
		for _, s := range tk.ShadowResting {
			p.Shadow(box, radius, s.X, s.Y, s.Blur, s.Spread, s.Color)
		}
		p.Fill(box, tk.Surface, radius)
		p.Stroke(box, tk.Border, radius, 1)
	}
}

func paintCompactThumb(tk theme.Tokens) thumbPainter {
	return func(p *ui.Painter, box ui.Rect, radius float32) {
		if tk.Dark {
			p.Fill(box, tk.SurfaceRaised.Mix(tk.Foreground, compactThumbDark), radius)
			return
		}
		p.Shadow(box, radius, 0, 1, 1.5, 0, tk.Foreground.Alpha(compactThumbShadow))
		p.Fill(box, tk.SurfaceRaised, radius)
		p.Stroke(box, tk.Foreground.Alpha(compactThumbLine), radius, compactThumbHairline)
	}
}

func paintSelection(c *ui.Context, track, chosen ui.Element, radius float32, paint thumbPainter) ui.Element {
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
		paint(p, ui.Rect{X: r.X + x, Y: r.Y + y, W: w, H: h}, radius)
	})
}
