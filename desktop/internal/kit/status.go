package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	statusRing       = 2
	statusPingPeriod = time.Second
	statusPingSpan   = 0.75
	statusPingScale  = 2
	statusPingAlpha  = 0.6
)

var pingEase = ui.CubicBezier(0, 0, 0.2, 1)

func statusColor(tk theme.Tokens, status string) ui.Color {
	switch status {
	case "idle":
		return tk.Success
	case "working":
		return tk.Accent
	case "error":
		return tk.Danger
	}
	return tk.BorderStrong
}

func statusLabel(status string) string {
	switch status {
	case "idle":
		return locales.StatusesIdle()
	case "working":
		return locales.StatusesWorking()
	case "error":
		return locales.StatusesError()
	}
	return ""
}

func StatusDot(c *ui.Context, tk theme.Tokens, status string, d float32) ui.Element {
	fill := statusColor(tk, status)
	dot := ui.Box(c).Size(d, d).Shrink(0).Radius(theme.RadiusPill).Background(fill).
		Shadow(0, 0, 0, statusRing, tk.Surface)
	if label := statusLabel(status); label != "" {
		Tooltip(c, tk, dot.Label(label).Role(ui.RoleImage), label)
	}
	return dot.Children(func() {
		if status != "working" {
			return
		}
		ping := pingEase(min(1, dot.Loop("ping", statusPingPeriod, ui.Linear)/statusPingSpan))
		scale := 1 + (statusPingScale-1)*ping
		ui.Box(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).Radius(theme.RadiusPill).Background(fill).
			Opacity(statusPingAlpha*(1-ping)).Scale(scale, scale)
	})
}
