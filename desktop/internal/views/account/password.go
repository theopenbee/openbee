package account

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	minPasswordLength = 6
	formGap           = 16
	errorWrongCurrent = "old_password_incorrect"
	fieldCurrent      = "current"
	fieldNext         = "next"
	fieldConfirm      = "confirm"
)

type passwordForm struct {
	open       bool
	shown      bool
	attempt    int
	current    string
	next       string
	confirm    string
	reveal     [3]bool
	currentErr string
	nextErr    string
	confirmErr string
	failure    error
	loading    bool
	done       bool
	focus      string
	selectOld  bool
}

func (p *passwordForm) show() {
	attempt := p.attempt
	*p = passwordForm{open: true, attempt: attempt, focus: fieldCurrent}
}

func (p *passwordForm) reset() {
	attempt := p.attempt + 1
	*p = passwordForm{attempt: attempt}
}

func (a *Account) Dialogs(c *ui.Context) {
	tk := theme.Apply(c)
	p := &a.password
	kit.Dialog(c, tk, &p.open, kit.DialogOpts{
		Title:       locales.AccountChangePassword(),
		Description: locales.AccountChangePasswordDescription(),
		Locked:      p.loading,
		Footer:      func() { a.footer(c, tk) },
	}, func() { a.body(c, tk) })
	if p.shown && !p.open {
		finished := p.done
		p.reset()
		if finished {
			a.s.Logout()
			c.Invalidate()
		}
	}
	p.shown = p.open
}

func (a *Account) footer(c *ui.Context, tk theme.Tokens) {
	p := &a.password
	if p.done {
		kit.Button(c, tk, locales.AccountSignInAgain(), kit.ButtonOpts{Size: kit.SizeMD}).AutoFocus().
			OnClick(func() { p.open = false })
		return
	}
	kit.Button(c, tk, locales.CommonCancel(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Disabled: p.loading}).
		OnClick(func() { p.open = false })
	kit.Button(c, tk, locales.AccountChangePassword(), kit.ButtonOpts{Size: kit.SizeMD, Loading: p.loading}).
		OnClick(a.submit)
}

func (a *Account) body(c *ui.Context, tk theme.Tokens) {
	p := &a.password
	if p.done {
		kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneSuccess, Title: locales.AccountPasswordChanged(), Body: locales.AccountPasswordChangedRelogin()})
		return
	}
	ui.Column(c).Gap(formGap).Disabled(p.loading).Children(func() {
		if p.failure != nil {
			kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.AccountErrorTitle(), Body: l10n.ErrorText(p.failure)})
		}
		kit.Field(c, tk, locales.AccountCurrentPassword(), "", p.currentErr, func() {
			in := kit.PasswordInput(c, tk, &p.current, &p.reveal[0], locales.AccountCurrentPassword(), "").
				OnChange(func() { p.currentErr = "" }).OnSubmit(a.submit)
			if p.selectOld {
				p.selectOld = false
				in.SetTextSelection(0, utf8.RuneCountInString(p.current))
			}
			focusOnce(in, &p.focus, fieldCurrent)
		})
		hint := locales.AccountNewPasswordHint()
		if p.nextErr != "" {
			hint = ""
		}
		kit.Field(c, tk, locales.AccountNewPassword(), hint, p.nextErr, func() {
			in := kit.PasswordInput(c, tk, &p.next, &p.reveal[1], locales.AccountNewPassword(), "").
				OnChange(func() { p.nextErr = "" }).OnSubmit(a.submit)
			focusOnce(in, &p.focus, fieldNext)
		})
		kit.Field(c, tk, locales.AccountConfirmPassword(), "", p.confirmErr, func() {
			in := kit.PasswordInput(c, tk, &p.confirm, &p.reveal[2], locales.AccountConfirmPassword(), "").
				OnChange(func() { p.confirmErr = "" }).OnSubmit(a.submit)
			focusOnce(in, &p.focus, fieldConfirm)
		})
	})
}

func (a *Account) submit() {
	p := &a.password
	if p.loading || p.done || !p.open {
		return
	}
	p.currentErr, p.nextErr, p.confirmErr = validate(p.current, p.next, p.confirm)
	p.failure = nil
	switch {
	case p.currentErr != "":
		p.focus = fieldCurrent
		return
	case p.nextErr != "":
		p.focus = fieldNext
		return
	case p.confirmErr != "":
		p.focus = fieldConfirm
		return
	}
	p.loading = true
	attempt := p.attempt
	a.s.ChangePassword(p.current, p.next, func(err error) {
		if p.attempt != attempt {
			return
		}
		p.loading = false
		a.changed(err)
	})
}

func (a *Account) changed(err error) {
	p := &a.password
	var apiErr *api.Error
	switch {
	case err == nil:
		p.done = true
	case errors.Is(err, context.Canceled):
	case errors.As(err, &apiErr) && apiErr.Code == errorWrongCurrent:
		p.currentErr = l10n.ErrorText(err)
		p.focus = fieldCurrent
		p.selectOld = true
	default:
		p.failure = err
	}
}

func validate(current, next, confirm string) (currentErr, nextErr, confirmErr string) {
	if current == "" {
		currentErr = locales.AccountCurrentRequired()
	}
	switch {
	case utf8.RuneCountInString(next) < minPasswordLength:
		nextErr = locales.AccountErrorTooShort()
	case next == current:
		nextErr = locales.AccountErrorSameAsOld()
	}
	if nextErr == "" && confirm != next {
		confirmErr = locales.AccountErrorMismatch()
	}
	return
}

func focusOnce(in ui.Element, want *string, name string) {
	if *want == name {
		*want = ""
		in.Focus()
	}
}
