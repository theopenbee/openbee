package storetest

import (
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/internal/store"
)

const settleTimeout = 3 * time.Second

type FakeNotifier struct {
	Unsupported bool
	Visible     bool
	ShowErr     error
	Shown       []store.Notification
	Closed      []string
	Unread      int
}

func (n *FakeNotifier) Supported() bool { return !n.Unsupported }

func (n *FakeNotifier) Show(x store.Notification) error {
	if n.ShowErr != nil {
		return n.ShowErr
	}
	n.Shown = append(n.Shown, x)
	return nil
}

func (n *FakeNotifier) CloseConversation(cid string) { n.Closed = append(n.Closed, cid) }

func (n *FakeNotifier) SetUnread(total int) { n.Unread = total }

func (n *FakeNotifier) Foreground() bool { return n.Visible }

type Fixture struct {
	Notifier *FakeNotifier
	Prefs    prefs.Store
	Tokens   prefs.TokenStore
	Locales  []string
	Themes   []string

	store *store.Store
	queue *queue
}

type queue struct {
	mu    sync.Mutex
	items []func()
	wake  chan struct{}
}

func (q *queue) push(fn func()) {
	q.mu.Lock()
	q.items = append(q.items, fn)
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *queue) take() []func() {
	q.mu.Lock()
	defer q.mu.Unlock()
	items := q.items
	q.items = nil
	return items
}

func New(t testing.TB, seed func(st *fakeapi.State)) (*store.Store, *fakeapi.Server, *Fixture) {
	t.Helper()
	fake := fakeapi.New(t)
	if seed != nil {
		fake.Update(seed)
	}
	fix := &Fixture{
		Notifier: &FakeNotifier{},
		Prefs:    prefs.NewMemoryStore(prefs.Prefs{}),
		Tokens:   prefs.NewMemoryTokenStore(),
		queue:    &queue{wake: make(chan struct{}, 1)},
	}
	fix.store = store.New(store.Options{
		Post:           fix.queue.push,
		Prefs:          fix.Prefs,
		Tokens:         fix.Tokens,
		Notifier:       fix.Notifier,
		CacheDir:       t.TempDir(),
		SetLocale:      func(lang string) { fix.Locales = append(fix.Locales, lang) },
		SetThemeSource: func(pref string) { fix.Themes = append(fix.Themes, pref) },
	})
	return fix.store, fake, fix
}

func (f *Fixture) Settle(t testing.TB) {
	t.Helper()
	deadline := time.Now().Add(settleTimeout)
	for {
		idle := f.store.Busy() == 0
		items := f.queue.take()
		for _, fn := range items {
			fn()
		}
		if len(items) > 0 {
			continue
		}
		if idle {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("store did not settle within %v: %d tasks still running", settleTimeout, f.store.Busy())
		}
		select {
		case <-f.queue.wake:
		case <-time.After(5 * time.Millisecond):
		}
	}
}
