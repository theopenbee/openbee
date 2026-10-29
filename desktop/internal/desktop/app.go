package desktop

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views"
)

const (
	prefsFile    = "prefs.json"
	windowState  = "main"
	windowWidth  = 1280
	windowHeight = 800
	minWidth     = 720
	minHeight    = 480
	flushTimeout = 2 * time.Second
)

type App struct {
	debug       bool
	prefs       prefs.Store
	tokens      prefs.TokenStore
	cache       string
	theme       mygo.ThemeSource
	notifier    *Notifier
	tray        *Tray
	st          *store.Store
	root        *views.Root
	win         atomic.Pointer[mygo.Window]
	postMu      sync.Mutex
	queued      []func()
	pending     string
	quitting    bool
	windowBadge string
}

func New() *App {
	a := &App{debug: slices.ContainsFunc(os.Args[1:], isDebugFlag)}
	a.notifier = NewNotifier(a.window, a.open, a.showUnread)
	return a
}

func (a *App) Run() error {
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return fmt.Errorf("user data directory: %w", err)
	}
	cache, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return fmt.Errorf("cache directory: %w", err)
	}
	a.prefs = prefs.NewFileStore(filepath.Join(data, prefsFile))
	a.tokens = prefs.NewFileTokenStore(data)
	a.cache = cache
	a.theme = themeSource(a.prefs.Load().Theme)
	mygo.Theme.SetSource(a.theme)
	mygo.App.OnNotificationClick(a.notifier.Click)
	mygo.App.OnSecondInstance(func([]string, string) { a.show() })
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			a.show()
		}
	})
	mygo.App.OnWindowAllClosed(func() {
		if runtime.GOOS != darwin {
			mygo.App.Quit()
		}
	})
	mygo.Power.OnResume(func() { a.post(a.reconnect) })
	mygo.Network.OnChanged(func(status mygo.NetworkStatus) {
		if status.Online {
			a.post(a.reconnect)
		}
	})
	mygo.App.OnDidBecomeActive(a.syncForeground)
	mygo.App.OnDidResignActive(a.syncForeground)
	mygo.App.OnLocaleChanged(func(string) {
		a.setMenu()
		if a.tray != nil {
			a.tray.Relabel()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.flush() })
	mygo.App.WhenReady(a.start)
	return mygo.App.Run()
}

func (a *App) start() {
	a.st = store.New(store.Options{
		Post:           a.post,
		Prefs:          a.prefs,
		Tokens:         a.tokens,
		Notifier:       a.notifier,
		CacheDir:       a.cache,
		SetLocale:      l10n.SetLang,
		SetThemeSource: a.setTheme,
	})
	a.root = views.NewRoot(a.st)
	a.setMenu()
	a.startTray()
	a.openWindow()
	a.st.Start()
}

func (a *App) window() *mygo.Window {
	return a.win.Load()
}

func (a *App) post(fn func()) {
	a.postMu.Lock()
	w := a.win.Load()
	if w == nil {
		a.queued = append(a.queued, fn)
		a.postMu.Unlock()
		return
	}
	a.postMu.Unlock()
	w.Update(fn)
}

func (a *App) attach(w *mygo.Window) {
	a.postMu.Lock()
	defer a.postMu.Unlock()
	a.win.Store(w)
	for _, fn := range a.queued {
		w.Update(fn)
	}
	a.queued = nil
}

func (a *App) openWindow() *mygo.Window {
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:           TitleWithUnread(a.notifier.unread),
		Width:           windowWidth,
		Height:          windowHeight,
		MinWidth:        minWidth,
		MinHeight:       minHeight,
		StateKey:        windowState,
		TitleBarStyle:   titleBarStyle(runtime.GOOS),
		BackgroundColor: background(a.theme),
		Content:         ui.View(a.view),
	})
	w.OnFocus(a.syncForeground)
	w.OnBlur(a.syncForeground)
	w.OnClose(func(e *mygo.CloseEvent) {
		if closesToTray(runtime.GOOS, a.tray != nil, a.quitting) {
			e.PreventDefault()
			w.Hide()
			a.syncForeground()
		}
	})
	w.OnClosed(a.syncForeground)
	a.attach(w)
	a.windowBadge = ""
	a.setWindowIcon(w, a.notifier.unread)
	a.syncForeground()
	return w
}

