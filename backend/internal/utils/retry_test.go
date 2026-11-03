package utils_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/utils"
)

var errFake = errors.New("fake error")

// TestRetryWithBackoff merges the retry-outcome scenarios (success on first/third
// attempt, exhausting all retries, and context cancellation mid-retry) into one
// table. Each case builds its own context/counter so state is never shared.
func TestRetryWithBackoff(t *testing.T) {
	cases := []struct {
		name       string
		maxRetries int
		fn         func(calls *int, cancel func()) error
		wantErrIs  error // nil means expect success
		wantCalls  int
	}{
		{
			name:       "SuccessOnFirst",
			maxRetries: 5,
			fn: func(calls *int, _ func()) error {
				*calls++
				return nil
			},
			wantCalls: 1,
		},
		{
			name:       "SuccessOnThird",
			maxRetries: 5,
			fn: func(calls *int, _ func()) error {
				*calls++
				if *calls < 3 {
					return errFake
				}
				return nil
			},
			wantCalls: 3,
		},
		{
			name:       "AllFail",
			maxRetries: 5,
			fn: func(calls *int, _ func()) error {
				*calls++
				return errFake
			},
			wantErrIs: errFake,
			wantCalls: 5,
		},
		{
			name:       "ContextCancelled",
			maxRetries: 5,
			fn: func(calls *int, cancel func()) error {
				*calls++
				cancel() // cancel after first failure
				return errFake
			},
			wantErrIs: context.Canceled,
			wantCalls: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			err := utils.RetryWithBackoff(ctx, func() error {
				return tc.fn(&calls, cancel)
			}, tc.maxRetries, time.Millisecond)

			if tc.wantErrIs != nil {
				require.ErrorIs(t, err, tc.wantErrIs)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantCalls, calls)
		})
	}
}

func TestRetryWithBackoff_ZeroMaxRetries(t *testing.T) {
	called := false
	err := utils.RetryWithBackoff(context.Background(), func() error {
		called = true
		return errFake
	}, 0, time.Millisecond)
	require.ErrorIs(t, err, errFake)
	require.True(t, called, "expected fn to be called once")
}
