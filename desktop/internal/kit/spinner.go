package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	spinnerPeriod  = time.Second
	shimmerSweep   = 1800 * time.Millisecond
	shimmerRest    = 0.32
	shimmerBandEm  = 2.2
	shimmerBaseMix = 0.12
)

var shimmerFalloff = [...]struct{ at, strength float32 }{
	{-1, 0}, {-0.5, 0.5}, {-0.2, 0.9}, {0, 1}, {0.2, 0.9}, {0.5, 0.5}, {1, 0},
}

func Spinner(c *ui.Context, tk theme.Tokens, size float32) ui.Element {
	return spin(c, size, spinnerPeriod).TextColor(tk.TextMuted)
}

func spin(c *ui.Context, size float32, period time.Duration) ui.Element {
	s := ui.Icon(c, icons.Get("loader-circle")).Size(size, size).Shrink(0).Role(ui.RoleProgress)
	return s.Rotate(s.Loop("spin", period, ui.Linear) * 360)
}

func Shimmer(c *ui.Context, tk theme.Tokens, text string) ui.Element {
	highlight := tk.TextSecondary
	base := tk.TextMuted.Mix(highlight, shimmerBaseMix)
	stage := ui.Row(c).MinWidth(0)
	period := time.Duration(float64(shimmerSweep) * (1 + shimmerRest))
	sweep := min(1, stage.Loop("shimmer", period, ui.Linear)*(1+shimmerRest))
	runes := []rune(text)
	widths := make([]float32, len(runes))
	var total float32
	for i, r := range runes {
		w, _ := c.MeasureText(0, ui.Span{Text: string(r), Size: bodyFontSize})
		widths[i] = w
		total += w
	}
	band := float32(shimmerBandEm * bodyFontSize)
	centre := sweep*total + (2*sweep-1)*band
	spans := make([]ui.Span, len(runes))
	var x float32
	for i, r := range runes {
		mid := x + widths[i]/2
		x += widths[i]
		spans[i] = ui.Span{Text: string(r), Color: base.Mix(highlight, falloff((mid-centre)/band))}
	}
	return stage.Children(func() {
		ui.RichText(c, spans...).FontSize(bodyFontSize).SingleLine()
	})
}

func falloff(at float32) float32 {
	stops := shimmerFalloff
	if at <= stops[0].at || at >= stops[len(stops)-1].at {
		return 0
	}
	for i := 1; i < len(stops); i++ {
		if at <= stops[i].at {
			a, b := stops[i-1], stops[i]
			return a.strength + (b.strength-a.strength)*(at-a.at)/(b.at-a.at)
		}
	}
	return 0
}
