package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform/local"
)

func TestBeeRunStateReachesBeeConversation(t *testing.T) {
	hub := local.NewSSEHub()
	pub := local.NewEventPublisher(hub)
	observer := localTaskObserver{pub: pub}
	key := local.SessionKey("u1")
	ch, unsub := hub.Subscribe("u1")
	defer unsub()

	observer.BeeRunStateChanged(key, "bee-exec-1", model.ExecStatusRunning)

	assert.Equal(t, local.Activity{State: "running", ExecutionID: "bee-exec-1"}, pub.Activity(key))
	select {
	case data := <-ch:
		var ev local.Event
		require.NoError(t, json.Unmarshal([]byte(data), &ev))
		assert.Equal(t, local.Event{
			Type:           "task",
			Seq:            ev.Seq,
			ConversationID: local.BeeConversationID,
			TaskID:         "bee-exec-1",
			State:          "running",
			ExecutionID:    "bee-exec-1",
		}, ev)
	case <-time.After(time.Second):
		require.FailNow(t, "no task event")
	}

	observer.BeeRunStateChanged(key, "bee-exec-1", model.ExecStatusCompleted)
	assert.Equal(t, local.Activity{State: "idle"}, pub.Activity(key))
}
