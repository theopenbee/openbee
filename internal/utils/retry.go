package utils

import (
	"context"
	"time"
)

// RetryWithBackoff retries fn with exponential backoff up to maxRetries times.
// The first attempt is immediate. On failure, waits baseDelay before the next,
// doubling each round. Returns ctx.Err() if cancelled between retries, or the
// last error if all attempts fail.
func RetryWithBackoff(ctx context.Context, fn func() error, maxRetries int, baseDelay time.Duration) error {
	if maxRetries <= 0 {
		return fn()
	}
	var err error
	delay := baseDelay
	for i := 0; i < maxRetries; i++ {
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
