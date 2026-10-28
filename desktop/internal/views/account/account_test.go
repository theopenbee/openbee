package account_test

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	wideW, wideH = 1440, 900
	username     = "admin"
	password     = "secret"
	fresh        = "secret2"
)

type fixture struct {
	*testkit.Harness
	t *testing.T
}

func signedIn(t *testing.T) *fixture {
	t.Helper()
	return &fixture{Harness: testkit.SignedIn(t, wideW, wideH, nil), t: t}
}

func (f *fixture) click(name string) {
	f.t.Helper()
	if err := f.T.Click(name); err != nil {
		f.t.Fatalf("click %q: %v (texts %q)", name, err, f.T.Texts())
	}
	f.Settle()
}

func (f *fixture) openMenu() {
	f.t.Helper()
	f.click(locales.ArcUserMenuTrigger(username))
	if !f.T.HasText(locales.AccountRole()) {
		f.t.Fatalf("menu did not open; texts %q", f.T.Texts())
	}
}

func (f *fixture) openPasswordDialog() {
	f.t.Helper()
	f.openMenu()
	f.click(locales.AccountChangePassword())
	if !f.T.HasText(locales.AccountChangePasswordDescription()) {
		f.t.Fatalf("password dialog did not open; texts %q", f.T.Texts())
	}
}

func (f *fixture) fill(label, text string) {
	f.t.Helper()
	f.click(label)
	f.T.Command("selectAll")
	f.T.Type(text)
	f.Settle()
}

func (f *fixture) submit() {
	f.t.Helper()
	f.T.Key(0, ui.KeyEnter)
	f.Settle()
}

func (f *fixture) shows(text string) bool {
	return slices.Contains(f.T.Texts(), text)
}

func (f *fixture) wantShown(texts ...string) {
	f.t.Helper()
	for _, text := range texts {
		if !f.shows(text) {
			f.t.Fatalf("screen does not show %q; texts %q", text, f.T.Texts())
		}
	}
}

func (f *fixture) wantHidden(texts ...string) {
	f.t.Helper()
	for _, text := range texts {
		if f.shows(text) {
			f.t.Fatalf("screen still shows %q; texts %q", text, f.T.Texts())
		}
	}
}

func (f *fixture) wantErrorBelow(errText, field, next string) {
	f.t.Helper()
	got, ok := f.T.Find(errText)
	above, okAbove := f.T.Find(field)
	below, okBelow := f.T.Find(next)
	if !ok || !okAbove || !okBelow {
		f.t.Fatalf("missing %q, %q or %q; texts %q", errText, field, next, f.T.Texts())
	}
	if got.Y <= above.Y || got.Y >= below.Y {
		f.t.Fatalf("%q at y=%v is not between %q (y=%v) and %q (y=%v)", errText, got.Y, field, above.Y, next, below.Y)
	}
}

func TestMenuShowsUsername(t *testing.T) {
	f := signedIn(t)
	f.wantShown(username)
	f.openMenu()
	names := 0
	for _, text := range f.T.Texts() {
		if text == username {
			names++
		}
	}
	if names != 2 {
		t.Fatalf("username shown %d times, want 2 (trigger and header); texts %q", names, f.T.Texts())
	}
	f.wantShown(locales.AccountRole(), locales.AccountChangePassword(), locales.ArcUserMenuTheme(), locales.ArcUserMenuLanguage(),
		locales.ArcUserMenuNotifications(), locales.ArcUserMenuSignOut())
}

func TestThemeSwitch(t *testing.T) {
	f := signedIn(t)
	f.openMenu()
	f.click(locales.ArcUserMenuThemesLight())
	if got := f.Fix.Themes[len(f.Fix.Themes)-1]; got != "light" {
		t.Fatalf("last theme source = %q, want light", got)
	}
	if got := f.Fix.Prefs.Load().Theme; got != "light" {
		t.Fatalf("saved theme = %q, want light", got)
	}
	f.click(locales.ArcUserMenuThemesSystem())
	if got := f.Fix.Themes[len(f.Fix.Themes)-1]; got != "system" {
		t.Fatalf("last theme source = %q, want system", got)
	}
}

func TestLanguageSwitch(t *testing.T) {
	f := signedIn(t)
	f.openMenu()
	f.click("中文")
	if got := f.Fix.Prefs.Load().Language; got != "zh" {
		t.Fatalf("saved language = %q, want zh", got)
	}
	f.wantShown(locales.In("zh").ArcUserMenuSignOut(), locales.In("zh").AccountRole())
	f.wantHidden(locales.In("en").ArcUserMenuSignOut())
	f.click("EN")
	f.wantShown(locales.In("en").ArcUserMenuSignOut())
}

