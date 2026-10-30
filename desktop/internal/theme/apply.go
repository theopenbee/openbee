package theme

import "github.com/egoist/mygo/ui"

const (
	baseFontSize      float32 = 14
	focusAlpha        float32 = 0.72
	accentHoverMix    float32 = 0.25
	accentPressedMix  float32 = 0.5
	surfacePressedMix float32 = 0.5
	fontStack                 = FontBody + ", system-ui"
)

const BodyLineHeight float32 = 1.4

func Apply(c *ui.Context) Tokens {
	t := *c.Theme()
	tk := Light()
	if t.Dark {
		tk = Dark()
	}
	t.Background = tk.Background
	t.Surface = tk.Surface
	t.SurfaceHover = tk.SurfaceMuted
	t.SurfacePressed = tk.SurfaceMuted.Mix(tk.Border, surfacePressedMix)
	t.Border = tk.Border
	t.Text = tk.Foreground
	t.TextMuted = tk.TextSecondary
	t.Accent = tk.ControlOn
	t.AccentHover = tk.ControlOn.Mix(tk.AccentStrong, accentHoverMix)
	t.AccentPressed = tk.ControlOn.Mix(tk.AccentStrong, accentPressedMix)
	t.AccentText = tk.AccentForeground
	t.Danger = tk.Danger
	t.Warning = tk.Warning
	t.Success = tk.Success
	t.Selection = tk.AccentSubtle
	t.Focus = tk.AccentStrong.Alpha(focusAlpha)
	t.Inverse = tk.Foreground
	t.InverseText = tk.Background
	t.Radius = RadiusControl
	t.Font = fontStack
	t.FontSize = baseFontSize
	c.SetTheme(&t)
	c.Root().LineHeight(BodyLineHeight)
	return tk
}
