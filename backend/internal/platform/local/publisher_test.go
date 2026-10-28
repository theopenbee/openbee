package local_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/theopenbee/openbee/internal/platform/local"
)

func TestActivityAggregates(t *testing.T) {
	pub := local.NewEventPublisher(local.NewSSEHub())
	key := "local:u1:worker:w1"

	assert.Equal(t, local.Activity{State: "idle"}, pub.Activity(key))

	pub.TaskState(key, "t1", "w1", "queued", 2)
	assert.Equal(t, local.Activity{State: "queued", QueuedAhead: 2}, pub.Activity(key))

	pub.TaskState(key, "t2", "w1", "queued", 1)
	assert.Equal(t, local.Activity{State: "queued", QueuedAhead: 1}, pub.Activity(key))

	pub.TaskState(key, "t3", "w1", "running", 0)
	assert.Equal(t, local.Activity{State: "running"}, pub.Activity(key))

	pub.TaskState(key, "t3", "w1", "completed", 0)
	assert.Equal(t, local.Activity{State: "queued", QueuedAhead: 1}, pub.Activity(key))

	pub.TaskState(key, "t1", "w1", "cancelled", 0)
	pub.TaskState(key, "t2", "w1", "failed", 0)
	assert.Equal(t, local.Activity{State: "idle"}, pub.Activity(key))
}

func TestActivityIsPerSession(t *testing.T) {
	pub := local.NewEventPublisher(local.NewSSEHub())

	pub.TaskState("local:u1:worker:w1", "t1", "w1", "running", 0)

	assert.Equal(t, local.Activity{State: "running"}, pub.Activity("local:u1:worker:w1"))
	assert.Equal(t, local.Activity{State: "idle"}, pub.Activity("local:u1:worker:w2"))
	assert.Equal(t, local.Activity{State: "idle"}, pub.Activity("local:u2:worker:w1"))
	assert.Equal(t, local.Activity{State: "idle"}, pub.Activity("local:u1"))
}

func TestPublisherIgnoresNonLocal(t *testing.T) {
	hub := local.NewSSEHub()
	pub := local.NewEventPublisher(hub)
	ch, unsub := hub.Subscribe("u1")
	defer unsub()

	pub.Message("telegram:u1", local.ChatMessage{ID: "m1"})
	pub.TaskState("telegram:u1", "t1", "w1", "running", 0)
	pub.Message("local:", local.ChatMessage{ID: "m2"})
	pub.TaskState("local:u1:bogus", "t2", "w1", "running", 0)

	assertNoEvent(t, ch)
	assert.Equal(t, local.Activity{State: "idle"}, pub.Activity("telegram:u1"))
	assert.Equal(t, local.Activity{State: "idle"}, pub.Activity("local:u1:bogus"))
}

func TestMessageEventCarriesConversation(t *testing.T) {
	hub := local.NewSSEHub()
	pub := local.NewEventPublisher(hub)
	ch, unsub := hub.Subscribe("u1")
	defer unsub()

	pub.Message("local:u1:worker:w1", local.ChatMessage{ID: "m1", Content: "hi"})
	pub.Message("local:u1", local.ChatMessage{ID: "m2"})

	ev := recvEvent(t, ch)
	assert.Equal(t, "message", ev.Type)
	assert.Equal(t, "worker:w1", ev.ConversationID)
	assert.Equal(t, uint64(1), ev.Seq)
	if assert.NotNil(t, ev.Message) {
		assert.Equal(t, "m1", ev.Message.ID)
		assert.Equal(t, "hi", ev.Message.Content)
	}
	assert.Equal(t, "bee", recvEvent(t, ch).ConversationID)
}

func TestTaskStateEventCarriesTask(t *testing.T) {
	hub := local.NewSSEHub()
	pub := local.NewEventPublisher(hub)
	ch, unsub := hub.Subscribe("u1")
	defer unsub()

	pub.TaskState("local:u1:worker:w1", "t1", "w1", "queued", 3)

	ev := recvEvent(t, ch)
	assert.Equal(t, local.Event{
		Type:           "task",
		Seq:            1,
		ConversationID: "worker:w1",
		TaskID:         "t1",
		WorkerID:       "w1",
		State:          "queued",
		QueuedAhead:    3,
	}, ev)
}

func TestWorkerStatusReachesEveryUser(t *testing.T) {
	hub := local.NewSSEHub()
	pub := local.NewEventPublisher(hub)
	ch1, unsub1 := hub.Subscribe("u1")
	defer unsub1()
	ch2, unsub2 := hub.Subscribe("u2")
	defer unsub2()

	pub.WorkerStatus("w1", "working")

	for _, ch := range []<-chan string{ch1, ch2} {
		ev := recvEvent(t, ch)
		assert.Equal(t, "worker_status", ev.Type)
		assert.Equal(t, "w1", ev.WorkerID)
		assert.Equal(t, "working", ev.Status)
		assert.Empty(t, ev.ConversationID)
	}
}
