package storetest

import (
	"context"
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

type Clock struct {
	mu       sync.Mutex
	now      time.Time
	sleepers map[*sleeper]struct{}
}

type sleeper struct {
	until time.Time
	done  chan struct{}
}

func NewClock(start time.Time) *Clock {
	return &Clock{now: start, sleepers: map[*sleeper]struct{}{}}
}

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for sl := range c.sleepers {
		if !sl.until.After(c.now) {
			close(sl.done)
			delete(c.sleepers, sl)
		}
	}
}

func (c *Clock) Sleep(ctx context.Context, d time.Duration, wake <-chan struct{}) error {
	c.mu.Lock()
	if d <= 0 {
		c.mu.Unlock()
		return ctx.Err()
	}
	sl := &sleeper{until: c.now.Add(d), done: make(chan struct{})}
	c.sleepers[sl] = struct{}{}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.sleepers, sl)
		c.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wake:
		return nil
	case <-sl.done:
		return nil
	}
}

func (c *Clock) Sleepers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sleepers)
}

func (c *Clock) AwaitSleepers(t testing.TB, n int) {
	t.Helper()
	deadline := time.Now().Add(settleTimeout)
	for c.Sleepers() < n {
		if time.Now().After(deadline) {
			t.Fatalf("%d sleepers after %v, want %d", c.Sleepers(), settleTimeout, n)
		}
		time.Sleep(time.Millisecond)
	}
}

type Fixture struct {
	Clock    *Clock
	Notifier *FakeNotifier
	Prefs    prefs.Store
	Tokens   prefs.TokenStore
	Locales  []string
	Themes   []string
	OnLocale func(lang string)

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

func (q *queue) empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) == 0
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
		Clock:    NewClock(time.Now()),
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
		Now:            fix.Clock.Now,
		Sleep:          fix.Clock.Sleep,
		CacheDir:       t.TempDir(),
		SetLocale:      fix.setLocale,
		SetThemeSource: func(pref string) { fix.Themes = append(fix.Themes, pref) },
	})
	return fix.store, fake, fix
}

func (f *Fixture) setLocale(lang string) {
	f.Locales = append(f.Locales, lang)
	if f.OnLocale != nil {
		f.OnLocale(lang)
	}
}

func (f *Fixture) Idle() bool {
	return f.store.Busy() == 0 && f.queue.empty()
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

func (f *Fixture) Await(t testing.TB, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(settleTimeout)
	for {
		f.Settle(t)
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %v", settleTimeout)
		}
		select {
		case <-f.queue.wake:
		case <-time.After(5 * time.Millisecond):
		}
	}
}
