package model

import (
	"reflect"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const minute = int64(time.Minute / time.Millisecond)

func viewMsg(sender api.ChatSender, ts int64, mutate ...func(*Message)) Message {
	m := Message{ChatMessage: api.ChatMessage{ID: sender.Type + "-" + sender.ID, Sender: sender, Kind: "message", Content: "hi", TS: ts}}
	for _, f := range mutate {
		f(&m)
	}
	return m
}

func asReset(m *Message) {
	m.Kind = "reset"
	m.Content = ""
}

func startFlags(msgs []Message) []bool {
	out := make([]bool, len(msgs))
	for i := range msgs {
		out[i] = StartsGroup(msgs, i)
	}
	return out
}

func TestGroupGap(t *testing.T) {
	if GroupGap != 5*time.Minute {
		t.Fatalf("GroupGap = %v", GroupGap)
	}
}

func TestStartsGroup(t *testing.T) {
	user := api.ChatSender{Type: "user"}
	bee := api.ChatSender{Type: "bee"}
	system := api.ChatSender{Type: "system"}

	t.Run("splits on sender and five minute gap", func(t *testing.T) {
		msgs := []Message{
			viewMsg(user, 0),
			viewMsg(user, 1*minute),
			viewMsg(bee, 2*minute),
			viewMsg(bee, 6*minute+59_999),
			viewMsg(bee, 11*minute+59_999),
			viewMsg(api.ChatSender{Type: "worker", ID: "w1"}, 12*minute),
			viewMsg(api.ChatSender{Type: "worker", ID: "w2"}, 12*minute),
			viewMsg(api.ChatSender{Type: "worker", ID: "w2"}, 13*minute),
		}

		got := startFlags(msgs)

		want := []bool{true, false, true, false, true, true, true, false}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("returns nothing for no messages", func(t *testing.T) {
		if got := startFlags(nil); len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("starts a new group on both sides of a new conversation divider", func(t *testing.T) {
		msgs := []Message{
			viewMsg(system, 0),
			viewMsg(system, 1*minute, asReset),
			viewMsg(system, 2*minute),
			viewMsg(system, 3*minute),
		}

		got := startFlags(msgs)

		want := []bool{true, true, true, false}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("an index outside the list starts nothing", func(t *testing.T) {
		msgs := []Message{viewMsg(user, 0)}
		if StartsGroup(msgs, 1) || StartsGroup(msgs, -1) {
			t.Fatal("out of range index reported a group start")
		}
	})
}

func TestChatStatus(t *testing.T) {
	w := func(status string) *api.Worker {
		worker := worker("w1", "Lin", "")
		worker.Status = status
		return &worker
	}

	tests := []struct {
		name      string
		activity  api.ConversationActivity
		worker    *api.Worker
		wantKind  StatusKind
		wantAhead int
	}{
		{"queued beats worker error", api.ConversationActivity{State: "queued", QueuedAhead: 2}, w("error"), StatusQueued, 2},
		{"running beats worker error", api.ConversationActivity{State: "running"}, w("error"), StatusRunning, 0},
		{"counts zero ahead when the position is unknown", api.ConversationActivity{State: "queued"}, nil, StatusQueued, 0},
		{"idle with a worker in error", api.ConversationActivity{State: "idle"}, w("error"), StatusError, 0},
		{"idle with a working worker", api.ConversationActivity{State: "idle"}, w("working"), StatusIdle, 0},
		{"idle without a worker", api.ConversationActivity{State: "idle"}, nil, StatusIdle, 0},
		{"empty state is idle", api.ConversationActivity{}, nil, StatusIdle, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kind, ahead := ChatStatus(tc.activity, tc.worker)
			if kind != tc.wantKind || ahead != tc.wantAhead {
				t.Fatalf("got (%v, %d), want (%v, %d)", kind, ahead, tc.wantKind, tc.wantAhead)
			}
		})
	}
}

func TestHandoffPreview(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"keeps the first three non-blank lines", "  one\n\n two \nthree\nfour", "one\ntwo\nthree"},
		{"keeps fewer lines as they are", "only", "only"},
		{"is empty for blank content", " \n\t\n", ""},
		{"trims carriage returns", "one\r\ntwo\r\n", "one\ntwo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := HandoffPreview(tc.content); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
