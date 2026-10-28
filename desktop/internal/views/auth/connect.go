package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const defaultServer = "http://localhost:8080"

type connectForm struct {
	address string
	tried   string
	err     error
	loading bool
}

type connectKey struct{}

func (a *Screens) Connect(c *ui.Context, tk theme.Tokens, page ui.Element) {
	f := ui.Local(page, connectKey{}, func() connectForm { return connectForm{address: defaultServer} })
	submit := func() { a.connect(f) }
	a.layout(c, tk, locales.ConnectTitle(), locales.ConnectDescription(), func() {
		ui.Column(c).Gap(formGap).Children(func() {
			errText := connectError(f)
			help := locales.ConnectServerHelp()
			if errText != "" {
				help = ""
			}
			kit.Field(c, tk, locales.ConnectServerLabel(), help, errText, func() {
				kit.TextInput(c, tk, &f.address, locales.ConnectServerLabel(), "").AutoFocus().
					OnChange(func() { f.err = nil }).OnSubmit(submit)
			})
			kit.Button(c, tk, locales.ConnectSubmit(), kit.ButtonOpts{Loading: f.loading}).
				AlignSelf(ui.Stretch).Margin(submitGap, 0, 0, 0).OnClick(submit)
		})
	})
}

func (a *Screens) connect(f *connectForm) {
	if f.loading {
		return
	}
	f.err = nil
	f.loading = true
	f.tried = shownAddress(f.address)
	a.s.Connect(f.address, func(err error) {
		f.loading = false
		if !errors.Is(err, context.Canceled) {
			f.err = err
		}
	})
}

func shownAddress(raw string) string {
	if base, err := api.NormalizeBaseURL(raw); err == nil {
		return base
	}
	return strings.TrimSpace(raw)
}

func connectError(f *connectForm) string {
	switch {
	case f.err == nil:
		return ""
	case errors.Is(f.err, store.ErrInvalidURL):
		return locales.ConnectErrorInvalid()
	case errors.Is(f.err, api.ErrNotOpenBee):
		return locales.ConnectErrorNotOpenBee(f.tried)
	}
	return locales.ConnectErrorUnreachable(f.tried)
}
