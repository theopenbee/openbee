package sse_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/sse"
)

func readAll(t *testing.T, input string) []string {
	t.Helper()
	var got []string
	err := sse.Read(strings.NewReader(input), func(e sse.Event) error {
		got = append(got, e.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return got
}

func TestReadDataLines(t *testing.T) {
	got := readAll(t, ": ok\n\ndata: {\"a\":1}\n\n: ping\n\ndata: x\n\n")
	want := []string{`{"a":1}`, "x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadMultiLineData(t *testing.T) {
	got := readAll(t, "data: one\ndata: two\ndata:three\n\n")
	want := []string{"one\ntwo\nthree"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadCRLF(t *testing.T) {
	got := readAll(t, ": ok\r\n\r\ndata: a\r\ndata: b\r\n\r\ndata: c\r\n\r\n")
	want := []string{"a\nb", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadIgnoresOtherFields(t *testing.T) {
	got := readAll(t, "event: custom\nid: 7\nretry: 100\nunknown\ndata: kept\n\nevent: only\n\n")
	want := []string{"kept"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadOnlyStripsOneLeadingSpace(t *testing.T) {
	got := readAll(t, "data:  two spaces\n\ndata:\n\n")
	want := []string{" two spaces", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadDropsUnterminatedFinalEvent(t *testing.T) {
	got := readAll(t, "data: kept\n\ndata: partial\n")
	want := []string{"kept"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadLongLine(t *testing.T) {
	payload := strings.Repeat("x", 512*1024)
	got := readAll(t, "data: "+payload+"\n\n")
	if len(got) != 1 || got[0] != payload {
		t.Fatalf("long line not delivered intact: %d events", len(got))
	}
}

func TestReadLineTooLong(t *testing.T) {
	payload := strings.Repeat("x", 2*1024*1024)
	err := sse.Read(strings.NewReader("data: "+payload+"\n\n"), func(sse.Event) error { return nil })
	if err == nil {
		t.Fatal("expected an error for a line over 1 MiB")
	}
}

func TestReadStopsOnCallbackError(t *testing.T) {
	stop := errors.New("stop")
	calls := 0
	err := sse.Read(strings.NewReader("data: a\n\ndata: b\n\n"), func(sse.Event) error {
		calls++
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestBackoffChat(t *testing.T) {
	b := sse.Backoff{Base: 2 * time.Second, Max: 30 * time.Second}
	cases := map[int]time.Duration{
		1: 2 * time.Second,
		2: 4 * time.Second,
		3: 8 * time.Second,
		4: 16 * time.Second,
		5: 30 * time.Second,
		9: 30 * time.Second,
	}
	for attempt, want := range cases {
		if got := b.Delay(attempt); got != want {
			t.Errorf("Delay(%d) = %v, want %v", attempt, got, want)
		}
	}
}

func TestBackoffSetup(t *testing.T) {
	b := sse.Backoff{Base: time.Second, Max: 10 * time.Second}
	cases := map[int]time.Duration{
		1: time.Second,
		4: 8 * time.Second,
		5: 10 * time.Second,
	}
	for attempt, want := range cases {
		if got := b.Delay(attempt); got != want {
			t.Errorf("Delay(%d) = %v, want %v", attempt, got, want)
		}
	}
}

func TestBackoffEdges(t *testing.T) {
	b := sse.Backoff{Base: 2 * time.Second, Max: 30 * time.Second}
	if got := b.Delay(0); got != 2*time.Second {
		t.Errorf("Delay(0) = %v", got)
	}
	if got := b.Delay(-3); got != 2*time.Second {
		t.Errorf("Delay(-3) = %v", got)
	}
	if got := b.Delay(1 << 30); got != 30*time.Second {
		t.Errorf("Delay(huge) = %v", got)
	}
	if got := (sse.Backoff{Base: 40 * time.Second, Max: 30 * time.Second}).Delay(1); got != 30*time.Second {
		t.Errorf("base above max = %v", got)
	}
	if got := (sse.Backoff{Base: time.Second}).Delay(4); got != 8*time.Second {
		t.Errorf("uncapped Delay(4) = %v", got)
	}
}

type fakeBody struct {
	io.Reader
	mu     sync.Mutex
	closed int
}

func (b *fakeBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed++
	return nil
}

func (b *fakeBody) closeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

type sleepCall struct {
	d       time.Duration
	pending int
}

type recorder struct {
	mu     sync.Mutex
	opens  int
	drops  []int
	errs   []error
	events []string
	sleeps []sleepCall
}

func (r *recorder) onDrop(err error, attempt int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drops = append(r.drops, attempt)
	r.errs = append(r.errs, err)
}

func (r *recorder) onEvent(e sse.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e.Data)
}

func (r *recorder) sleep(_ context.Context, d time.Duration, wake <-chan struct{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sleeps = append(r.sleeps, sleepCall{d: d, pending: len(wake)})
	return nil
}

var chatBackoff = sse.Backoff{Base: 2 * time.Second, Max: 30 * time.Second}

func TestRunReconnects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := &recorder{}
	body := newBlockingBody()
	opened := 0
	err := sse.Run(ctx, sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) {
			rec.opens++
			if rec.opens <= 2 {
				return nil, errors.New("connect failed")
			}
			return body, nil
		},
		OnOpen: func() {
			opened++
			cancel()
		},
		OnDrop:  rec.onDrop,
		Backoff: chatBackoff,
		Sleep:   rec.sleep,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	if !reflect.DeepEqual(rec.drops, []int{1, 2}) {
		t.Fatalf("drop attempts = %v, want [1 2]", rec.drops)
	}
	if opened != 1 {
		t.Fatalf("OnOpen calls = %d, want 1", opened)
	}
	wantSleeps := []time.Duration{2 * time.Second, 4 * time.Second}
	var gotSleeps []time.Duration
	for _, s := range rec.sleeps {
		gotSleeps = append(gotSleeps, s.d)
	}
	if !reflect.DeepEqual(gotSleeps, wantSleeps) {
		t.Fatalf("sleeps = %v, want %v", gotSleeps, wantSleeps)
	}
	if body.closeCount() == 0 {
		t.Fatal("body was not closed")
	}
}

func TestRunAttemptResetsAfterOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := &recorder{}
	closers := []*fakeBody{}
	step := 0
	err := sse.Run(ctx, sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) {
			step++
			switch step {
			case 1, 3, 4:
				return nil, errors.New("connect failed")
			case 2:
				b := &fakeBody{Reader: strings.NewReader(": ok\n\ndata: hello\n\n")}
				closers = append(closers, b)
				return b, nil
			default:
				cancel()
				return nil, ctx.Err()
			}
		},
		OnEvent: rec.onEvent,
		OnDrop:  rec.onDrop,
		Backoff: chatBackoff,
		Sleep:   rec.sleep,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v", err)
	}
	if want := []int{1, 1, 2, 3}; !reflect.DeepEqual(rec.drops, want) {
		t.Fatalf("drop attempts = %v, want %v", rec.drops, want)
	}
	if !errors.Is(rec.errs[1], io.EOF) {
		t.Fatalf("stream end error = %v, want io.EOF", rec.errs[1])
	}
	if !reflect.DeepEqual(rec.events, []string{"hello"}) {
		t.Fatalf("events = %q", rec.events)
	}
	if closers[0].closeCount() == 0 {
		t.Fatal("ended stream was not closed")
	}
	var gotSleeps []time.Duration
	for _, s := range rec.sleeps {
		gotSleeps = append(gotSleeps, s.d)
	}
	wantSleeps := []time.Duration{2 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	if !reflect.DeepEqual(gotSleeps, wantSleeps) {
		t.Fatalf("sleeps = %v, want %v", gotSleeps, wantSleeps)
	}
}

type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }

func TestRunReadErrorIsDrop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := &recorder{}
	boom := errors.New("connection reset")
	step := 0
	err := sse.Run(ctx, sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) {
			step++
			if step == 1 {
				return &fakeBody{Reader: failingReader{boom}}, nil
			}
			cancel()
			return nil, ctx.Err()
		},
		OnDrop:  rec.onDrop,
		Backoff: chatBackoff,
		Sleep:   rec.sleep,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v", err)
	}
	if len(rec.errs) != 1 || !errors.Is(rec.errs[0], boom) || rec.drops[0] != 1 {
		t.Fatalf("drops = %v errs = %v", rec.drops, rec.errs)
	}
}

func TestRunGivesUp(t *testing.T) {
	rec := &recorder{}
	connectErr := errors.New("connect failed")
	err := sse.Run(context.Background(), sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) {
			rec.opens++
			return nil, connectErr
		},
		OnDrop:      rec.onDrop,
		Backoff:     chatBackoff,
		MaxAttempts: 5,
		Sleep:       rec.sleep,
	})
	if !errors.Is(err, sse.ErrGaveUp) {
		t.Fatalf("Run = %v, want ErrGaveUp", err)
	}
	if rec.opens != 5 {
		t.Fatalf("opens = %d, want 5", rec.opens)
	}
	if want := []int{1, 2, 3, 4, 5}; !reflect.DeepEqual(rec.drops, want) {
		t.Fatalf("drop attempts = %v, want %v", rec.drops, want)
	}
	if len(rec.sleeps) != 4 {
		t.Fatalf("sleeps = %d, want 4 (none after the final drop)", len(rec.sleeps))
	}
}

func TestRunMaxAttemptsCountsConsecutiveFailures(t *testing.T) {
	rec := &recorder{}
	step := 0
	err := sse.Run(context.Background(), sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) {
			step++
			if step%3 == 0 {
				return &fakeBody{Reader: strings.NewReader("")}, nil
			}
			return nil, errors.New("connect failed")
		},
		OnDrop:      rec.onDrop,
		Backoff:     chatBackoff,
		MaxAttempts: 3,
		Sleep:       rec.sleep,
	})
	if !errors.Is(err, sse.ErrGaveUp) {
		t.Fatalf("Run = %v, want ErrGaveUp", err)
	}
	if want := []int{1, 2, 1, 2, 3}; !reflect.DeepEqual(rec.drops, want) {
		t.Fatalf("drop attempts = %v, want %v", rec.drops, want)
	}
}

func TestRunWakeSkipsSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wake := make(chan struct{})
	dropped := make(chan struct{}, 1)
	done := make(chan error, 1)
	step := 0
	go func() {
		done <- sse.Run(ctx, sse.Options{
			Open: func(context.Context) (io.ReadCloser, error) {
				step++
				if step == 1 {
					return nil, errors.New("connect failed")
				}
				cancel()
				return nil, ctx.Err()
			},
			OnDrop:  func(error, int) { dropped <- struct{}{} },
			Backoff: sse.Backoff{Base: time.Hour, Max: time.Hour},
			Wake:    wake,
		})
	}()
	select {
	case <-dropped:
	case <-time.After(5 * time.Second):
		t.Fatal("no drop observed")
	}
	select {
	case wake <- struct{}{}:
	case <-time.After(5 * time.Second):
		t.Fatal("sleep did not receive the wake signal")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after wake")
	}
}

func TestRunIgnoresWakeWhileConnected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wake := make(chan struct{}, 1)
	rec := &recorder{}
	step := 0
	err := sse.Run(ctx, sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) {
			step++
			if step == 1 {
				return &fakeBody{Reader: strings.NewReader("data: x\n\n")}, nil
			}
			cancel()
			return nil, ctx.Err()
		},
		OnEvent: func(sse.Event) { wake <- struct{}{} },
		OnDrop:  rec.onDrop,
		Backoff: chatBackoff,
		Wake:    wake,
		Sleep:   rec.sleep,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v", err)
	}
	if len(rec.sleeps) != 1 || rec.sleeps[0].pending != 0 {
		t.Fatalf("sleeps = %+v, want one sleep with no pending wake", rec.sleeps)
	}
}

type blockingBody struct {
	r      *io.PipeReader
	w      *io.PipeWriter
	mu     sync.Mutex
	closed int
}

func newBlockingBody() *blockingBody {
	r, w := io.Pipe()
	return &blockingBody{r: r, w: w}
}

func (b *blockingBody) Read(p []byte) (int, error) { return b.r.Read(p) }

func (b *blockingBody) Close() error {
	b.mu.Lock()
	b.closed++
	b.mu.Unlock()
	b.w.Close()
	return b.r.Close()
}

func (b *blockingBody) closeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

func TestRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := newBlockingBody()
	opened := make(chan struct{})
	done := make(chan error, 1)
	var drops int
	go func() {
		done <- sse.Run(ctx, sse.Options{
			Open:    func(context.Context) (io.ReadCloser, error) { return body, nil },
			OnOpen:  func() { close(opened) },
			OnDrop:  func(error, int) { drops++ },
			Backoff: chatBackoff,
		})
	}()
	select {
	case <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("stream never opened")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if body.closeCount() == 0 {
		t.Fatal("body was not closed on cancel")
	}
	if drops != 0 {
		t.Fatalf("OnDrop called %d times on cancel", drops)
	}
}

func TestRunStopsOnCancelDuringSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dropped := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- sse.Run(ctx, sse.Options{
			Open:    func(context.Context) (io.ReadCloser, error) { return nil, errors.New("connect failed") },
			OnDrop:  func(error, int) { dropped <- struct{}{} },
			Backoff: sse.Backoff{Base: time.Hour, Max: time.Hour},
		})
	}()
	select {
	case <-dropped:
	case <-time.After(5 * time.Second):
		t.Fatal("no drop observed")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel during sleep")
	}
}

func TestRunCancelledBeforeOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := sse.Run(ctx, sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) {
			called = true
			return nil, errors.New("unreachable")
		},
		Backoff: chatBackoff,
	})
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestRunNilCallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	step := 0
	err := sse.Run(ctx, sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) {
			step++
			if step == 1 {
				return &fakeBody{Reader: strings.NewReader("data: x\n\n")}, nil
			}
			cancel()
			return nil, ctx.Err()
		},
		Backoff: sse.Backoff{Base: time.Millisecond, Max: time.Millisecond},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v", err)
	}
}

