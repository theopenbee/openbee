package testkit

import (
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	ScreensEnv   = "OPENBEE_SCREENS_DIR"
	settleRounds = 20
	shotScale    = 2
	shotSettle   = time.Second
	motionSettle = time.Second
	englishTag   = "en"
	chineseTag   = "zh-Hans"
)

var (
	localeOnce sync.Once
	locale     atomic.Value
)

type Harness struct {
	Store *store.Store
	Fake  *fakeapi.Server
	Root  *views.Root
	T     *ui.Tester
	Fix   *storetest.Fixture

	t      testing.TB
	shown  string
	wanted string
}

func New(t testing.TB, w, h int, seed func(st *fakeapi.State)) *Harness {
	t.Helper()
	prepare(t)
	st, fake, fix := storetest.New(t, seed)
	return mount(t, st, fake, fix, w, h)
}

func Live(t testing.TB, url, user, pass string, w, h int) *Harness {
	t.Helper()
	prepare(t)
	st, fix := storetest.Live(t)
	fix.Notifier.Unsupported = true
	hs := mount(t, st, nil, fix, w, h)
	t.Cleanup(st.Logout)
	hs.Fill(locales.ConnectServerLabel(), url)
	hs.Click(locales.ConnectSubmit())
	if st.Phase != store.PhaseLogin {
		t.Fatalf("connect to %s: phase=%v; texts %q", url, st.Phase, hs.T.Texts())
	}
	hs.SignIn(user, pass)
	if st.Phase != store.PhaseReady {
		t.Fatalf("sign in to %s as %s: phase=%v; texts %q", url, user, st.Phase, hs.T.Texts())
	}
	return hs
}

func prepare(t testing.TB) {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatalf("register fonts: %v", err)
	}
	followLocale()
	setLocale("")
	t.Cleanup(func() { setLocale("") })
}

func mount(t testing.TB, st *store.Store, fake *fakeapi.Server, fix *storetest.Fixture, w, h int) *Harness {
	t.Helper()
	hs := &Harness{Store: st, Fake: fake, Fix: fix, t: t}
	fix.OnLocale = hs.setLanguage
	hs.Root = views.NewRoot(st)
	st.Start()
	hs.T = ui.NewTester(hs.Root.View, w, h)
	hs.Settle()
	return hs
}

func SignedIn(t testing.TB, w, h int, seed func(st *fakeapi.State)) *Harness {
	t.Helper()
	hs := New(t, w, h, seed)
	hs.Connect()
	snap := hs.Fake.Snapshot()
	var loginErr error
	hs.Store.Login(snap.Username, snap.Password, func(err error) { loginErr = err })
	hs.Settle()
	if loginErr != nil || hs.Store.Phase != store.PhaseReady {
		t.Fatalf("sign in: err=%v phase=%v", loginErr, hs.Store.Phase)
	}
	return hs
}

func (h *Harness) Connect() {
	h.t.Helper()
	var connectErr error
	h.Store.Connect(h.Fake.URL, func(err error) { connectErr = err })
	h.Settle()
	if connectErr != nil {
		h.t.Fatalf("connect to the fake server: %v", connectErr)
	}
}

func (h *Harness) SignIn(user, pass string) {
	h.t.Helper()
	h.Fill(locales.LoginUsername(), user)
	h.Fill(locales.LoginPassword(), pass)
	h.T.Key(0, ui.KeyEnter)
	h.Motion()
}

func (h *Harness) Click(name string) {
	h.t.Helper()
	if err := h.T.Click(name); err != nil {
		h.t.Fatalf("click %q: %v; texts %q", name, err, h.T.Texts())
	}
	h.Settle()
}

func (h *Harness) Fill(label, text string) {
	h.t.Helper()
	h.Click(label)
	h.T.Command("selectAll")
	h.T.Type(text)
	h.Settle()
}

func (h *Harness) Motion() {
	h.t.Helper()
	h.Settle()
	h.T.Advance(motionSettle)
	h.Settle()
}

func (h *Harness) Settle() {
	h.t.Helper()
	for range settleRounds {
		h.Fix.Settle(h.t)
		h.syncLocale()
		h.T.Frame()
		if h.Fix.Idle() {
			return
		}
	}
	h.t.Fatalf("view did not settle after %d rounds: %d store tasks running", settleRounds, h.Store.Busy())
}

func (h *Harness) Shot(name string) {
	h.t.Helper()
	dir := os.Getenv(ScreensEnv)
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		h.t.Fatal(err)
	}
	h.T.SetScale(shotScale)
	for _, mode := range []string{"light", "dark"} {
		h.T.SetDark(mode == "dark")
		h.Settle()
		h.T.Advance(shotSettle)
		h.T.Frame()
		h.writePNG(filepath.Join(dir, name+"-"+mode+".png"))
	}
	h.T.SetDark(false)
	h.Settle()
}

func (h *Harness) writePNG(path string) {
	h.t.Helper()
	f, err := os.Create(path)
	if err != nil {
		h.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, h.T.Image()); err != nil {
		h.t.Fatal(err)
	}
}

func (h *Harness) setLanguage(lang string) {
	tag := englishTag
	if lang == "zh" {
		tag = chineseTag
	}
	setLocale(tag)
	h.wanted = tag
}

func (h *Harness) syncLocale() {
	if h.wanted != h.shown {
		h.shown = h.wanted
		h.T.SetLocale(h.shown)
	}
}

func followLocale() {
	localeOnce.Do(func() {
		i18n.SetLocaleFunc(func() string {
			tag, _ := locale.Load().(string)
			return tag
		})
	})
}

func setLocale(tag string) {
	locale.Store(tag)
}
