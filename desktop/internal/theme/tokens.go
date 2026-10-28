package theme

import "github.com/egoist/mygo/ui"

type Tokens struct {
	Dark bool

	Background, Surface, SurfaceRaised, SurfaceMuted ui.Color
	Foreground, TextSecondary, TextMuted             ui.Color
	Border, BorderSubtle, BorderStrong               ui.Color
	Accent, AccentStrong, AccentSubtle               ui.Color
	AccentForeground                                 ui.Color
	Success, Warning, Danger, Brand                  ui.Color
	ControlOn, ControlTrack, ControlThumb            ui.Color
	ControlTrackHover                                ui.Color
	AvatarBee, AvatarForeground                      ui.Color
	Avatars                                          [8]ui.Color
	Scrim, DrawerScrim                               ui.Color

	ShadowResting, ShadowRaised, ShadowFloating []Shadow
	ShadowThumb                                 []Shadow
}

var avatarHues = [8]float32{25, 60, 95, 150, 195, 240, 285, 330}

func avatarPalette(lightness float32) [8]ui.Color {
	var out [8]ui.Color
	for i, hue := range avatarHues {
		out[i] = ui.Oklch(lightness, 0.11, hue)
	}
	return out
}

func black(alpha float32) ui.Color {
	return ui.Oklch(0, 0, 0).Alpha(alpha)
}

func scrim(alpha float32) ui.Color {
	return ui.Oklch(0.10, 0, 0).Alpha(alpha)
}

var orange = ui.Hex("#f48120")

func Light() Tokens {
	surfaceMuted := ui.Oklch(0.978, 0, 0)
	return Tokens{
		Dark:              false,
		Background:        ui.Oklch(1, 0, 0),
		Surface:           ui.Oklch(1, 0, 0),
		SurfaceRaised:     ui.Oklch(1, 0, 0),
		SurfaceMuted:      surfaceMuted,
		Foreground:        ui.Oklch(0.15, 0, 0),
		TextSecondary:     ui.Oklch(0.46, 0, 0),
		TextMuted:         ui.Oklch(0.59, 0, 0),
		Border:            ui.Oklch(0.935, 0, 0),
		BorderSubtle:      ui.Oklch(0.965, 0, 0),
		BorderStrong:      ui.Oklch(0.82, 0, 0),
		Accent:            orange,
		AccentStrong:      ui.Hex("#aa4700"),
		AccentSubtle:      orange.Alpha(0.13),
		AccentForeground:  ui.Hex("#17130f"),
		Success:           ui.Oklch(0.49, 0.19, 150),
		Warning:           ui.Oklch(0.58, 0.18, 75),
		Danger:            ui.Oklch(0.54, 0.21, 25),
		Brand:             ui.Oklch(0.70, 0.161, 49),
		ControlOn:         ui.Hex("#f07818"),
		ControlTrack:      ui.Oklch(0.895, 0, 0),
		ControlThumb:      ui.Oklch(1, 0, 0),
		ControlTrackHover: ui.Oklch(0.86, 0, 0),
		AvatarBee:         surfaceMuted,
		AvatarForeground:  ui.Oklch(0.15, 0, 0),
		Avatars:           avatarPalette(0.72),
		Scrim:             scrim(0.46),
		DrawerScrim:       scrim(0.38),
		ShadowResting: []Shadow{
			{0, 1, 2, 0, black(0.035)},
		},
		ShadowRaised: []Shadow{
			{0, 6, 18, 0, black(0.065)},
			{0, 1, 3, 0, black(0.035)},
		},
		ShadowFloating: []Shadow{
			{0, 20, 48, 0, black(0.105)},
			{0, 3, 10, 0, black(0.045)},
		},
		ShadowThumb: []Shadow{
			{0, 0, 0, 0.5, black(0.07)},
			{0, 1, 2, 0, black(0.14)},
			{0, 2, 6, 0, black(0.06)},
		},
	}
}

func Dark() Tokens {
	return Tokens{
		Dark:              true,
		Background:        ui.Oklch(0.19, 0, 0),
		Surface:           ui.Oklch(0.215, 0, 0),
		SurfaceRaised:     ui.Oklch(0.24, 0, 0),
		SurfaceMuted:      ui.Oklch(0.26, 0, 0),
		Foreground:        ui.Oklch(0.96, 0, 0),
		TextSecondary:     ui.Oklch(0.77, 0, 0),
		TextMuted:         ui.Oklch(0.64, 0, 0),
		Border:            ui.Oklch(0.29, 0, 0),
		BorderSubtle:      ui.Oklch(0.25, 0, 0),
		BorderStrong:      ui.Oklch(0.40, 0, 0),
		Accent:            ui.Hex("#ffad5e"),
		AccentStrong:      ui.Hex("#ffd0a0"),
		AccentSubtle:      orange.Alpha(0.12),
		AccentForeground:  ui.Hex("#17130f"),
		Success:           ui.Oklch(0.78, 0.18, 150),
		Warning:           ui.Oklch(0.83, 0.17, 80),
		Danger:            ui.Oklch(0.76, 0.2, 25),
		Brand:             ui.Oklch(0.753, 0.135, 51),
		ControlOn:         ui.Hex("#ef873a"),
		ControlTrack:      ui.Oklch(0.33, 0, 0),
		ControlThumb:      ui.Oklch(0.97, 0, 0),
		ControlTrackHover: ui.Oklch(0.37, 0, 0),
		AvatarBee:         ui.Oklch(0.62, 0, 0),
		AvatarForeground:  ui.Oklch(0.15, 0, 0),
		Avatars:           avatarPalette(0.58),
		Scrim:             scrim(0.46),
		DrawerScrim:       scrim(0.38),
		ShadowResting: []Shadow{
			{0, 1, 2, 0, black(0.18)},
		},
		ShadowRaised: []Shadow{
			{0, 8, 22, 0, black(0.22)},
		},
		ShadowFloating: []Shadow{
			{0, 22, 55, 0, black(0.34)},
		},
		ShadowThumb: []Shadow{
			{0, 0, 0, 0.5, black(0.3)},
			{0, 1, 2, 0, black(0.35)},
			{0, 2, 6, 0, black(0.2)},
		},
	}
}
