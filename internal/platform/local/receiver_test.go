package local_test

import (
	"context"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
)

func TestLocalReceiver_EnqueueAndDispatch(t *testing.T) {
	r := local.NewLocalReceiver(8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var dispatched []platform.InboundMessage
	done := make(chan struct{})

	go func() {
		r.Start(ctx, func(msg platform.InboundMessage) { //nolint:errcheck
			dispatched = append(dispatched, msg)
			if len(dispatched) == 1 {
				close(done)
			}
		})
	}()

	r.Enqueue(platform.InboundMessage{
		Platform:   "local",
		SessionKey: "local:s1",
		Content:    "hello",
	})

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout: dispatch not called")
	}

	if dispatched[0].Content != "hello" {
		t.Errorf("expected hello, got %q", dispatched[0].Content)
	}
}

func TestLocalReceiver_Start_ReturnsNilOnCancel(t *testing.T) {
	r := local.NewLocalReceiver(8)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- r.Start(ctx, func(platform.InboundMessage) {})
	}()

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("expected nil error on cancel, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Start did not return after context cancel")
	}
}

func TestLocalReceiver_Enqueue_DropWhenFull(t *testing.T) {
	// Channel size 1, never drained — second enqueue must not block
	r := local.NewLocalReceiver(1)

	r.Enqueue(platform.InboundMessage{Content: "first"})
	// This must return immediately (not block) even though channel is full
	done := make(chan struct{})
	go func() {
		r.Enqueue(platform.InboundMessage{Content: "second"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Enqueue blocked on full channel")
	}
}
