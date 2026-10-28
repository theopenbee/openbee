package model

import (
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func notifyEvent(cid string, sender api.ChatSender, mutate ...func(*api.ChatMessage)) api.ChatEvent {
	m := api.ChatMessage{ID: "m1", Sender: sender, Kind: "message", Content: "Build finished", TS: 1}
	for _, f := range mutate {
		f(&m)
	}
	return api.ChatEvent{Type: "message", Seq: 1, ConversationID: cid, Message: &m}
}

func TestShouldNotify(t *testing.T) {
	workerSender := api.ChatSender{Type: "worker", ID: "w1"}
	reset := func(m *api.ChatMessage) {
		m.Kind = "reset"
		m.Content = ""
	}

	tests := []struct {
		name       string
		ev         api.ChatEvent
		foreground bool
		want       bool
	}{
		{"notifies an agent reply while the page is in the background", notifyEvent("worker:w1", workerSender), false, true},
		{"notifies while the page is visible but the window has lost focus", notifyEvent("worker:w1", workerSender), false, true},
		{"stays quiet while the page is in front", notifyEvent("worker:w1", workerSender), true, false},
		{"never notifies the user's own messages", notifyEvent("bee", api.ChatSender{Type: "user"}), false, false},
		{"notifies Bee messages in the Bee chat", notifyEvent("bee", api.ChatSender{Type: "bee"}), false, true},
		{"notifies system messages", notifyEvent("worker:w1", api.ChatSender{Type: "system"}), false, true},
		{"skips the Bee's handoff copy in a worker chat", notifyEvent("worker:w1", api.ChatSender{Type: "bee"}), false, false},
		{"skips the new conversation divider", notifyEvent("worker:w1", api.ChatSender{Type: "system"}, reset), false, false},
		{"skips a worker's divider in the Bee chat too", notifyEvent("bee", workerSender, reset), false, false},
		{"ignores task events", api.ChatEvent{Type: "task", ConversationID: "worker:w1", WorkerID: "w1", State: "running"}, false, false},
		{"ignores worker status events", api.ChatEvent{Type: "worker_status", WorkerID: "w1", Status: "error"}, false, false},
		{"ignores a message event without a message", api.ChatEvent{Type: "message", ConversationID: "bee"}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldNotify(tc.ev, tc.foreground); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUnreadTotal(t *testing.T) {
	at := func(unread int) api.ConversationSummary {
		return api.ConversationSummary{ID: "bee", Kind: "bee", Unread: unread}
	}

	if got := UnreadTotal([]api.ConversationSummary{at(2), at(0), at(3)}); got != 5 {
		t.Fatalf("got %d, want 5", got)
	}
	if got := UnreadTotal(nil); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestBadgeText(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{-1, ""},
		{0, ""},
		{5, "5"},
		{99, "99"},
		{100, "99+"},
		{120, "99+"},
	}
	for _, tc := range tests {
		if got := BadgeText(tc.n); got != tc.want {
			t.Errorf("BadgeText(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