func (a *App) startTray() {
	if !showsTray(runtime.GOOS) {
		return
	}
	t, err := NewTray(runtime.GOOS, a.show)
	if err != nil {
		log.Printf("desktop: tray: %v", err)
		return
	}
	a.tray = t
	t.SetUnread(a.notifier.unread)
}

func (a *App) showUnread(n int) {
	if a.tray != nil {
		a.tray.SetUnread(n)
	}
	if w := a.win.Load(); w != nil {
		a.setWindowIcon(w, n)
	}
}

func (a *App) setWindowIcon(w *mygo.Window, unread int) {
	if runtime.GOOS != windows {
		return
	}
	badge := model.IconBadgeText(unread)
	if badge == a.windowBadge {
		return
	}
	a.windowBadge = badge
	if err := w.SetIcon(windowIcon(unread)); err != nil {
		log.Printf("desktop: window icon: %v", err)
	}
}

func (a *App) view(c *ui.Context) {
	if a.pending != "" && a.st.Phase != store.PhaseBoot {
		if a.st.Phase == store.PhaseReady {
			a.root.OpenConversation(a.pending)
		}
		a.pending = ""
	}
	a.root.View(c)
}

func (a *App) show() {
	w := a.win.Load()
	if w == nil {
		return
	}
	if w.IsDestroyed() {
		w = a.openWindow()
	}
	if w.IsMinimized() {
		w.Restore()
	}
	if !w.IsVisible() {
		w.Show()
	}
	w.Focus()
	mygo.App.Focus()
}

func (a *App) open(cid string) {
	a.show()
	if cid == "" {
		return
	}
	a.pending = cid
	if w := a.win.Load(); w != nil {
		w.Invalidate()
	}
}

func (a *App) act(fn func()) {
	if a.st == nil {
		return
	}
	a.show()
	a.post(fn)
}

func (a *App) setMenu() {
	actions := Actions{
		Settings:     func() { a.act(func() { a.root.OpenSettings("") }) },
		ChangeServer: func() { a.act(func() { a.st.ChangeServer() }) },
		NewWorker:    func() { a.act(func() { a.root.NewWorker() }) },
		Inspector:    a.debug,
	}
	mygo.App.SetMenu(mygo.NewMenu(Menu(actions, runtime.GOOS)))
}

func (a *App) syncForeground() {
	if a.st == nil {
		return
	}
	if fg := a.notifier.Foreground(); fg != a.st.Foreground() {
		a.st.SetForeground(fg)
	}
}

func (a *App) reconnect() {
	if a.st != nil {
		a.st.Reconnect()
	}
}

func (a *App) setTheme(pref string) {
	a.theme = themeSource(pref)
	mygo.Theme.SetSource(a.theme)
	if w := a.win.Load(); w != nil {
		if err := w.SetBackgroundColor(background(a.theme)); err != nil {
			log.Printf("desktop: window background: %v", err)
		}
	}
}

func (a *App) flush() {
	if a.st != nil && !a.st.Flush(flushTimeout) {
		log.Printf("desktop: preferences and tokens not saved before quit")
	}
}

func titleBarStyle(goos string) mygo.TitleBarStyle {
	if goos == darwin {
		return mygo.TitleBarHidden
	}
	return mygo.TitleBarDefault
}

func themeSource(pref string) mygo.ThemeSource {
	switch s := mygo.ThemeSource(pref); s {
	case mygo.ThemeLight, mygo.ThemeSystem:
		return s
	}
	return mygo.ThemeDark
}

func background(source mygo.ThemeSource) string {
	light, dark := cssColor(theme.Light().Background), cssColor(theme.Dark().Background)
	switch source {
	case mygo.ThemeLight:
		return light
	case mygo.ThemeSystem:
		return "light-dark(" + light + ", " + dark + ")"
	}
	return dark
}

func cssColor(c ui.Color) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func isDebugFlag(arg string) bool {
	return arg == "-debug" || arg == "--debug"
}
