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
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	fieldUsername = "username"
	fieldPassword = "password"
	serverRowGap  = 4
	serverSize    = 12
)

type loginForm struct {
	username, password string
	shown              bool
	usernameMissing    bool
	passwordMissing    bool
	failure            error
	loading            bool
	focus              string
	selectPassword     bool
}

type loginKey struct{}

func (a *Screens) Login(c *ui.Context, tk theme.Tokens, page ui.Element) {
	f := ui.Local(page, loginKey{}, func() loginForm {
		form := loginForm{username: a.s.Prefs.Username, focus: fieldUsername}
		if form.username != "" {
			form.focus = fieldPassword
		}
		return form
	})
	submit := func() { a.login(f) }
	a.layout(c, tk, locales.LoginTitle(), locales.LoginDescription(), func() {
		ui.Column(c).Gap(formGap).Children(func() {
			if f.failure != nil {
				kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.LoginErrorTitle(), Body: loginError(f.failure)})
			}
			kit.Field(c, tk, locales.LoginUsername(), "", missing(f.usernameMissing, locales.LoginUsernameRequired()), func() {
				in := kit.TextInput(c, tk, &f.username, locales.LoginUsername(), "").
					OnChange(func() { f.usernameMissing = false }).OnSubmit(submit)
				focusOnce(in, &f.focus, fieldUsername)
			})
			kit.Field(c, tk, locales.LoginPassword(), "", missing(f.passwordMissing, locales.LoginPasswordRequired()), func() {
				in := kit.PasswordInput(c, tk, &f.password, &f.shown, locales.LoginPassword(), "").
					OnChange(func() { f.passwordMissing = false }).OnSubmit(submit)
				if f.selectPassword {
					f.selectPassword = false
					in.SetTextSelection(0, utf8.RuneCountInString(f.password))
				}
				focusOnce(in, &f.focus, fieldPassword)
			})
			kit.Button(c, tk, locales.LoginSubmit(), kit.ButtonOpts{Loading: f.loading}).
				AlignSelf(ui.Stretch).Margin(submitGap, 0, 0, 0).OnClick(submit)
			a.serverRow(c, tk)
		})
	})
}

func (a *Screens) login(f *loginForm) {
	if f.loading {
		return
	}
	f.usernameMissing = strings.TrimSpace(f.username) == ""
	f.passwordMissing = f.password == ""
	switch {
	case f.usernameMissing:
		f.focus = fieldUsername
		return
	case f.passwordMissing:
		f.focus = fieldPassword
		return
	}
	f.failure = nil
	f.loading = true
	a.s.Login(f.username, f.password, func(err error) {
		f.loading = false
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		f.failure = err
		if api.IsStatus(err, http.StatusUnauthorized) {
			f.focus = fieldPassword
			f.selectPassword = true
		}
	})
}

func (a *Screens) serverRow(c *ui.Context, tk theme.Tokens) {
	ui.Row(c).Justify(ui.Center).AlignItems(ui.Center).Gap(serverRowGap).Wrap().Children(func() {
		ui.Text(c, locales.LoginServer(a.s.Server)).FontSize(serverSize).TextColor(tk.TextSecondary).MinWidth(0)
		kit.Button(c, tk, locales.LoginChangeServer(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).
			OnClick(a.s.ChangeServer)
	})
}

func loginError(err error) string {
	switch {
	case api.IsStatus(err, http.StatusUnauthorized):
		return locales.LoginError401()
	case api.IsStatus(err, http.StatusTooManyRequests):
		return locales.LoginError429()
	}
	return locales.LoginErrorGeneric()
}

func missing(on bool, text string) string {
	if on {
		return text
	}
	return ""
}

func focusOnce(in ui.Element, want *string, name string) {
	if *want == name {
		*want = ""
		in.Focus()
	}
}
