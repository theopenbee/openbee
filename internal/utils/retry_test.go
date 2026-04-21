package utils_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/utils"
)

var errFake = errors.New("fake error")

func TestRetryWithBackoff_SuccessOnFirst(t *testing.T) {
	calls := 0
	err := utils.RetryWithBackoff(context.Background(), func() error {
		calls++
		return nil
	}, 5, time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryWithBackoff_SuccessOnThird(t *testing.T) {
	calls := 0
	err := utils.RetryWithBackoff(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errFake
		}
		return nil
	}, 5, time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_AllFail(t *testing.T) {
	calls := 0
	err := utils.RetryWithBackoff(context.Background(), func() error {
		calls++
		return errFake
	}, 5, time.Millisecond)
	if !errors.Is(err, errFake) {
		t.Fatalf("expected errFake, got %v", err)
	}
	if calls != 5 {
		t.Fatalf("expected 5 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := utils.RetryWithBackoff(ctx, func() error {
		calls++
		cancel() // cancel after first failure
		return errFake
	}, 5, time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call before cancel, got %d", calls)
	}
}

func TestRetryWithBackoff_ZeroMaxRetries(t *testing.T) {
	called := false
	err := utils.RetryWithBackoff(context.Background(), func() error {
		called = true
		return errFake
	}, 0, time.Millisecond)
	if !errors.Is(err, errFake) {
		t.Fatalf("expected errFake, got %v", err)
	}
	if !called {
		t.Fatal("expected fn to be called once")
	}
}
