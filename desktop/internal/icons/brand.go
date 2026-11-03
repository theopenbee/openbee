package icons

import (
	"bytes"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

var brandVar = []byte("var(--brand)")

func hexOf(c ui.Color) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func brandData(name string) []byte {
	data, err := brandFS.ReadFile("brand/" + name + ".svg")
	if err != nil {
		panic(fmt.Sprintf("icons: missing brand image %q", name))
	}
	return data
}

func brandSource(name string, t theme.Tokens) []byte {
	return bytes.ReplaceAll(brandData(name), brandVar, []byte(hexOf(t.Brand)))
}

func logo(name string, t theme.Tokens) *ui.SVG {
	key := fmt.Sprintf("brand/%s/dark=%t", name, t.Dark)
	return cached(key, func() *ui.SVG { return ui.MustParseSVG(brandSource(name, t)) })
}

func LogoMark(t theme.Tokens) *ui.SVG {
	return logo("logo-mark", t)
}

func LogoFull(t theme.Tokens) *ui.SVG {
	return logo("logo-full", t)
}
