package utils

import (
	"context"
	"time"
)

func RetryWithBackoff(ctx context.Context, fn func() error, maxRetries int, baseDelay time.Duration) error {
	if maxRetries <= 0 {
		return fn()
	}
	var err error
	delay := baseDelay
	for i := range maxRetries {
		if err = fn(); err == nil {
			return nil
		}
		if i < maxRetries-1 {
			t := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
			delay *= 2
		}
	}
	return err
}