func TestNotificationsDisabledWhenUnsupported(t *testing.T) {
	f := signedIn(t)
	f.Fix.Notifier.Unsupported = true
	f.Settle()
	f.openMenu()
	f.wantShown(locales.NotificationsUnavailableDesktop())
	_ = f.T.Click(locales.ArcUserMenuTogglesOn())
	f.Settle()
	if got := f.Fix.Prefs.Load().Notifications; got != "" {
		t.Fatalf("saved notifications = %q, want unset", got)
	}
}

func TestNotificationsDisabledWhenDenied(t *testing.T) {
	f := signedIn(t)
	f.Store.Prefs.NotificationsDenied = true
	f.Settle()
	f.openMenu()
	f.wantShown(locales.NotificationsUnavailableDenied())
	f.wantHidden(locales.NotificationsUnavailableDesktop())
	_ = f.T.Click(locales.ArcUserMenuTogglesOn())
	f.Settle()
	if got := f.Fix.Prefs.Load().Notifications; got != "" {
		t.Fatalf("saved notifications = %q, want unset", got)
	}
}

func TestNotificationsToggle(t *testing.T) {
	f := signedIn(t)
	f.openMenu()
	f.wantHidden(locales.NotificationsUnavailableDesktop(), locales.NotificationsUnavailableDenied())
	f.click(locales.ArcUserMenuTogglesOn())
	if got := f.Fix.Prefs.Load().Notifications; got != "on" {
		t.Fatalf("saved notifications = %q, want on", got)
	}
	if len(f.Fix.Notifier.Shown) != 1 {
		t.Fatalf("notifications shown = %d, want the confirmation", len(f.Fix.Notifier.Shown))
	}
	f.click(locales.ArcUserMenuTogglesOff())
	if got := f.Fix.Prefs.Load().Notifications; got != "off" {
		t.Fatalf("saved notifications = %q, want off", got)
	}
}

func TestSignOut(t *testing.T) {
	f := signedIn(t)
	f.openMenu()
	f.click(locales.ArcUserMenuSignOut())
	if f.Store.Phase != store.PhaseLogin {
		t.Fatalf("Phase = %v, want PhaseLogin", f.Store.Phase)
	}
	f.wantShown(locales.LoginTitle())
}

func TestKeyboardOpensMenuAndTabCloses(t *testing.T) {
	f := signedIn(t)
	f.openMenu()
	f.T.Key(0, ui.KeyEscape)
	f.Settle()
	f.wantHidden(locales.AccountRole())
	if !f.T.Focused(locales.ArcUserMenuTrigger(username)) {
		t.Fatalf("trigger lost the focus when Escape closed the menu; texts %q", f.T.Texts())
	}
	f.T.Key(0, ui.KeyDown)
	f.Settle()
	f.wantShown(locales.AccountRole())
	if !f.T.Focused(locales.AccountChangePassword()) {
		t.Fatalf("ArrowDown did not focus the first item; texts %q", f.T.Texts())
	}
	f.T.Key(0, ui.KeyTab)
	f.Settle()
	f.wantHidden(locales.AccountRole())
	if !f.T.Focused(locales.ArcUserMenuTrigger(username)) {
		t.Fatalf("trigger lost the focus when Tab closed the menu; texts %q", f.T.Texts())
	}
	f.T.Key(0, ui.KeyUp)
	f.Settle()
	if !f.T.Focused(locales.ArcUserMenuSignOut()) {
		t.Fatalf("ArrowUp did not focus the last item; texts %q", f.T.Texts())
	}
}

