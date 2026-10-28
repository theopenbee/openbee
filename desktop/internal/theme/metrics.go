package theme

const RadiusControl, RadiusPanel, RadiusSurface, RadiusInset, RadiusPill float32 = 18, 26, 34, 10, 9999

const minConcentricRadius float32 = 6

func Concentric(outer, padding float32) float32 {
	return max(minConcentricRadius, outer-padding)
}

const (
	ControlSM, ControlMD, ControlLG = 36, 44, 50

	HeaderHeight   = 64
	SidebarWidth   = 304
	PanelWidth     = 400
	DrawerMaxWidth = 448
	ChatColumnMax  = 768
	DialogWidth    = 440

	SettingsMaxW, SettingsMaxH = 1152, 832

	BreakTwoPane     = 768
	BreakInlinePanel = 1184
	BreakComposer    = 640
)