func TestRunIdleTimeoutDrops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := newBlockingBody()
	go func() { _, _ = io.WriteString(body.w, ": ok\n\n") }()
	rec := &recorder{}
	step := 0
	err := sse.Run(ctx, sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) {
			step++
			if step == 1 {
				return body, nil
			}
			cancel()
			return nil, ctx.Err()
		},
		OnDrop:      rec.onDrop,
		Backoff:     chatBackoff,
		IdleTimeout: 50 * time.Millisecond,
		Sleep:       rec.sleep,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	if len(rec.errs) != 1 || !errors.Is(rec.errs[0], sse.ErrIdle) {
		t.Fatalf("drop errors = %v, want one ErrIdle", rec.errs)
	}
	if body.closeCount() == 0 {
		t.Fatal("the silent stream was not closed")
	}
	if len(rec.sleeps) != 1 || rec.sleeps[0].d != chatBackoff.Base {
		t.Fatalf("sleeps = %+v, want the usual backoff after an idle drop", rec.sleeps)
	}
}

func TestRunIdleTimerResetsOnRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const idle, beat, beats = 120 * time.Millisecond, 20 * time.Millisecond, 15
	body := newBlockingBody()
	go func() {
		for range beats {
			if _, err := io.WriteString(body.w, ": ping\n\n"); err != nil {
				return
			}
			time.Sleep(beat)
		}
		_, _ = io.WriteString(body.w, "data: done\n\n")
	}()
	rec := &recorder{}
	err := sse.Run(ctx, sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) { return body, nil },
		OnEvent: func(e sse.Event) {
			rec.onEvent(e)
			cancel()
		},
		OnDrop:      rec.onDrop,
		Backoff:     chatBackoff,
		IdleTimeout: idle,
		Sleep:       rec.sleep,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	if len(rec.drops) != 0 {
		t.Fatalf("drops = %v (%v) while pings kept coming every %v, want none", rec.drops, rec.errs, beat)
	}
	if !reflect.DeepEqual(rec.events, []string{"done"}) {
		t.Fatalf("events = %q, want the event after %v of pings", rec.events, beats*beat)
	}
}

func TestRunRestartReconnectsAtOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	restart := make(chan struct{}, 1)
	first := newBlockingBody()
	rec := &recorder{}
	opens := 0
	err := sse.Run(ctx, sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) {
			opens++
			if opens == 1 {
				return first, nil
			}
			cancel()
			return nil, ctx.Err()
		},
		OnOpen:  func() { restart <- struct{}{} },
		OnDrop:  rec.onDrop,
		Backoff: chatBackoff,
		Restart: restart,
		Sleep:   rec.sleep,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	if opens != 2 {
		t.Fatalf("opens = %d, want a second connection after the restart", opens)
	}
	if len(rec.errs) != 1 || !errors.Is(rec.errs[0], sse.ErrRestarted) {
		t.Fatalf("drop errors = %v, want one ErrRestarted", rec.errs)
	}
	if len(rec.sleeps) != 0 {
		t.Fatalf("sleeps = %+v, want an immediate reconnect", rec.sleeps)
	}
	if first.closeCount() == 0 {
		t.Fatal("the restarted stream was not closed")
	}
}

func TestRunIgnoresRestartBeforeOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	restart := make(chan struct{}, 1)
	restart <- struct{}{}
	body := newBlockingBody()
	go func() { _, _ = io.WriteString(body.w, "data: hello\n\n") }()
	rec := &recorder{}
	err := sse.Run(ctx, sse.Options{
		Open: func(context.Context) (io.ReadCloser, error) { return body, nil },
		OnEvent: func(e sse.Event) {
			rec.onEvent(e)
			cancel()
		},
		OnDrop:  rec.onDrop,
		Backoff: chatBackoff,
		Restart: restart,
		Sleep:   rec.sleep,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	if len(rec.drops) != 0 || !reflect.DeepEqual(rec.events, []string{"hello"}) {
		t.Fatalf("drops = %v events = %q, want a restart from before the connection ignored", rec.errs, rec.events)
	}
}