func TestPasswordValidation(t *testing.T) {
	f := signedIn(t)
	f.openPasswordDialog()
	if !f.T.Focused(locales.AccountCurrentPassword()) {
		t.Fatalf("current password is not focused on open; texts %q", f.T.Texts())
	}
	f.submit()
	f.wantShown(locales.AccountCurrentRequired(), locales.AccountErrorTooShort())
	f.wantHidden(locales.AccountErrorMismatch(), locales.AccountNewPasswordHint())
	f.wantErrorBelow(locales.AccountCurrentRequired(), locales.AccountCurrentPassword(), locales.AccountNewPassword())
	f.wantErrorBelow(locales.AccountErrorTooShort(), locales.AccountNewPassword(), locales.AccountConfirmPassword())
	if !f.T.Focused(locales.AccountCurrentPassword()) {
		t.Fatalf("first invalid field is not focused; texts %q", f.T.Texts())
	}

	f.fill(locales.AccountCurrentPassword(), password)
	f.wantHidden(locales.AccountCurrentRequired())
	f.fill(locales.AccountNewPassword(), "12345")
	f.wantHidden(locales.AccountErrorTooShort())
	f.wantShown(locales.AccountNewPasswordHint())
	f.submit()
	f.wantShown(locales.AccountErrorTooShort())
	f.wantHidden(locales.AccountCurrentRequired(), locales.AccountErrorSameAsOld(), locales.AccountErrorMismatch())
	if !f.T.Focused(locales.AccountNewPassword()) {
		t.Fatalf("new password is not focused after its error; texts %q", f.T.Texts())
	}

	f.fill(locales.AccountNewPassword(), password)
	f.submit()
	f.wantShown(locales.AccountErrorSameAsOld())
	f.wantHidden(locales.AccountErrorTooShort(), locales.AccountErrorMismatch())

	f.fill(locales.AccountNewPassword(), fresh)
	f.fill(locales.AccountConfirmPassword(), "secret3")
	f.submit()
	f.wantShown(locales.AccountErrorMismatch())
	f.wantHidden(locales.AccountErrorSameAsOld(), locales.AccountErrorTooShort())
	f.wantErrorBelow(locales.AccountErrorMismatch(), locales.AccountConfirmPassword(), locales.CommonCancel())
	if calls := f.Fake.Calls("POST", "/api/me/password"); len(calls) != 0 {
		t.Fatalf("validation sent %d requests, want none", len(calls))
	}
}

func TestPasswordWrongCurrent(t *testing.T) {
	f := signedIn(t)
	f.openPasswordDialog()
	f.fill(locales.AccountCurrentPassword(), "wrong")
	f.fill(locales.AccountNewPassword(), fresh)
	f.fill(locales.AccountConfirmPassword(), fresh)
	f.submit()
	want := locales.ErrorsOldPasswordIncorrect()
	f.wantShown(want)
	f.wantHidden(locales.AccountErrorTitle(), locales.AccountPasswordChanged())
	f.wantErrorBelow(want, locales.AccountCurrentPassword(), locales.AccountNewPassword())
	if !f.T.Focused(locales.AccountCurrentPassword()) {
		t.Fatalf("current password is not focused after the server error; texts %q", f.T.Texts())
	}
	if f.Store.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v, want PhaseReady", f.Store.Phase)
	}
}

func fillValid(f *fixture) {
	f.fill(locales.AccountCurrentPassword(), password)
	f.fill(locales.AccountNewPassword(), fresh)
	f.fill(locales.AccountConfirmPassword(), fresh)
	f.submit()
}

func TestPasswordSuccessThenCloseSignsOut(t *testing.T) {
	f := signedIn(t)
	f.openPasswordDialog()
	fillValid(f)
	f.wantShown(locales.AccountPasswordChanged(), locales.AccountPasswordChangedRelogin())
	f.wantHidden(locales.AccountCurrentPassword())
	if f.Store.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v before the dialog closes, want PhaseReady", f.Store.Phase)
	}
	f.click(locales.AccountSignInAgain())
	if f.Store.Phase != store.PhaseLogin {
		t.Fatalf("Phase = %v after closing, want PhaseLogin", f.Store.Phase)
	}
	f.Settle()
	f.wantShown(locales.LoginTitle())
	f.wantHidden(locales.AccountPasswordChanged())
}

func TestPasswordSuccessEscapeSignsOut(t *testing.T) {
	f := signedIn(t)
	f.openPasswordDialog()
	fillValid(f)
	f.wantShown(locales.AccountPasswordChanged())
	f.T.Key(0, ui.KeyEscape)
	f.Settle()
	if f.Store.Phase != store.PhaseLogin {
		t.Fatalf("Phase = %v after Escape, want PhaseLogin", f.Store.Phase)
	}
}

func TestPasswordCancelKeepsSession(t *testing.T) {
	f := signedIn(t)
	f.openPasswordDialog()
	f.fill(locales.AccountCurrentPassword(), password)
	f.click(locales.CommonCancel())
	f.wantHidden(locales.AccountChangePasswordDescription())
	if f.Store.Phase != store.PhaseReady {
		t.Fatalf("Phase = %v, want PhaseReady", f.Store.Phase)
	}
	f.openPasswordDialog()
	f.submit()
	f.wantShown(locales.AccountCurrentRequired())
}

func TestSignOutClosesPasswordDialog(t *testing.T) {
	f := signedIn(t)
	f.openPasswordDialog()
	f.Store.Logout()
	f.Settle()
	f.wantHidden(locales.AccountChangePasswordDescription())
	f.wantShown(locales.LoginTitle())
}
