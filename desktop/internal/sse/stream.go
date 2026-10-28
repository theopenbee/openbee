package sse

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"
	"time"
)

var ErrGaveUp = errors.New("sse: gave up")

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
	Wake        <-chan struct{}
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
	var once sync.Once
	closeBody := func() { once.Do(func() { body.Close() }) }
	stop := context.AfterFunc(ctx, closeBody)
	defer func() {
		stop()
		closeBody()
	}()
	*attempt = 0
	if o.OnOpen != nil {
		o.OnOpen()
	}
	err = Read(body, func(e Event) error {
		if o.OnEvent != nil {
			o.OnEvent(e)
		}
		return nil
	})
	drain(o.Wake)
	if err == nil {
		return io.EOF
	}
	return err
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
