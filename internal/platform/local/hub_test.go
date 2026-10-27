package local_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform/local"
)

func recvEvent(t *testing.T, ch <-chan string) local.Event {
	t.Helper()
	select {
	case data := <-ch:
		var ev local.Event
		require.NoError(t, json.Unmarshal([]byte(data), &ev))
		return ev
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
		return local.Event{}
	}
}

func assertNoEvent(t *testing.T, ch <-chan string) {
	t.Helper()
	select {
	case data := <-ch:
		t.Fatalf("expected no event, got %q", data)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSSEHub_SubscribeAndPublish(t *testing.T) {
	h := local.NewSSEHub()
	ch, unsub := h.Subscribe("u1")
	defer unsub()

	h.Publish("u1", local.Event{Type: "task", TaskID: "t1"})

	ev := recvEvent(t, ch)
	assert.Equal(t, "task", ev.Type)
	assert.Equal(t, "t1", ev.TaskID)
	assert.Equal(t, uint64(1), ev.Seq)
}

func TestSSEHub_Publish_IsolatesUsers(t *testing.T) {
	h := local.NewSSEHub()
	chA, unsubA := h.Subscribe("u1")
	defer unsubA()
	chB, unsubB := h.Subscribe("u2")
	defer unsubB()

	h.Publish("u1", local.Event{Type: "task"})

	recvEvent(t, chA)
	assertNoEvent(t, chB)
}

func TestSSEHub_Unsubscribe_StopsReceiving(t *testing.T) {
	h := local.NewSSEHub()
	ch, unsub := h.Subscribe("u1")
	unsub()

	h.Publish("u1", local.Event{Type: "task"})
	h.PublishAll(local.Event{Type: "worker_status"})

	select {
	case msg := <-ch:
		t.Fatalf("should not receive after unsubscribe, got %q", msg)
	default:
	}
}

func TestSSEHub_MultipleSubscribers(t *testing.T) {
	h := local.NewSSEHub()
	ch1, unsub1 := h.Subscribe("u1")
	ch2, unsub2 := h.Subscribe("u1")
	defer unsub1()
	defer unsub2()

	h.Publish("u1", local.Event{Type: "task", TaskID: "t1"})

	for _, ch := range []<-chan string{ch1, ch2} {
		ev := recvEvent(t, ch)
		assert.Equal(t, "t1", ev.TaskID)
		assert.Equal(t, uint64(1), ev.Seq)
	}
}

func TestHubSeqPerUser(t *testing.T) {
	h := local.NewSSEHub()
	ch1, unsub1 := h.Subscribe("u1")
	defer unsub1()
	ch2, unsub2 := h.Subscribe("u2")
	defer unsub2()

	h.Publish("u1", local.Event{Type: "task"})
	h.Publish("u1", local.Event{Type: "task"})
	h.Publish("u2", local.Event{Type: "task"})

	assert.Equal(t, uint64(1), recvEvent(t, ch1).Seq)
	assert.Equal(t, uint64(2), recvEvent(t, ch1).Seq)
	assert.Equal(t, uint64(1), recvEvent(t, ch2).Seq)
}

func TestHubPublishAllReachesEveryUserWithItsOwnSeq(t *testing.T) {
	h := local.NewSSEHub()
	ch1, unsub1 := h.Subscribe("u1")
	defer unsub1()
	ch2, unsub2 := h.Subscribe("u2")
	defer unsub2()
	h.Publish("u1", local.Event{Type: "task"})

	h.PublishAll(local.Event{Type: "worker_status", WorkerID: "w1", Status: "busy"})

	assert.Equal(t, uint64(1), recvEvent(t, ch1).Seq)
	ev1 := recvEvent(t, ch1)
	assert.Equal(t, uint64(2), ev1.Seq)
	assert.Equal(t, "w1", ev1.WorkerID)
	ev2 := recvEvent(t, ch2)
	assert.Equal(t, uint64(1), ev2.Seq)
	assert.Equal(t, "busy", ev2.Status)
}

func TestHubFullSubscriberDropsEventAndSeqShowsGap(t *testing.T) {
	h := local.NewSSEHub()
	ch, unsub := h.Subscribe("u1")
	defer unsub()

	for range 9 {
		h.Publish("u1", local.Event{Type: "task"})
	}
	for want := uint64(1); want <= 8; want++ {
		assert.Equal(t, want, recvEvent(t, ch).Seq)
	}
	assertNoEvent(t, ch)

	h.Publish("u1", local.Event{Type: "task"})
	assert.Equal(t, uint64(10), recvEvent(t, ch).Seq)
}
