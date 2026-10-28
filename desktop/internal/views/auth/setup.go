package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const minPasswordLength = 6

type setupForm struct {
	username, displayName, password string
	shown                           bool
	usernameMissing                 bool
	passwordShort                   bool
	failure                         error
	loading                         bool
	focus                           string
}

type setupKey struct{}

func (a *Screens) Setup(c *ui.Context, tk theme.Tokens, page ui.Element) {
	f := ui.Local(page, setupKey{}, func() setupForm { return setupForm{focus: fieldUsername} })
	submit := func() { a.setup(f) }
	a.layout(c, tk, locales.SetupTitle(), locales.SetupDescription(), func() {
		ui.Column(c).Gap(formGap).Children(func() {
			if f.failure != nil {
				a.setupAlert(c, tk, f.failure)
			}
			kit.Field(c, tk, locales.SetupUsername(), "", missing(f.usernameMissing, locales.SetupUsernameRequired()), func() {
				in := kit.TextInput(c, tk, &f.username, locales.SetupUsername(), locales.SetupUsernamePlaceholder()).
					OnChange(func() { f.usernameMissing = false }).OnSubmit(submit)
				focusOnce(in, &f.focus, fieldUsername)
			})
			kit.Field(c, tk, locales.SetupDisplayName(), locales.SetupDisplayNameOptional(), "", func() {
				kit.TextInput(c, tk, &f.displayName, locales.SetupDisplayName(), locales.SetupDisplayNamePlaceholder()).
					OnSubmit(submit)
			})
			hint := locales.SetupPasswordHint()
			if f.passwordShort {
				hint = ""
			}
			kit.Field(c, tk, locales.SetupPassword(), hint, missing(f.passwordShort, locales.SetupErrorPasswordLength()), func() {
				in := kit.PasswordInput(c, tk, &f.password, &f.shown, locales.SetupPassword(), "").
					OnChange(func() { f.passwordShort = false }).OnSubmit(submit)
				focusOnce(in, &f.focus, fieldPassword)
			})
			kit.Button(c, tk, locales.SetupSubmit(), kit.ButtonOpts{Loading: f.loading}).
				AlignSelf(ui.Stretch).Margin(submitGap, 0, 0, 0).OnClick(submit)
			a.serverRow(c, tk)
		})
	})
}

func (a *Screens) setupAlert(c *ui.Context, tk theme.Tokens, err error) {
	o := kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.SetupErrorTitle(), Body: setupError(err)}
	if api.IsStatus(err, http.StatusConflict) {
		o.ActionLabel = locales.SetupGoToLogin()
		o.OnAction = func() { a.nav.Replace(route.LoginPath) }
	}
	kit.Alert(c, tk, o)
}

func (a *Screens) setup(f *setupForm) {
	if f.loading {
		return
	}
	username := strings.TrimSpace(f.username)
	f.usernameMissing = username == ""
	f.passwordShort = utf8.RuneCountInString(f.password) < minPasswordLength
	switch {
	case f.usernameMissing:
		f.focus = fieldUsername
		return
	case f.passwordShort:
		f.focus = fieldPassword
		return
	}
	f.failure = nil
	f.loading = true
	req := api.SetupRequest{Username: username, Password: f.password, DisplayName: strings.TrimSpace(f.displayName)}
	a.s.Setup(req, func(err error) {
		f.loading = false
		if err != nil && !errors.Is(err, context.Canceled) {
			f.failure = err
		}
	})
}

func setupError(err error) string {
	var apiErr *api.Error
	switch {
	case api.IsStatus(err, http.StatusConflict):
		return locales.SetupErrorConflict()
	case errors.As(err, &apiErr):
		if text := l10n.ErrorText(err); text != "" {
			return text
		}
	}
	return locales.SetupErrorGeneric()
}
