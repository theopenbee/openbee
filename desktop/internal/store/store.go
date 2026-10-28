package store

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
)

type Notifier interface {
	Supported() bool
	Show(n Notification) error
	CloseConversation(cid string)
	SetUnread(total int)
	Foreground() bool
}

type Notification struct{ ID, Group, Title, Body, ConversationID string }

type Options struct {
	Post           func(func())
	HTTP           *http.Client
	Prefs          prefs.Store
	Tokens         prefs.TokenStore
	Notifier       Notifier
	Now            func() time.Time
	CacheDir       string
	SetLocale      func(lang string)
	SetThemeSource func(pref string)
}

type Phase int

const (
	PhaseBoot Phase = iota
	PhaseConnect
	PhaseSetup
	PhaseLogin
	PhaseReady
)

const (
	themeLight      = "light"
	themeDark       = "dark"
	themeSystem     = "system"
	languageEnglish = "en"
	languageChinese = "zh"
)

type Store struct {
	Phase  Phase
	Prefs  prefs.Prefs
	Server string
	Config *Resource[api.Config]
	Toasts []kit.Toast

	o      Options
	sess   *session
	resets []func()
	busy   atomic.Int64
	lang   atomic.Value
	writes writer
}

type writer struct {
	mu      sync.Mutex
	queue   []func() error
	running bool
}

type noNotifier struct{}

func (noNotifier) Supported() bool          { return false }
func (noNotifier) Show(Notification) error  { return nil }
func (noNotifier) CloseConversation(string) {}
func (noNotifier) SetUnread(int)            {}
func (noNotifier) Foreground() bool         { return true }

func New(o Options) *Store {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Notifier == nil {
		o.Notifier = noNotifier{}
	}
	if o.SetLocale == nil {
		o.SetLocale = func(string) {}
	}
	if o.SetThemeSource == nil {
		o.SetThemeSource = func(string) {}
	}
	s := &Store{o: o}
	s.Config = NewResource(s, func(ctx context.Context, c *api.Client) (api.Config, error) {
		return c.Config(ctx)
	})
	s.Config.settled = s.configSettled
	s.sess = s.open("", prefs.Tokens{}, 1)
	return s
}

func (s *Store) Go(fn func(ctx context.Context)) {
	ctx := s.sess.ctx
	s.busy.Add(1)
	go func() {
		defer s.busy.Add(-1)
		fn(ctx)
	}()
}

func (s *Store) Busy() int {
	return int(s.busy.Load())
}

func (s *Store) async(work func(ctx context.Context, c *api.Client) func(), canceled func()) {
	sess := s.sess
	s.Go(func(ctx context.Context) {
		apply := work(ctx, sess.client)
		s.o.Post(func() {
			switch {
			case s.sess == sess:
				apply()
			case canceled != nil:
				canceled()
			}
		})
	})
}

func (s *Store) persist(fn func() error) {
	w := &s.writes
	w.mu.Lock()
	defer w.mu.Unlock()
	w.queue = append(w.queue, fn)
	if w.running {
		return
	}
	w.running = true
	s.busy.Add(1)
	go s.flushWrites()
}

func (s *Store) flushWrites() {
	defer s.busy.Add(-1)
	w := &s.writes
	for {
		w.mu.Lock()
		if len(w.queue) == 0 {
			w.running = false
			w.mu.Unlock()
			return
		}
		fn := w.queue[0]
		w.queue = w.queue[1:]
		w.mu.Unlock()
		if err := fn(); err != nil {
			log.Printf("store: %v", err)
		}
	}
}

func (s *Store) savePrefs() {
	p := s.Prefs
	s.persist(func() error {
		return wrap("save preferences", s.o.Prefs.Save(p))
	})
}

func (s *Store) ThemePref() string {
	return normalizeTheme(s.Prefs.Theme)
}

func (s *Store) SetTheme(pref string) {
	pref = normalizeTheme(pref)
	s.Prefs.Theme = pref
	s.savePrefs()
	s.o.SetThemeSource(pref)
}

func (s *Store) SetLanguage(lang string) {
	lang = resolveLanguage(lang)
	s.Prefs.Language = lang
	s.savePrefs()
	s.applyLanguage(lang)
}

func (s *Store) resolveLanguage() {
	if lang, ok := normalizeLanguage(s.Prefs.Language); ok {
		s.applyLanguage(lang)
		return
	}
	if s.Server == "" {
		s.applyLanguage(languageEnglish)
		return
	}
	s.Config.Ensure()
}

func (s *Store) configSettled() {
	if _, ok := normalizeLanguage(s.Prefs.Language); ok {
		return
	}
	switch {
	case s.Config.Loaded:
		s.applyLanguage(resolveLanguage(s.Config.Data.Language))
	case s.language() == "":
		s.applyLanguage(languageEnglish)
	}
}

func (s *Store) applyLanguage(lang string) {
	if s.language() == lang {
		return
	}
	s.lang.Store(lang)
	s.o.SetLocale(lang)
}

func (s *Store) language() string {
	lang, _ := s.lang.Load().(string)
	return lang
}

func normalizeTheme(pref string) string {
	if pref == themeLight || pref == themeSystem {
		return pref
	}
	return themeDark
}

func normalizeLanguage(tag string) (string, bool) {
	base := strings.ToLower(tag)
	if i := strings.IndexAny(base, "-_"); i >= 0 {
		base = base[:i]
	}
	switch base {
	case languageEnglish, languageChinese:
		return base, true
	}
	return "", false
}

func resolveLanguage(tag string) string {
	if lang, ok := normalizeLanguage(tag); ok {
		return lang
	}
	return languageEnglish
}
