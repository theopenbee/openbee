package sse

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"
	"time"
)

var (
	ErrGaveUp    = errors.New("sse: gave up")
	ErrIdle      = errors.New("sse: stream went silent")
	ErrRestarted = errors.New("sse: stream restarted")
)

type Backoff struct{ Base, Max time.Duration }

func (b Backoff) Delay(attempt int) time.Duration {
	limit := b.Max
	if limit <= 0 {
		limit = math.MaxInt64
	}
	d := min(b.Base, limit)
	for i := 1; i < attempt && d > 0; i++ {
		if d > limit-d {
			return limit
		}
		d *= 2
	}
	return d
}

type Options struct {
	Open        func(ctx context.Context) (io.ReadCloser, error)
	OnOpen      func()
	OnEvent     func(Event)
	OnDrop      func(err error, attempt int)
	Backoff     Backoff
	MaxAttempts int
	IdleTimeout time.Duration
	Wake        <-chan struct{}
	Restart     <-chan struct{}
	Sleep       func(ctx context.Context, d time.Duration, wake <-chan struct{}) error
}

func Run(ctx context.Context, o Options) error {
	sleep := o.Sleep
	if sleep == nil {
		sleep = defaultSleep
	}
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		dropErr := stream(ctx, &o, &attempt)
		if err := ctx.Err(); err != nil {
			return err
		}
		attempt++
		if o.OnDrop != nil {
			o.OnDrop(dropErr, attempt)
		}
		if o.MaxAttempts > 0 && attempt >= o.MaxAttempts {
			return ErrGaveUp
		}
		if errors.Is(dropErr, ErrRestarted) {
			continue
		}
		if err := sleep(ctx, o.Backoff.Delay(attempt), o.Wake); err != nil {
			return err
		}
	}
}

func stream(ctx context.Context, o *Options, attempt *int) error {
	body, err := o.Open(ctx)
	if err != nil {
		return err
	}
	e := &ending{body: body}
	stop := context.AfterFunc(ctx, func() { e.end(ctx.Err()) })
	done := make(chan struct{})
	defer func() {
		close(done)
		stop()
		e.end(nil)
	}()
	*attempt = 0
	if o.Restart != nil {
		drain(o.Restart)
		go func() {
			select {
			case <-o.Restart:
				e.end(ErrRestarted)
			case <-done:
			}
		}()
	}
	var r io.Reader = body
	if o.IdleTimeout > 0 {
		idle := time.AfterFunc(o.IdleTimeout, func() { e.end(ErrIdle) })
		defer idle.Stop()
		r = &watchedReader{r: body, idle: idle, timeout: o.IdleTimeout}
	}
	if o.OnOpen != nil {
		o.OnOpen()
	}
	err = Read(r, func(ev Event) error {
		if o.OnEvent != nil {
			o.OnEvent(ev)
		}
		return nil
	})
	drain(o.Wake)
	if cause := e.cause(); cause != nil {
		return cause
	}
	if err == nil {
		return io.EOF
	}
	return err
}

type ending struct {
	body io.Closer
	once sync.Once
	mu   sync.Mutex
	why  error
}

func (e *ending) end(why error) {
	e.once.Do(func() {
		e.mu.Lock()
		e.why = why
		e.mu.Unlock()
		e.body.Close()
	})
}

func (e *ending) cause() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.why
}

type watchedReader struct {
	r       io.Reader
	idle    *time.Timer
	timeout time.Duration
}

func (w *watchedReader) Read(p []byte) (int, error) {
	n, err := w.r.Read(p)
	if n > 0 {
		w.idle.Reset(w.timeout)
	}
	return n, err
}

func drain(wake <-chan struct{}) {
	for {
		select {
		case <-wake:
		default:
			return
		}
	}
}

func defaultSleep(ctx context.Context, d time.Duration, wake <-chan struct{}) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wake:
		return nil
	case <-t.C:
		return nil
	}
}
