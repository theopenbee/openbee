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
	var err error
	delay := baseDelay
	for i := 0; i < maxRetries; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i < maxRetries-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
	}
	return err
}
