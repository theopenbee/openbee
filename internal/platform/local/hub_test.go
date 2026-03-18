package local_test

import (
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/platform/local"
)

func TestSSEHub_SubscribeAndBroadcast(t *testing.T) {
	h := local.NewSSEHub()
	ch, unsub := h.Subscribe("local:s1")
	defer unsub()

	h.Broadcast("local:s1", `{"id":"r1"}`)

	select {
	case got := <-ch:
		if got != `{"id":"r1"}` {
			t.Errorf("expected JSON, got %q", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestSSEHub_Broadcast_IsolatesSessions(t *testing.T) {
	h := local.NewSSEHub()
	chA, unsubA := h.Subscribe("local:s1")
	defer unsubA()
	chB, unsubB := h.Subscribe("local:s2")
	defer unsubB()

	h.Broadcast("local:s1", "for-A")

	select {
	case <-chA:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("A should have received broadcast")
	}
	select {
	case msg := <-chB:
		t.Fatalf("B should not have received broadcast, got %q", msg)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestSSEHub_Unsubscribe_StopsReceiving(t *testing.T) {
	h := local.NewSSEHub()
	ch, unsub := h.Subscribe("local:s1")
	unsub()

	// Broadcast after unsubscribe: must not panic and must not deliver
	h.Broadcast("local:s1", "orphan")

	select {
	case msg := <-ch:
		t.Fatalf("should not receive after unsubscribe, got %q", msg)
	default:
		// expected: channel is empty and no panic occurred
	}
}

func TestSSEHub_MultipleSubscribers(t *testing.T) {
	h := local.NewSSEHub()
	ch1, unsub1 := h.Subscribe("local:s1")
	ch2, unsub2 := h.Subscribe("local:s1")
	defer unsub1()
	defer unsub2()

	h.Broadcast("local:s1", "hello")

	for _, ch := range []<-chan string{ch1, ch2} {
		select {
		case got := <-ch:
			if got != "hello" {
				t.Errorf("expected hello, got %q", got)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout: both subscribers should receive")
		}
	}
}
