package theme

import "github.com/egoist/mygo/ui"

type Shadow struct {
	X, Y, Blur, Spread float32
	Color              ui.Color
}

func WithShadow(e ui.Element, s []Shadow) ui.Element {
	for i := len(s) - 1; i >= 0; i-- {
		sh := s[i]
		e = e.Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color)
	}
	return e
}
